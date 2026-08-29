// Command relay is the content-blind APNs forwarder, operated once by the
// project maintainer. It holds the Apple .p8 key and sees only an opaque device
// token plus ciphertext.
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
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	// TODO(fase-2): POST /relay/v1/register, POST /relay/v1/push (per-token
	// rate limited) -> APNs forwarder using the .p8 key.

	return httpx.Serve(cfg.ListenAddr, mux, log)
}
