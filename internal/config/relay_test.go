package config

import (
	"strings"
	"testing"
)

func TestLoadRelayOK(t *testing.T) {
	t.Setenv("RELAY_APNS_KEY_PATH", "/keys/AuthKey.p8")
	t.Setenv("RELAY_APNS_KEY_ID", "ABC123")
	t.Setenv("RELAY_APNS_TEAM_ID", "TEAM99")
	t.Setenv("RELAY_APNS_TOPIC", "com.example.vikunja")

	r, err := LoadRelay()
	if err != nil {
		t.Fatalf("LoadRelay: %v", err)
	}
	if r.ListenAddr != ":8081" {
		t.Errorf("ListenAddr = %q, want :8081", r.ListenAddr)
	}
	if r.DBPath != "/data/relay.db" {
		t.Errorf("DBPath = %q", r.DBPath)
	}
	if r.APNS.KeyID != "ABC123" {
		t.Errorf("APNS.KeyID = %q", r.APNS.KeyID)
	}
}

func TestLoadRelayMissingAll(t *testing.T) {
	for _, k := range []string{"RELAY_APNS_KEY_PATH", "RELAY_APNS_KEY_ID", "RELAY_APNS_TEAM_ID", "RELAY_APNS_TOPIC"} {
		t.Setenv(k, "")
	}

	_, err := LoadRelay()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"RELAY_APNS_KEY_PATH", "RELAY_APNS_TOPIC"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}
