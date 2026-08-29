// Command companion is the self-hosted transparent reverse proxy to a Vikunja
// instance that also serves its own features under /companion/v1/*
// (docs/COMPANION.md §1–§6).
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/seergs/vikunja-companion/internal/config"
	"github.com/seergs/vikunja-companion/internal/httpx"
)

// Version is injected at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadCompanion()
	if err != nil {
		return err
	}

	log := httpx.Logger(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting companion",
		"version", Version,
		"listen", cfg.ListenAddr,
		"upstream", cfg.UpstreamURL,
		"public_url", cfg.PublicURL,
		"webhook_events", cfg.WebhookEvents,
		"byo_apns", cfg.APNS != nil,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	// TODO(fase-1): mount internal/proxy for everything not under /companion/,
	// and the /companion/v1/* routes (info, devices, settings, webhooks/vikunja).

	return httpx.Serve(cfg.ListenAddr, mux, log)
}
