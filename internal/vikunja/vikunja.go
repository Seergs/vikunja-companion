// Package vikunja is the thin HTTP client for Vikunja's public API. All
// Vikunja-version coupling lives here — no schema access, no fork
// (docs/COMPANION.md §1, §3, §6.3).
//
// Fase 1 surface: GET /api/v1/info, GET /api/v1/user (caller identity).
// Fase 2 surface: GET/PUT/POST/DELETE /api/v1/user/settings/webhooks,
// task fetches for the digest cron.
package vikunja

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxBodyBytes caps how much of a Vikunja response body we read into memory.
const maxBodyBytes = 1 << 20

// Client talks to a single Vikunja instance.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient returns a Client for baseURL (trailing slash trimmed). If hc is nil
// a client with a 10s timeout is used.
func NewClient(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), hc: hc}
}

// APIError is returned for any non-2xx response from Vikunja.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	return fmt.Sprintf("vikunja: unexpected status %d: %s", e.StatusCode, body)
}

// IsUnauthorized reports whether err is an APIError with a 401 status — i.e. the
// caller's token is invalid or expired.
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}

// Info is the subset of GET /api/v1/info the companion needs.
//
// NOTE (docs/COMPANION.md §11): the full /api/v1/info shape is not verified
// against a live instance; only `version` is relied on here.
type Info struct {
	Version string `json:"version"`
}

// Info fetches GET /api/v1/info. No authentication required.
func (c *Client) Info(ctx context.Context) (*Info, error) {
	var info Info
	if err := c.do(ctx, http.MethodGet, "/api/v1/info", "", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// User is the subset of GET /api/v1/user identifying the caller (§3).
//
// NOTE (docs/COMPANION.md §11): only `id` is relied on; `username` is kept for
// logging.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// User fetches GET /api/v1/user for the given bearer token. Returns an APIError
// with StatusCode 401 (see IsUnauthorized) when the token is rejected.
func (c *Client) User(ctx context.Context, token string) (*User, error) {
	var user User
	if err := c.do(ctx, http.MethodGet, "/api/v1/user", token, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// do performs a request against the Vikunja API and decodes a JSON response
// into out. token, when non-empty, is sent as a bearer credential.
func (c *Client) do(ctx context.Context, method, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("vikunja: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("vikunja: reading %s %s: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("vikunja: decoding %s %s: %w", method, path, err)
		}
	}
	return nil
}
