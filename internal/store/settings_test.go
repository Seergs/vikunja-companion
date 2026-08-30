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
	seedUser(t, db, 1)

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

func TestListDigestTargets(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	// user 1: has a device, custom settings
	seedUser(t, db, 1)
	if _, err := db.UpsertDevice(ctx, Device{UserID: 1, APNsToken: "a", PublicKey: make([]byte, 32)}); err != nil {
		t.Fatal(err)
	}
	db.SetUserTimezone(ctx, 1, "UTC")
	db.PutDigestSettings(ctx, 1, true, "06:30")

	// user 2: has a device, no settings row -> defaults
	seedUser(t, db, 2)
	if _, err := db.UpsertDevice(ctx, Device{UserID: 2, APNsToken: "b", PublicKey: make([]byte, 32)}); err != nil {
		t.Fatal(err)
	}

	// user 3: no device -> excluded
	seedUser(t, db, 3)

	targets, err := db.ListDigestTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2: %+v", len(targets), targets)
	}
	if targets[0].UserID != 1 || targets[0].Time != "06:30" || targets[0].Timezone != "UTC" || !targets[0].Enabled {
		t.Errorf("target 1 = %+v", targets[0])
	}
	if targets[1].UserID != 2 || targets[1].Time != "08:00" || targets[1].Timezone != "" || !targets[1].Enabled {
		t.Errorf("target 2 = %+v", targets[1])
	}
}
