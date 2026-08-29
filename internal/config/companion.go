package config

import (
	"errors"
	"fmt"
	"strings"
)

// DefaultRelayURL is the project-operated relay used when COMPANION_RELAY_URL is
// unset. Placeholder until the relay is deployed.
const DefaultRelayURL = "https://relay.vikunja-companion.invalid"

// KnownWebhookEvents is the complete v1 push surface: the only user-directed
// webhook events Vikunja emits.
var KnownWebhookEvents = []string{
	"task.reminder.fired",
	"task.overdue",
	"tasks.overdue",
}

// APNS holds a bring-your-own Apple Push credential set. When set, the companion
// talks to APNs directly and bypasses the relay.
type APNS struct {
	KeyPath string
	KeyID   string
	TeamID  string
	Topic   string
}

// Companion is the validated configuration for cmd/companion.
type Companion struct {
	ListenAddr    string
	PublicURL     string
	UpstreamURL   string
	DBPath        string
	MasterKey     []byte
	RelayURL      string
	RelayToken    string
	WebhookEvents []string
	APNS          *APNS
	LogLevel      string
}

// LoadCompanion reads and validates the companion configuration from the
// environment.
func LoadCompanion() (*Companion, error) {
	var errs []error
	push := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	c := &Companion{
		ListenAddr: get("COMPANION_LISTEN_ADDR", ":8080"),
		DBPath:     get("COMPANION_DB_PATH", "/data/companion.db"),
		RelayToken: get("COMPANION_RELAY_TOKEN", ""),
		LogLevel:   get("COMPANION_LOG_LEVEL", "info"),
	}

	if raw, err := required("COMPANION_PUBLIC_URL"); err != nil {
		push(err)
	} else if v, err := parseHTTPURL("COMPANION_PUBLIC_URL", raw); err != nil {
		push(err)
	} else {
		c.PublicURL = v
	}

	if raw, err := required("VIKUNJA_UPSTREAM_URL"); err != nil {
		push(err)
	} else if v, err := parseHTTPURL("VIKUNJA_UPSTREAM_URL", raw); err != nil {
		push(err)
	} else {
		c.UpstreamURL = v
	}

	if raw, err := required("COMPANION_MASTER_KEY"); err != nil {
		push(err)
	} else if k, err := decodeKey("COMPANION_MASTER_KEY", raw); err != nil {
		push(err)
	} else {
		c.MasterKey = k
	}

	if v, err := parseHTTPURL("COMPANION_RELAY_URL", get("COMPANION_RELAY_URL", DefaultRelayURL)); err != nil {
		push(err)
	} else {
		c.RelayURL = v
	}

	if events, err := parseWebhookEvents(get("COMPANION_WEBHOOK_EVENTS", strings.Join(KnownWebhookEvents, ","))); err != nil {
		push(err)
	} else {
		c.WebhookEvents = events
	}

	if apns, err := loadAPNS(); err != nil {
		push(err)
	} else {
		c.APNS = apns
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

func parseWebhookEvents(raw string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		e := strings.TrimSpace(part)
		if e == "" {
			continue
		}
		if !contains(KnownWebhookEvents, e) {
			return nil, fmt.Errorf("COMPANION_WEBHOOK_EVENTS: unknown event %q", e)
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("COMPANION_WEBHOOK_EVENTS: at least one event is required")
	}
	return out, nil
}

// loadAPNS returns a fully-populated *APNS, nil if none of the vars are set, or
// an error if the set is partial.
func loadAPNS() (*APNS, error) {
	keys := []string{
		"COMPANION_APNS_KEY_PATH",
		"COMPANION_APNS_KEY_ID",
		"COMPANION_APNS_TEAM_ID",
		"COMPANION_APNS_TOPIC",
	}
	vals := make([]string, len(keys))
	set := 0
	for i, k := range keys {
		if v, ok := lookup(k); ok && v != "" {
			vals[i] = v
			set++
		}
	}
	if set == 0 {
		return nil, nil
	}
	if set < len(keys) {
		return nil, fmt.Errorf("COMPANION_APNS_*: all of %s must be set together", strings.Join(keys, ", "))
	}
	return &APNS{KeyPath: vals[0], KeyID: vals[1], TeamID: vals[2], Topic: vals[3]}, nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
