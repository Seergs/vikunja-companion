package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seergs/vikunja-companion/internal/webhook"
)

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestGetWebhookIssuesStableSecret(t *testing.T) {
	e := newTestEnv(t, nil)

	rec := do(t, e.handler, "GET", "/companion/v1/webhook", e.userToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var first webhookInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if first.TargetURL != "https://companion.example.com/companion/v1/webhooks/vikunja" {
		t.Errorf("target_url = %q", first.TargetURL)
	}
	if len(first.Secret) != 64 || len(first.Events) != 3 {
		t.Errorf("resp = %+v", first)
	}
	if first.LastDeliveryAt != nil {
		t.Errorf("last_delivery_at should be null, got %v", first.LastDeliveryAt)
	}

	// Second call returns the SAME secret (so the user's Vikunja config stays valid).
	rec2 := do(t, e.handler, "GET", "/companion/v1/webhook", e.userToken, "")
	var second webhookInfoResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &second); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if second.Secret != first.Secret {
		t.Errorf("secret changed between calls: %q -> %q", first.Secret, second.Secret)
	}
}

func TestGetWebhookRequiresAuth(t *testing.T) {
	e := newTestEnv(t, nil)
	if rec := do(t, e.handler, "GET", "/companion/v1/webhook", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
	if rec := do(t, e.handler, "GET", "/companion/v1/webhook", "bad-token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestInboundWebhookFullFlow(t *testing.T) {
	e := newTestEnv(t, nil)

	// 1. user sets up push -> gets a secret
	rec := do(t, e.handler, "GET", "/companion/v1/webhook", e.userToken, "")
	var setup webhookInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &setup); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// 2. Vikunja delivers a signed tasks.overdue
	eventBody := `{"event_name":"tasks.overdue","time":"2026-08-29T09:00:00Z","data":{` +
		`"user":{"id":1,"username":"tester"},` +
		`"tasks":[{"id":5,"title":"A"},{"id":9,"title":"B"}],` +
		`"projects":{"1":{"id":1,"title":"Inbox"}}}}`
	sig := webhook.Sign([]byte(eventBody), setup.Secret)

	r := httptest.NewRequest("POST", "/companion/v1/webhooks/vikunja", strings.NewReader(eventBody))
	r.Header.Set(webhook.SignatureHeader, sig)
	wr := httptest.NewRecorder()
	e.handler.ServeHTTP(wr, r)
	if wr.Code != http.StatusOK {
		t.Fatalf("inbound webhook: %d %s", wr.Code, wr.Body)
	}

	// 3. a notification was dispatched for the user
	if len(e.dispatch.calls) != 1 {
		t.Fatalf("dispatch called %d times", len(e.dispatch.calls))
	}
	call := e.dispatch.calls[0]
	if call.userID != 1 {
		t.Errorf("dispatched for user %d, want 1", call.userID)
	}
	if len(call.notifs) != 1 || call.notifs[0].Body != "You have 2 overdue tasks" {
		t.Errorf("notifs = %+v", call.notifs)
	}

	// 4. last_delivery_at is now set
	rec = do(t, e.handler, "GET", "/companion/v1/webhook", e.userToken, "")
	var after webhookInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if after.LastDeliveryAt == nil {
		t.Error("last_delivery_at still null after a delivery")
	}
}

func TestInboundWebhookRejectsBadSignature(t *testing.T) {
	e := newTestEnv(t, nil)
	do(t, e.handler, "GET", "/companion/v1/webhook", e.userToken, "") // create a secret

	body := `{"event_name":"task.overdue","time":"2026-08-29T09:00:00Z","data":{}}`
	r := httptest.NewRequest("POST", "/companion/v1/webhooks/vikunja", strings.NewReader(body))
	r.Header.Set(webhook.SignatureHeader, "deadbeef")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
	if len(e.dispatch.calls) != 0 {
		t.Error("dispatched despite bad signature")
	}
}

func TestInboundWebhookUnsupportedEventIs200(t *testing.T) {
	e := newTestEnv(t, nil)
	rec := do(t, e.handler, "GET", "/companion/v1/webhook", e.userToken, "")
	var setup webhookInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &setup); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	body := `{"event_name":"task.created","time":"2026-08-29T09:00:00Z","data":{}}`
	r := httptest.NewRequest("POST", "/companion/v1/webhooks/vikunja", strings.NewReader(body))
	r.Header.Set(webhook.SignatureHeader, webhook.Sign([]byte(body), setup.Secret))
	wr := httptest.NewRecorder()
	e.handler.ServeHTTP(wr, r)

	if wr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (ignored)", wr.Code)
	}
	if len(e.dispatch.calls) != 0 {
		t.Error("unsupported event should not dispatch")
	}
}
