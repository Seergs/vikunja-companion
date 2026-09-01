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
	if len(c.MasterKey) != 32 {
		t.Errorf("MasterKey length = %d, want 32", len(c.MasterKey))
	}
	if len(c.WebhookEvents) != len(KnownWebhookEvents) {
		t.Errorf("WebhookEvents = %v", c.WebhookEvents)
	}
	if c.Apprise.Enabled() {
		t.Errorf("Apprise should be disabled by default, got %+v", c.Apprise)
	}
}

func TestLoadCompanionApprise(t *testing.T) {
	setCompanionEnv(t, map[string]string{
		"COMPANION_APPRISE_API_URL": "https://apprise.example.com/notify/vikunja/",
		"COMPANION_APPRISE_URLS":    "ntfy://ntfy.sh/topic",
		"COMPANION_APPRISE_TOKEN":   "sekret",
	})

	c, err := LoadCompanion()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Apprise.Enabled() {
		t.Fatal("Apprise should be enabled")
	}
	if c.Apprise.APIURL != "https://apprise.example.com/notify/vikunja" {
		t.Errorf("APIURL = %q (trailing slash not stripped?)", c.Apprise.APIURL)
	}
	if c.Apprise.URLs != "ntfy://ntfy.sh/topic" || c.Apprise.Token != "sekret" {
		t.Errorf("Apprise = %+v", c.Apprise)
	}
}

func TestLoadCompanionAppriseBadURL(t *testing.T) {
	setCompanionEnv(t, map[string]string{"COMPANION_APPRISE_API_URL": "not-a-url"})

	if _, err := LoadCompanion(); err == nil || !strings.Contains(err.Error(), "COMPANION_APPRISE_API_URL") {
		t.Fatalf("err = %v, want COMPANION_APPRISE_API_URL complaint", err)
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
