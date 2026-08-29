// Package relay holds both sides of the content-blind APNs relay:
//
//   - Client: the companion uses it to register once and to push sealed
//     ciphertext ({apns_token, ciphertext, collapse_id});
//   - Server: cmd/relay runs it, forwards to APNs with the .p8 key, and stores
//     only opaque registration tokens — no PII, no content.
package relay

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a relay instance from the companion.
type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewClient returns a relay client. token may be empty if the caller will
// Register first. If hc is nil a client with a 15s timeout is used.
func NewClient(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, hc: hc}
}

// Token returns the client's current relay token.
func (c *Client) Token() string { return c.token }

// Register mints a fresh relay token and adopts it as the client's token.
func (c *Client) Register(ctx context.Context) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, "POST", "/relay/v1/register", nil, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("relay: register returned an empty token")
	}
	c.token = out.Token
	return out.Token, nil
}

// PushRequest is one sealed delivery.
type PushRequest struct {
	APNsToken  string
	Ciphertext []byte
	CollapseID string
}

type pushBody struct {
	APNsToken  string `json:"apns_token"`
	Ciphertext string `json:"ciphertext"` // base64 std
	CollapseID string `json:"collapse_id,omitempty"`
}

// Push forwards a sealed payload to the relay, which delivers it to APNs.
func (c *Client) Push(ctx context.Context, req PushRequest) error {
	if c.token == "" {
		return fmt.Errorf("relay: no token; call Register first")
	}
	body := pushBody{
		APNsToken:  req.APNsToken,
		Ciphertext: base64.StdEncoding.EncodeToString(req.Ciphertext),
		CollapseID: req.CollapseID,
	}
	return c.do(ctx, "POST", "/relay/v1/push", body, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var reader io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("relay: marshal %s: %w", path, err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("relay: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("relay: decoding %s response: %w", path, err)
		}
	}
	return nil
}
