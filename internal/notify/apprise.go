package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Apprise is a Sender that POSTs to an apprise-api endpoint the operator runs
// (https://github.com/caronc/apprise-api). It handles both forms:
//
//   - persistent config: apiURL is .../notify/{key}, urls is empty
//   - stateless:         apiURL is .../notify,       urls is the Apprise URL(s)
type Apprise struct {
	apiURL string
	urls   string
	token  string
	client *http.Client
}

// NewApprise builds an Apprise sender. A nil client gets a 10s-timeout default.
func NewApprise(apiURL, urls, token string, client *http.Client) *Apprise {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Apprise{apiURL: apiURL, urls: urls, token: token, client: client}
}

// appriseRequest is apprise-api's JSON notify body.
type appriseRequest struct {
	URLs   string `json:"urls,omitempty"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body"`
	Type   string `json:"type,omitempty"`   // info | success | warning | failure
	Format string `json:"format,omitempty"` // text | markdown | html
}

// Send delivers n to the apprise-api endpoint. userID is ignored in v1 (the
// endpoint is one fleet-wide operator setting).
func (a *Apprise) Send(ctx context.Context, _ int64, n Notification) error {
	body := strings.TrimRight(n.Body, "\n")
	if n.Deeplink != "" {
		body += "\n" + n.Deeplink
	}
	payload, err := json.Marshal(appriseRequest{
		URLs:   a.urls,
		Title:  n.Title,
		Body:   body,
		Type:   appriseType(n.Level),
		Format: "text",
	})
	if err != nil {
		return fmt.Errorf("apprise: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("apprise: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("apprise: POST %s: %w", a.apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		// resp.Request.URL is the final URL after any redirects.
		return fmt.Errorf("apprise: POST %s -> %s: %s", resp.Request.URL, resp.Status, bytes.TrimSpace(snippet))
	}
	return nil
}

func appriseType(l Level) string {
	if l == LevelWarning {
		return "warning"
	}
	return "info"
}
