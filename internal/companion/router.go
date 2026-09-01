package companion

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/seergs/vikunja-companion/internal/crypto"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

const (
	defaultIdentityTTL = 5 * time.Minute
	defaultInfoTTL     = 1 * time.Minute
)

// v1Features is the capability list advertised by /companion/v1/info. "push"
// means the companion can deliver task notifications to a channel the user
// configures; "digest" gates the morning-briefing settings UI in the app.
var v1Features = []string{"push", "digest"}

// Options configures NewRouter.
type Options struct {
	Version       string // companion build version
	PublicURL     string // companion's own public base URL
	VikunjaURL    string // upstream URL, echoed in /companion/v1/info
	VikunjaClient *vikunja.Client
	Proxy         http.Handler // catch-all for everything not under /companion/
	Logger        *slog.Logger
	SeedVersion   string // upstream version from the startup probe

	Store         *store.DB
	Cipher        *crypto.Cipher
	Dispatcher    dispatcher
	WebhookEvents []string // operator ceiling (COMPANION_WEBHOOK_EVENTS)

	IdentityTTL time.Duration // 0 -> default
	InfoTTL     time.Duration // 0 -> default
}

// NewRouter returns the companion's top-level HTTP handler: /companion/v1/*
// feature routes, with everything else proxied verbatim to Vikunja.
func NewRouter(opts Options) http.Handler {
	switch {
	case opts.Proxy == nil:
		panic("companion: NewRouter requires a non-nil Proxy")
	case opts.Store == nil || opts.Cipher == nil || opts.Dispatcher == nil:
		panic("companion: NewRouter requires Store, Cipher, and Dispatcher")
	}
	if opts.IdentityTTL == 0 {
		opts.IdentityTTL = defaultIdentityTTL
	}
	if opts.InfoTTL == 0 {
		opts.InfoTTL = defaultInfoTTL
	}

	info := &infoHandler{
		companionVersion: opts.Version,
		vikunjaURL:       opts.VikunjaURL,
		features:         v1Features,
		fetcher:          opts.VikunjaClient,
		ttl:              opts.InfoTTL,
		log:              opts.Logger,
		now:              time.Now,
		seenVersion:      opts.SeedVersion,
		fetchedAt:        time.Now(),
	}

	a := &api{
		store:         opts.Store,
		cipher:        opts.Cipher,
		dispatch:      opts.Dispatcher,
		userSettings:  opts.VikunjaClient,
		identity:      NewIdentityCache(opts.VikunjaClient, opts.IdentityTTL),
		webhookTarget: strings.TrimRight(opts.PublicURL, "/") + "/companion/v1/webhooks/vikunja",
		webhookEvents: opts.WebhookEvents,
		log:           opts.Logger,
	}

	// Routes under /companion/. Anything unmatched here 404s (the app reads a
	// 404 as "no companion") — it must never fall through to the proxy.
	m := http.NewServeMux()
	m.HandleFunc("GET /companion/v1/info", info.ServeHTTP)
	m.HandleFunc("GET /companion/v1/webhook", a.getWebhook)
	m.HandleFunc("POST /companion/v1/webhooks/vikunja", a.inboundWebhook)
	m.HandleFunc("GET /companion/v1/settings", a.getSettings)
	m.HandleFunc("PUT /companion/v1/settings", a.putSettings)

	root := http.NewServeMux()
	root.Handle("/companion/", m)
	root.HandleFunc("GET /healthz", healthz)
	root.Handle("/", opts.Proxy)
	return root
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok\n"))
}
