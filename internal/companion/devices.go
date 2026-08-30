package companion

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/seergs/vikunja-companion/internal/crypto"
	"github.com/seergs/vikunja-companion/internal/store"
)

type registerDeviceRequest struct {
	APNsToken  string `json:"apns_token"`
	PublicKey  string `json:"public_key"` // base64 (std or url), 32 bytes
	AppVersion string `json:"app_version"`
}

type unregisterDeviceRequest struct {
	APNsToken string `json:"apns_token"`
}

// registerDevice upserts the caller's device and refreshes their stored token.
func (a *api) registerDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := a.resolve(w, r)
	if !ok {
		return
	}
	var body registerDeviceRequest
	if !readJSON(w, r, &body) {
		return
	}
	if body.APNsToken == "" {
		writeJSON(w, http.StatusBadRequest, errBody("apns_token is required"))
		return
	}
	pub, err := decodeKey(body.PublicKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}

	ctx := r.Context()
	if tokenEnc, err := a.cipher.Encrypt([]byte(bearerToken(r))); err == nil {
		if err := a.store.UpsertUserToken(ctx, id.UserID, tokenEnc); err != nil {
			a.log.Error("storing user token", "user", id.UserID, "err", err)
			writeJSON(w, http.StatusInternalServerError, errBody("could not register device"))
			return
		}
	}

	a.cacheTimezone(ctx, id.UserID, bearerToken(r))

	deviceID, err := a.store.UpsertDevice(ctx, store.Device{
		UserID:     id.UserID,
		APNsToken:  body.APNsToken,
		PublicKey:  pub,
		AppVersion: body.AppVersion,
	})
	if err != nil {
		a.log.Error("upserting device", "user", id.UserID, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("could not register device"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"device_id": deviceID})
}

// unregisterDevice deletes the caller's device. Removing their last device also
// deletes the stored token and webhook secret.
func (a *api) unregisterDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := a.resolve(w, r)
	if !ok {
		return
	}
	var body unregisterDeviceRequest
	if !readJSON(w, r, &body) {
		return
	}
	if body.APNsToken == "" {
		writeJSON(w, http.StatusBadRequest, errBody("apns_token is required"))
		return
	}

	ctx := r.Context()
	remaining, err := a.store.DeleteDevice(ctx, id.UserID, body.APNsToken)
	if err != nil {
		a.log.Error("deleting device", "user", id.UserID, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("could not unregister device"))
		return
	}
	if remaining == 0 {
		if err := a.store.DeleteUser(ctx, id.UserID); err != nil {
			a.log.Error("deleting user after last device", "user", id.UserID, "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeKey accepts a 32-byte X25519 public key as std or url base64.
func decodeKey(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil && len(b) == crypto.PublicKeySize {
			return b, nil
		}
	}
	return nil, errors.New("public_key must be a base64-encoded 32-byte X25519 key")
}
