package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetSettingsReturnsDefaults(t *testing.T) {
	e := newTestEnv(t, nil)

	rec := do(t, e.handler, "GET", "/companion/v1/settings", e.userToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var s settingsResponse
	json.Unmarshal(rec.Body.Bytes(), &s)
	if !s.Digest.Enabled || s.Digest.Time != "08:00" {
		t.Errorf("defaults = %+v", s)
	}
}

func TestGetSettingsRequiresAuth(t *testing.T) {
	e := newTestEnv(t, nil)
	if rec := do(t, e.handler, "GET", "/companion/v1/settings", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestPutSettingsStoresAndEchoes(t *testing.T) {
	e := newTestEnv(t, nil)

	rec := do(t, e.handler, "PUT", "/companion/v1/settings", e.userToken,
		`{"digest":{"enabled":false,"time":"07:30"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var s settingsResponse
	json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Digest.Enabled || s.Digest.Time != "07:30" {
		t.Errorf("echo = %+v", s)
	}
	// timezone was resolved from the upstream /api/v1/user and cached
	if s.Timezone != "America/New_York" {
		t.Errorf("timezone = %q", s.Timezone)
	}

	stored, err := e.store.UserSettings(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored.DigestEnabled || stored.DigestTime != "07:30" || stored.Timezone != "America/New_York" {
		t.Errorf("stored = %+v", stored)
	}
}

func TestPutSettingsRejectsBadTime(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, body := range []string{
		`{"digest":{"enabled":true,"time":"24:00"}}`,
		`{"digest":{"enabled":true,"time":"8am"}}`,
		`{"digest":{"enabled":true}}`,
		`{}`,
	} {
		if rec := do(t, e.handler, "PUT", "/companion/v1/settings", e.userToken, body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s -> status %d, want 400", body, rec.Code)
		}
	}
}
