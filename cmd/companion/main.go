// Command companion is the self-hosted transparent reverse proxy to a Vikunja
// instance that also serves its own features under /companion/v1/*
// (docs/COMPANION.md §1–§6).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/seergs/vikunja-companion/internal/companion"
	"github.com/seergs/vikunja-companion/internal/config"
	"github.com/seergs/vikunja-companion/internal/httpx"
	"github.com/seergs/vikunja-companion/internal/proxy"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
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

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database ready", "path", cfg.DBPath)

	vk := vikunja.NewClient(cfg.UpstreamURL, nil)

	// Refuse to start unless the upstream is a reachable Vikunja instance (§8).
	upstreamVersion, err := probeUpstream(vk)
	if err != nil {
		return fmt.Errorf("upstream check failed for %s: %w", cfg.UpstreamURL, err)
	}
	log.Info("upstream reachable", "url", cfg.UpstreamURL, "vikunja_version", upstreamVersion)

	rp, err := proxy.New(cfg.UpstreamURL, log)
	if err != nil {
		return err
	}

	handler := companion.NewRouter(companion.Options{
		Version:       Version,
		VikunjaURL:    cfg.UpstreamURL,
		VikunjaClient: vk,
		Proxy:         rp,
		Logger:        log,
		SeedVersion:   upstreamVersion,
	})

	return httpx.Serve(cfg.ListenAddr, handler, log)
}

// probeUpstream confirms VIKUNJA_UPSTREAM_URL answers GET /api/v1/info with a
// version, returning it.
func probeUpstream(vk *vikunja.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := vk.Info(ctx)
	if err != nil {
		return "", err
	}
	if info.Version == "" {
		return "", fmt.Errorf("/api/v1/info returned no version — not a Vikunja instance?")
	}
	return info.Version, nil
}
