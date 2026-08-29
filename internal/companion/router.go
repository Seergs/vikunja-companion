package companion

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/seergs/vikunja-companion/internal/vikunja"
)

const (
	defaultIdentityTTL = 5 * time.Minute
	defaultInfoTTL     = 1 * time.Minute
)

// v1Features is the capability list advertised by /companion/v1/info.
var v1Features = []string{"push"}

// Options configures NewRouter.
type Options struct {
	Version       string // companion build version
	VikunjaURL    string // upstream URL, echoed in /companion/v1/info
	VikunjaClient *vikunja.Client
	Proxy         http.Handler // catch-all for everything not under /companion/
	Logger        *slog.Logger
	SeedVersion   string // upstream version from the startup probe

	IdentityTTL time.Duration // 0 -> default
	InfoTTL     time.Duration // 0 -> default
}

// NewRouter returns the companion's top-level HTTP handler: /companion/v1/*
// feature routes, with everything else proxied verbatim to Vikunja.
func NewRouter(opts Options) http.Handler {
	if opts.Proxy == nil {
		panic("companion: NewRouter requires a non-nil Proxy")
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

	// Routes under /companion/. Anything unmatched here 404s (the app reads a
	// 404 as "no companion") — it must never fall through to the proxy.
	companionMux := http.NewServeMux()
	companionMux.HandleFunc("GET /companion/v1/info", info.ServeHTTP)

	root := http.NewServeMux()
	root.Handle("/companion/", companionMux)
	root.HandleFunc("GET /healthz", healthz)
	root.Handle("/", opts.Proxy)
	return root
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok\n"))
}
