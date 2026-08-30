package companion

import (
	"context"
	"net/http"

	"github.com/seergs/vikunja-companion/internal/digest"
)

// digestSettings is the morning-briefing preferences the iOS app controls.
type digestSettings struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"` // "HH:MM" in the user's Vikunja timezone
}

// settingsResponse is the body of GET /companion/v1/settings.
type settingsResponse struct {
	Digest   digestSettings `json:"digest"`
	Timezone string         `json:"timezone"` // resolved from Vikunja; "" until known
}

type putSettingsRequest struct {
	Digest *digestSettings `json:"digest"`
}

// getSettings returns the caller's notification preferences (defaults when they
// have never saved any).
func (a *api) getSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := a.resolve(w, r)
	if !ok {
		return
	}
	a.writeSettings(w, r.Context(), id.UserID)
}

// putSettings stores the caller's digest preferences and opportunistically
// caches their timezone so the cron does not have to wait for a tick.
func (a *api) putSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := a.resolve(w, r)
	if !ok {
		return
	}
	var body putSettingsRequest
	if !readJSON(w, r, &body) {
		return
	}
	if body.Digest == nil {
		writeJSON(w, http.StatusBadRequest, errBody("digest is required"))
		return
	}
	if _, _, err := digest.ParseHHMM(body.Digest.Time); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("digest.time must be HH:MM (24-hour)"))
		return
	}

	ctx := r.Context()
	// Persist the caller's token (the digest cron acts as them between app
	// sessions) — this also creates the users row user_settings references.
	if tokenEnc, err := a.cipher.Encrypt([]byte(bearerToken(r))); err == nil {
		if err := a.store.UpsertUserToken(ctx, id.UserID, tokenEnc); err != nil {
			a.log.Error("storing user token", "user", id.UserID, "err", err)
			writeJSON(w, http.StatusInternalServerError, errBody("could not save settings"))
			return
		}
	}
	if err := a.store.PutDigestSettings(ctx, id.UserID, body.Digest.Enabled, body.Digest.Time); err != nil {
		a.log.Error("saving settings", "user", id.UserID, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("could not save settings"))
		return
	}
	a.cacheTimezone(ctx, id.UserID, bearerToken(r))
	a.writeSettings(w, ctx, id.UserID)
}

func (a *api) writeSettings(w http.ResponseWriter, ctx context.Context, userID int64) {
	s, err := a.store.UserSettings(ctx, userID)
	if err != nil {
		a.log.Error("loading settings", "user", userID, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("could not load settings"))
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{
		Digest:   digestSettings{Enabled: s.DigestEnabled, Time: s.DigestTime},
		Timezone: s.Timezone,
	})
}

// cacheTimezone fetches the user's Vikunja timezone and stores it. Best effort:
// a failure here just means the cron resolves it on its first pass.
func (a *api) cacheTimezone(ctx context.Context, userID int64, token string) {
	s, err := a.userSettings.UserSettings(ctx, token)
	if err != nil {
		a.log.Debug("could not fetch timezone", "user", userID, "err", err)
		return
	}
	if s.Timezone == "" {
		return
	}
	if err := a.store.SetUserTimezone(ctx, userID, s.Timezone); err != nil {
		a.log.Warn("caching timezone", "user", userID, "err", err)
	}
}
