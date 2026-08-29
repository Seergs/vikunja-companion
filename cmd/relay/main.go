// Command relay is the content-blind APNs forwarder, operated once by the
// project maintainer. It holds the Apple .p8 key and sees only an opaque device
// token plus ciphertext.
package main

import (
	"log/slog"
	"os"

	"github.com/seergs/vikunja-companion/internal/config"
	"github.com/seergs/vikunja-companion/internal/httpx"
	"github.com/seergs/vikunja-companion/internal/relay"
	"github.com/seergs/vikunja-companion/internal/store"
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

	cfg, err := config.LoadRelay()
	if err != nil {
		return err
	}

	log := httpx.Logger(cfg.LogLevel)
	slog.SetDefault(log)
	if envFile != "" {
		log.Info("loaded local env file", "path", envFile)
	}
	log.Info("starting relay",
		"version", Version,
		"listen", cfg.ListenAddr,
		"apns_topic", cfg.APNS.Topic,
		"apns_key_id", cfg.APNS.KeyID,
		"apns_sandbox", cfg.APNSSandbox,
	)

	db, err := store.OpenRelay(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database ready", "path", cfg.DBPath)

	apns, err := relay.NewAPNS(relay.APNSConfig{
		KeyPath: cfg.APNS.KeyPath,
		KeyID:   cfg.APNS.KeyID,
		TeamID:  cfg.APNS.TeamID,
		Topic:   cfg.APNS.Topic,
		Sandbox: cfg.APNSSandbox,
	})
	if err != nil {
		return err
	}

	srv := relay.NewServer(db, apns, log, relay.ServerOptions{})
	return httpx.Serve(cfg.ListenAddr, srv.Handler(), log)
}
