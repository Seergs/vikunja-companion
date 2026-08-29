package relay

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// TokenStore persists the relay's opaque registration tokens (internal/store).
// It holds no PII.
type TokenStore interface {
	MintToken(ctx context.Context) (string, error)
	// ValidToken reports whether token is known, touching its last_seen.
	ValidToken(ctx context.Context, token string) (bool, error)
}

// APNSSender delivers a fully-built APNs payload to a device token.
type APNSSender interface {
	Send(ctx context.Context, deviceToken string, payload []byte, collapseID string) error
}

// ErrBadDeviceToken lets a sender report a token APNs rejected as invalid, so
// the relay can answer 410 Gone (the companion then drops the device).
var ErrBadDeviceToken = errors.New("relay: device token rejected by APNs")

// ServerOptions tunes the relay server.
type ServerOptions struct {
	// PushRatePerSec / PushBurst bound pushes per registration token.
	PushRatePerSec float64
	PushBurst      float64
}

// Server is the relay HTTP surface.
type Server struct {
	tokens  TokenStore
	apns    APNSSender
	limiter *rateLimiter
	log     *slog.Logger
}

// NewServer builds a relay server. Zero-valued options get sane defaults
// (1 push/sec, burst 20 per token).
func NewServer(tokens TokenStore, apns APNSSender, log *slog.Logger, opts ServerOptions) *Server {
	if opts.PushRatePerSec <= 0 {
		opts.PushRatePerSec = 1
	}
	if opts.PushBurst <= 0 {
		opts.PushBurst = 20
	}
	return &Server{
		tokens:  tokens,
		apns:    apns,
		limiter: newRateLimiter(opts.PushRatePerSec, opts.PushBurst),
		log:     log,
	}
}

// Handler returns the relay's router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /relay/v1/register", s.handleRegister)
	mux.HandleFunc("POST /relay/v1/push", s.handlePush)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokens.MintToken(r.Context())
	if err != nil {
		s.log.Error("relay: mint token", "err", err)
		writeErr(w, http.StatusInternalServerError, "could not mint token")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

type pushReq struct {
	APNsToken  string `json:"apns_token"`
	Ciphertext string `json:"ciphertext"` // base64; the relay never decodes it
	CollapseID string `json:"collapse_id"`
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing bearer token")
		return
	}
	ok, err := s.tokens.ValidToken(r.Context(), token)
	if err != nil {
		s.log.Error("relay: validate token", "err", err)
		writeErr(w, http.StatusInternalServerError, "token check failed")
		return
	}
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unknown token")
		return
	}
	if !s.limiter.allow(token) {
		writeErr(w, http.StatusTooManyRequests, "rate limited")
		return
	}

	var req pushReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.APNsToken == "" || req.Ciphertext == "" {
		writeErr(w, http.StatusBadRequest, "apns_token and ciphertext are required")
		return
	}

	payload := buildAPNSPayload(req.Ciphertext)
	if err := s.apns.Send(r.Context(), req.APNsToken, payload, req.CollapseID); err != nil {
		if errors.Is(err, ErrBadDeviceToken) {
			writeErr(w, http.StatusGone, "device token rejected")
			return
		}
		s.log.Warn("relay: apns send failed", "err", err)
		writeErr(w, http.StatusBadGateway, "apns send failed")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// buildAPNSPayload wraps the opaque ciphertext in a content-blind APNs envelope:
// a generic visible fallback plus `e`, which the app's Notification Service
// Extension decrypts and rewrites on-device.
func buildAPNSPayload(ciphertextB64 string) []byte {
	env := map[string]any{
		"aps": map[string]any{
			"mutable-content": 1,
			"alert":           map[string]any{"title": "Vikunja", "body": "New notification"},
			"sound":           "default",
		},
		"e": ciphertextB64,
	}
	b, _ := json.Marshal(env)
	return b
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
