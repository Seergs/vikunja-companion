// Command companion is the self-hosted transparent reverse proxy to a Vikunja
// instance that also serves its own features under /companion/v1/*.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Embed the IANA timezone database: CGO-free distroless builds have no
	// system zoneinfo, so time.LoadLocation needs this for the digest cron.
	_ "time/tzdata"

	"github.com/seergs/vikunja-companion/internal/companion"
	"github.com/seergs/vikunja-companion/internal/config"
	"github.com/seergs/vikunja-companion/internal/crypto"
	"github.com/seergs/vikunja-companion/internal/digest"
	"github.com/seergs/vikunja-companion/internal/httpx"
	"github.com/seergs/vikunja-companion/internal/notify"
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
	envFile, err := config.LoadDotenv()
	if err != nil {
		return err
	}

	cfg, err := config.LoadCompanion()
	if err != nil {
		return err
	}

	log := httpx.Logger(cfg.LogLevel)
	slog.SetDefault(log)
	if envFile != "" {
		log.Info("loaded local env file", "path", envFile)
	}
	log.Info("starting companion",
		"version", Version,
		"listen", cfg.ListenAddr,
		"upstream", cfg.UpstreamURL,
		"public_url", cfg.PublicURL,
		"webhook_events", cfg.WebhookEvents,
	)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database ready", "path", cfg.DBPath)

	cipher, err := crypto.NewCipher(cfg.MasterKey)
	if err != nil {
		return err
	}

	vk := vikunja.NewClient(cfg.UpstreamURL, nil)

	// Refuse to start unless the upstream is a reachable Vikunja instance.
	upstreamVersion, err := probeUpstream(vk)
	if err != nil {
		return fmt.Errorf("upstream check failed for %s: %w", cfg.UpstreamURL, err)
	}
	log.Info("upstream reachable", "url", cfg.UpstreamURL, "vikunja_version", upstreamVersion)

	// TODO: the Apprise Sender is not built yet. Until it lands, notifications
	// are logged instead of delivered.
	dispatcher := notify.New(db, logSender{log}, log)

	rp, err := proxy.New(cfg.UpstreamURL, log)
	if err != nil {
		return err
	}

	ctx, stop := httpx.SignalContext()
	defer stop()

	// The digest cron acts as one user: whoever COMPANION_VIKUNJA_TOKEN belongs
	// to. A bad/absent token just disables the digest — the webhook path does
	// not need it.
	digestUserID := resolveDigestUser(vk, cfg.VikunjaToken, log)
	digestRunner := digest.NewRunner(db, vk, dispatcher, cfg.VikunjaToken, digestUserID, time.Now, cfg.DigestEnabled, log)
	go digestRunner.Run(ctx)

	handler := companion.NewRouter(companion.Options{
		Version:       Version,
		PublicURL:     cfg.PublicURL,
		VikunjaURL:    cfg.UpstreamURL,
		VikunjaClient: vk,
		Proxy:         rp,
		Logger:        log,
		SeedVersion:   upstreamVersion,
		Store:         db,
		Cipher:        cipher,
		Dispatcher:    dispatcher,
		WebhookEvents: cfg.WebhookEvents,
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
		return "", errors.New("/api/v1/info returned no version — not a Vikunja instance?")
	}
	return info.Version, nil
}

// resolveDigestUser looks up which Vikunja user COMPANION_VIKUNJA_TOKEN belongs
// to. Returns 0 (digest disabled) when the token is empty or Vikunja rejects it.
func resolveDigestUser(vk *vikunja.Client, token string, log *slog.Logger) int64 {
	if token == "" {
		log.Info("COMPANION_VIKUNJA_TOKEN not set — morning digest disabled")
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, err := vk.User(ctx, token)
	if err != nil {
		log.Warn("COMPANION_VIKUNJA_TOKEN rejected by Vikunja — morning digest disabled", "err", err)
		return 0
	}
	log.Info("morning digest enabled", "user", u.ID, "username", u.Username)
	return u.ID
}

// logSender is a placeholder notify.Sender: it logs each notification instead
// of delivering it. It is replaced by the Apprise sender once that lands.
type logSender struct{ log *slog.Logger }

func (s logSender) Send(_ context.Context, userID int64, n notify.Notification) error {
	s.log.Warn("notification not delivered — no Apprise sender is implemented yet",
		"user", userID, "title", n.Title, "body", n.Body)
	return nil
}
