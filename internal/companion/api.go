package companion

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/seergs/vikunja-companion/internal/crypto"
	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

// dispatcher delivers a user's notifications to their configured channel
// (internal/notify).
type dispatcher interface {
	Dispatch(ctx context.Context, userID int64, notifications []notify.Notification) error
}

// userSettingsFetcher reads a caller's Vikunja settings (the timezone the
// digest send time is interpreted in). Satisfied by *vikunja.Client.
type userSettingsFetcher interface {
	UserSettings(ctx context.Context, token string) (*vikunja.UserSettings, error)
}

// api holds the dependencies for the authenticated /companion/v1/* routes and
// the inbound webhook route.
type api struct {
	store         *store.DB
	cipher        *crypto.Cipher
	dispatch      dispatcher
	userSettings  userSettingsFetcher
	identity      *IdentityCache
	webhookTarget string   // {PublicURL}/companion/v1/webhooks/vikunja
	webhookEvents []string // operator ceiling (COMPANION_WEBHOOK_EVENTS)
	log           *slog.Logger
}

// resolve authenticates the request and returns the caller. On failure it has
// already written the response and returns ok=false.
func (a *api) resolve(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	token := bearerToken(r)
	id, err := a.identity.Resolve(r.Context(), token)
	if err != nil {
		a.log.Warn("auth failed for "+r.URL.Path,
			"has_bearer", token != "",
			"upstream_401", vikunja.IsUnauthorized(err),
			"err", err)
		writeAuthError(w, err)
		return Identity{}, false
	}
	return id, true
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

func writeAuthError(w http.ResponseWriter, err error) {
	// A missing/blank token and a token Vikunja rejects are both 401; anything
	// else (transport failure, upstream 5xx) is a 502.
	if errors.Is(err, ErrNoToken) || vikunja.IsUnauthorized(err) {
		writeJSON(w, http.StatusUnauthorized, errBody("invalid or missing token"))
		return
	}
	writeJSON(w, http.StatusBadGateway, errBody("could not reach Vikunja to verify the token"))
}
