package config

import (
	"strings"
	"testing"
)

func setCompanionEnv(t *testing.T, extra map[string]string) {
	t.Helper()
	base := map[string]string{
		"COMPANION_PUBLIC_URL": "https://companion.example.com",
		"VIKUNJA_UPSTREAM_URL": "https://vikunja.example.com",
		"COMPANION_MASTER_KEY": strings.Repeat("a", 64), // 32 bytes hex
	}
	for k, v := range base {
		if _, ok := extra[k]; !ok {
			t.Setenv(k, v)
		}
	}
	for k, v := range extra {
		t.Setenv(k, v)
	}
}

func TestLoadCompanionDefaults(t *testing.T) {
	setCompanionEnv(t, nil)

	c, err := LoadCompanion()
	if err != nil {
		t.Fatalf("LoadCompanion: %v", err)
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", c.ListenAddr)
	}
	if c.DBPath != "/data/companion.db" {
		t.Errorf("DBPath = %q", c.DBPath)
	}
	if c.RelayURL != DefaultRelayURL {
		t.Errorf("RelayURL = %q, want default", c.RelayURL)
	}
	if len(c.MasterKey) != 32 {
		t.Errorf("MasterKey length = %d, want 32", len(c.MasterKey))
	}
	if len(c.WebhookEvents) != len(KnownWebhookEvents) {
		t.Errorf("WebhookEvents = %v", c.WebhookEvents)
	}
	if c.APNS != nil {
		t.Errorf("APNS = %+v, want nil", c.APNS)
	}
}

func TestLoadCompanionMissingRequired(t *testing.T) {
	t.Setenv("COMPANION_PUBLIC_URL", "")
	t.Setenv("VIKUNJA_UPSTREAM_URL", "")
	t.Setenv("COMPANION_MASTER_KEY", "")

	_, err := LoadCompanion()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"COMPANION_PUBLIC_URL", "VIKUNJA_UPSTREAM_URL", "COMPANION_MASTER_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestLoadCompanionBadMasterKey(t *testing.T) {
	setCompanionEnv(t, map[string]string{"COMPANION_MASTER_KEY": "too-short"})

	if _, err := LoadCompanion(); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("err = %v, want 32-bytes complaint", err)
	}
}

func TestLoadCompanionTrailingSlashStripped(t *testing.T) {
	setCompanionEnv(t, map[string]string{"COMPANION_PUBLIC_URL": "https://companion.example.com/"})

	c, err := LoadCompanion()
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicURL != "https://companion.example.com" {
		t.Errorf("PublicURL = %q", c.PublicURL)
	}
}

func TestLoadCompanionUnknownWebhookEvent(t *testing.T) {
	setCompanionEnv(t, map[string]string{"COMPANION_WEBHOOK_EVENTS": "task.overdue,task.bogus"})

	if _, err := LoadCompanion(); err == nil || !strings.Contains(err.Error(), "task.bogus") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadCompanionPartialAPNS(t *testing.T) {
	setCompanionEnv(t, map[string]string{
		"COMPANION_APNS_KEY_PATH": "/keys/AuthKey.p8",
		"COMPANION_APNS_KEY_ID":   "ABC123",
	})

	if _, err := LoadCompanion(); err == nil || !strings.Contains(err.Error(), "APNS") {
		t.Fatalf("err = %v, want partial-APNS complaint", err)
	}
}

func TestLoadCompanionFullAPNS(t *testing.T) {
	setCompanionEnv(t, map[string]string{
		"COMPANION_APNS_KEY_PATH": "/keys/AuthKey.p8",
		"COMPANION_APNS_KEY_ID":   "ABC123",
		"COMPANION_APNS_TEAM_ID":  "TEAM99",
		"COMPANION_APNS_TOPIC":    "com.example.vikunja",
	})

	c, err := LoadCompanion()
	if err != nil {
		t.Fatal(err)
	}
	if c.APNS == nil || c.APNS.Topic != "com.example.vikunja" {
		t.Fatalf("APNS = %+v", c.APNS)
	}
}
