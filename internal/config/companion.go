package config

import (
	"errors"
	"fmt"
	"strings"
)

// KnownWebhookEvents is the complete v1 notification surface: the only
// user-directed webhook events Vikunja emits.
var KnownWebhookEvents = []string{
	"task.reminder.fired",
	"task.overdue",
	"tasks.overdue",
}

// Companion is the validated configuration for cmd/companion.
type Companion struct {
	ListenAddr    string
	PublicURL     string
	UpstreamURL   string
	DBPath        string
	MasterKey     []byte
	WebhookEvents []string
	VikunjaToken  string // COMPANION_VIKUNJA_TOKEN; the digest cron acts as this user. Empty -> digest off.
	DigestEnabled bool
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
		ListenAddr:    get("COMPANION_LISTEN_ADDR", ":8080"),
		DBPath:        get("COMPANION_DB_PATH", "/data/companion.db"),
		VikunjaToken:  get("COMPANION_VIKUNJA_TOKEN", ""),
		DigestEnabled: boolDefault("COMPANION_DIGEST_ENABLED", true),
		LogLevel:      get("COMPANION_LOG_LEVEL", "info"),
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

	if events, err := parseWebhookEvents(get("COMPANION_WEBHOOK_EVENTS", strings.Join(KnownWebhookEvents, ","))); err != nil {
		push(err)
	} else {
		c.WebhookEvents = events
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

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
