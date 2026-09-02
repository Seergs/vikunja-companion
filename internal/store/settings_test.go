package store

import (
	"context"
	"testing"
)

func TestUserSettingsDefaults(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	s, err := db.UserSettings(ctx, 99)
	if err != nil {
		t.Fatal(err)
	}
	if !s.DigestEnabled || s.DigestTime != "08:00" || s.Timezone != "" {
		t.Errorf("defaults = %+v", s)
	}
}

func TestPutDigestAndTimezoneAreIndependent(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.SetUserTimezone(ctx, 1, "Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	if err := db.PutDigestSettings(ctx, 1, false, "07:15"); err != nil {
		t.Fatal(err)
	}
	// timezone survives a digest write
	if err := db.PutDigestSettings(ctx, 1, true, "09:00"); err != nil {
		t.Fatal(err)
	}

	s, err := db.UserSettings(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !s.DigestEnabled || s.DigestTime != "09:00" || s.Timezone != "Europe/Berlin" {
		t.Errorf("settings = %+v", s)
	}
}

func TestPutDigestSettingsRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.PutDigestSettings(ctx, 5, true, "06:30"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserTimezone(ctx, 5, "UTC"); err != nil {
		t.Fatal(err)
	}

	s, err := db.UserSettings(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !s.DigestEnabled || s.DigestTime != "06:30" || s.Timezone != "UTC" {
		t.Errorf("settings = %+v", s)
	}
}
