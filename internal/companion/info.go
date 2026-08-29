package companion

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/seergs/vikunja-companion/internal/vikunja"
)

// infoFetcher fetches the upstream Vikunja version. Satisfied by *vikunja.Client.
type infoFetcher interface {
	Info(ctx context.Context) (*vikunja.Info, error)
}

// infoResponse is the body of GET /companion/v1/info.
type infoResponse struct {
	Companion struct {
		Version string `json:"version"`
	} `json:"companion"`
	Vikunja struct {
		URL     string `json:"url"`
		Version string `json:"version"`
	} `json:"vikunja"`
	Features []string `json:"features"`
}

// infoHandler serves the unauthenticated capability-discovery endpoint. The
// upstream Vikunja version is cached briefly so a probe storm can't fan out
// into upstream calls.
type infoHandler struct {
	companionVersion string
	vikunjaURL       string
	features         []string
	fetcher          infoFetcher
	ttl              time.Duration
	log              *slog.Logger
	now              func() time.Time

	mu          sync.Mutex
	seenVersion string
	fetchedAt   time.Time
}

func (h *infoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp := infoResponse{Features: h.features}
	resp.Companion.Version = h.companionVersion
	resp.Vikunja.URL = h.vikunjaURL
	resp.Vikunja.Version = h.vikunjaVersion(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Warn("encoding /companion/v1/info", "err", err)
	}
}

// vikunjaVersion returns the cached upstream version, refreshing it when stale.
// A failed refresh keeps the last known value (possibly empty) rather than
// failing the probe.
func (h *infoHandler) vikunjaVersion(ctx context.Context) string {
	h.mu.Lock()
	if h.seenVersion != "" && h.now().Before(h.fetchedAt.Add(h.ttl)) {
		v := h.seenVersion
		h.mu.Unlock()
		return v
	}
	h.mu.Unlock()

	info, err := h.fetcher.Info(ctx)
	if err != nil {
		h.log.Warn("refreshing upstream version for /companion/v1/info", "err", err)
		h.mu.Lock()
		v := h.seenVersion
		h.mu.Unlock()
		return v
	}

	h.mu.Lock()
	h.seenVersion = info.Version
	h.fetchedAt = h.now()
	h.mu.Unlock()
	return info.Version
}
