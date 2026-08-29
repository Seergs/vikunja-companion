package relay

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type memTokens struct {
	mu     sync.Mutex
	tokens map[string]bool
	n      int
	err    error
}

func newMemTokens(seed ...string) *memTokens {
	m := &memTokens{tokens: map[string]bool{}}
	for _, s := range seed {
		m.tokens[s] = true
	}
	return m
}

func (m *memTokens) MintToken(context.Context) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n++
	tok := "rt_" + strings.Repeat("a", m.n)
	m.tokens[tok] = true
	return tok, nil
}

func (m *memTokens) ValidToken(_ context.Context, t string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tokens[t], nil
}

type fakeAPNS struct {
	mu   sync.Mutex
	sent []sent
	err  error
}
type sent struct {
	token, collapse string
	payload         []byte
}

func (f *fakeAPNS) Send(_ context.Context, token string, payload []byte, collapse string) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sent{token, collapse, payload})
	return nil
}

func newTestServer(t *testing.T, tokens TokenStore, apns APNSSender) *httptest.Server {
	t.Helper()
	s := NewServer(tokens, apns, discardLog(), ServerOptions{PushRatePerSec: 1000, PushBurst: 1000})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func TestServerRegisterThenPush(t *testing.T) {
	apns := &fakeAPNS{}
	srv := newTestServer(t, newMemTokens(), apns)

	// register
	resp, _ := http.Post(srv.URL+"/relay/v1/register", "application/json", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status %d", resp.StatusCode)
	}
	var reg struct{ Token string }
	json.NewDecoder(resp.Body).Decode(&reg)
	if reg.Token == "" {
		t.Fatal("no token")
	}

	// push
	body := `{"apns_token":"devtok","ciphertext":"AAECaGVsbG8=","collapse_id":"c1"}`
	req, _ := http.NewRequest("POST", srv.URL+"/relay/v1/push", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+reg.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("push status %d: %s", resp.StatusCode, b)
	}

	if len(apns.sent) != 1 {
		t.Fatalf("apns got %d sends", len(apns.sent))
	}
	s := apns.sent[0]
	if s.token != "devtok" || s.collapse != "c1" {
		t.Errorf("sent = %+v", s)
	}
	var env map[string]any
	json.Unmarshal(s.payload, &env)
	if env["e"] != "AAECaGVsbG8=" {
		t.Errorf("envelope e = %v", env["e"])
	}
	aps, _ := env["aps"].(map[string]any)
	if aps["mutable-content"].(float64) != 1 {
		t.Errorf("aps = %v", aps)
	}
}

func TestServerPushRejectsUnknownToken(t *testing.T) {
	srv := newTestServer(t, newMemTokens("good"), &fakeAPNS{})
	req, _ := http.NewRequest("POST", srv.URL+"/relay/v1/push",
		strings.NewReader(`{"apns_token":"d","ciphertext":"eA=="}`))
	req.Header.Set("Authorization", "Bearer bad")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestServerPushMissingAuth(t *testing.T) {
	srv := newTestServer(t, newMemTokens(), &fakeAPNS{})
	resp, _ := http.Post(srv.URL+"/relay/v1/push", "application/json",
		strings.NewReader(`{"apns_token":"d","ciphertext":"eA=="}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestServerPushBadDeviceTokenIsGone(t *testing.T) {
	srv := newTestServer(t, newMemTokens("t"), &fakeAPNS{err: ErrBadDeviceToken})
	req, _ := http.NewRequest("POST", srv.URL+"/relay/v1/push",
		strings.NewReader(`{"apns_token":"d","ciphertext":"eA=="}`))
	req.Header.Set("Authorization", "Bearer t")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status %d, want 410", resp.StatusCode)
	}
}

func TestServerPushValidatesBody(t *testing.T) {
	srv := newTestServer(t, newMemTokens("t"), &fakeAPNS{})
	for _, body := range []string{`{}`, `{"apns_token":"d"}`, `not json`} {
		req, _ := http.NewRequest("POST", srv.URL+"/relay/v1/push", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer t")
		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q -> status %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Now()
	l := newRateLimiter(1, 3)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.allow("k") {
			t.Fatalf("call %d should be allowed (burst 3)", i)
		}
	}
	if l.allow("k") {
		t.Fatal("4th call should be limited")
	}
	if !l.allow("other") {
		t.Fatal("different key has its own bucket")
	}

	now = now.Add(2 * time.Second) // refill 2 tokens
	if !l.allow("k") || !l.allow("k") {
		t.Fatal("should have refilled 2 tokens")
	}
	if l.allow("k") {
		t.Fatal("only 2 tokens should have refilled")
	}
}
