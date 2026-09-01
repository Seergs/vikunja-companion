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
	Apprise       Apprise
	DigestEnabled bool
	LogLevel      string
}

// Apprise is the notification-delivery target: an apprise-api endpoint the
// operator runs. Empty APIURL -> notifications are logged, not delivered.
type Apprise struct {
	APIURL string // COMPANION_APPRISE_API_URL, e.g. https://apprise.example/notify or .../notify/{key}
	URLs   string // COMPANION_APPRISE_URLS, comma-separated Apprise service URLs (stateless form)
	Token  string // COMPANION_APPRISE_TOKEN, bearer for the endpoint
}

// Enabled reports whether delivery is configured.
func (a Apprise) Enabled() bool { return a.APIURL != "" }

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

	c.Apprise = Apprise{
		URLs:  get("COMPANION_APPRISE_URLS", ""),
		Token: get("COMPANION_APPRISE_TOKEN", ""),
	}
	if raw := get("COMPANION_APPRISE_API_URL", ""); raw != "" {
		if v, err := parseHTTPURL("COMPANION_APPRISE_API_URL", raw); err != nil {
			push(err)
		} else {
			c.Apprise.APIURL = v
		}
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
