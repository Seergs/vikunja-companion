package companion

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/webhook"
)

// webhookInfoResponse is the body of GET /companion/v1/webhook — everything the
// app needs for the "Set up push" screen.
type webhookInfoResponse struct {
	TargetURL      string     `json:"target_url"`
	Secret         string     `json:"secret"`
	Events         []string   `json:"events"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
}

// getWebhook returns the values to paste into Vikunja's webhook form plus the
// last time a delivery was verified. Idempotent: it creates the user's secret
// on first call and returns the same one thereafter.
func (a *api) getWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.resolve(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	secret, err := a.ensureWebhookSecret(ctx, id.UserID, bearerToken(r))
	if err != nil {
		a.log.Error("webhook setup", "user", id.UserID, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("could not prepare webhook secret"))
		return
	}

	resp := webhookInfoResponse{TargetURL: a.webhookTarget, Secret: secret, Events: a.webhookEvents}
	if wh, err := a.store.Webhook(ctx, id.UserID); err == nil && !wh.LastDeliveryAt.IsZero() {
		t := wh.LastDeliveryAt
		resp.LastDeliveryAt = &t
	}
	writeJSON(w, http.StatusOK, resp)
}

// ensureWebhookSecret stores the caller's Vikunja token (encrypted) and returns
// their webhook HMAC secret, generating and storing it on first call.
func (a *api) ensureWebhookSecret(ctx context.Context, userID int64, token string) (string, error) {
	tokenEnc, err := a.cipher.Encrypt([]byte(token))
	if err != nil {
		return "", err
	}
	if err := a.store.UpsertUserToken(ctx, userID, tokenEnc); err != nil {
		return "", err
	}

	switch wh, err := a.store.Webhook(ctx, userID); {
	case err == nil:
		plain, derr := a.cipher.Decrypt(wh.SecretEnc)
		if derr != nil {
			return "", derr
		}
		return string(plain), nil
	case errors.Is(err, store.ErrNotFound):
		secret := webhook.NewSecret()
		secretEnc, eerr := a.cipher.Encrypt([]byte(secret))
		if eerr != nil {
			return "", eerr
		}
		if serr := a.store.UpsertWebhook(ctx, userID, secretEnc, a.webhookEvents); serr != nil {
			return "", serr
		}
		return secret, nil
	default:
		return "", err
	}
}

// inboundWebhook receives deliveries from Vikunja. It has no bearer token — the
// sender is identified by which stored HMAC secret verifies the body.
func (a *api) inboundWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, errBody("body too large"))
		return
	}
	ctx := r.Context()

	userID, matched := a.matchSecret(ctx, body, r.Header.Get(webhook.SignatureHeader))
	if !matched {
		writeJSON(w, http.StatusUnauthorized, errBody("signature verification failed"))
		return
	}
	if err := a.store.TouchWebhookDelivery(ctx, userID); err != nil {
		a.log.Warn("touch webhook delivery", "user", userID, "err", err)
	}

	ev, err := webhook.Parse(body)
	if err != nil {
		var unsupported *webhook.ErrUnsupportedEvent
		if errors.As(err, &unsupported) {
			a.log.Debug("ignoring unsupported webhook event", "event", unsupported.Name, "user", userID)
			w.WriteHeader(http.StatusOK)
			return
		}
		a.log.Warn("parsing webhook body", "user", userID, "err", err)
		writeJSON(w, http.StatusBadRequest, errBody("could not parse event"))
		return
	}

	notifications := webhook.Build(ev)
	if len(notifications) > 0 {
		devices, err := a.devicesForUser(ctx, userID)
		if err != nil {
			a.log.Error("loading devices", "user", userID, "err", err)
			writeJSON(w, http.StatusInternalServerError, errBody("delivery failed"))
			return
		}
		if err := a.dispatch.Dispatch(ctx, devices, notifications); err != nil {
			a.log.Error("dispatching notifications", "user", userID, "err", err)
			writeJSON(w, http.StatusInternalServerError, errBody("delivery failed"))
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// matchSecret finds the user whose stored secret verifies (body, sig).
func (a *api) matchSecret(ctx context.Context, body []byte, sig string) (int64, bool) {
	if sig == "" {
		return 0, false
	}
	secrets, err := a.store.AllWebhookSecrets(ctx)
	if err != nil {
		a.log.Error("loading webhook secrets", "err", err)
		return 0, false
	}
	for _, s := range secrets {
		plain, derr := a.cipher.Decrypt(s.SecretEnc)
		if derr != nil {
			a.log.Warn("decrypting stored webhook secret", "user", s.UserID, "err", derr)
			continue
		}
		if webhook.Verify(body, sig, string(plain)) {
			return s.UserID, true
		}
	}
	return 0, false
}

func (a *api) devicesForUser(ctx context.Context, userID int64) ([]notify.Device, error) {
	rows, err := a.store.DevicesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]notify.Device, 0, len(rows))
	for _, d := range rows {
		out = append(out, notify.Device{APNsToken: d.APNsToken, PublicKey: d.PublicKey})
	}
	return out, nil
}
