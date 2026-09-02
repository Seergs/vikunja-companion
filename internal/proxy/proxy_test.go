package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestProxyForwardsVerbatim(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotHost, gotXFF, gotBody, gotCustom string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		gotHost, gotXFF = r.Host, r.Header.Get("X-Forwarded-Host")
		gotCustom = r.Header.Get("X-Custom")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "hello from upstream")
	}))
	defer upstream.Close()

	p, err := New(upstream.URL, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(p)
	defer front.Close()

	req, _ := http.NewRequest(http.MethodPut, front.URL+"/api/v1/tasks/7?fields=a,b", strings.NewReader("payload"))
	req.Header.Set("X-Custom", "keep-me")
	req.Host = "companion.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")
	switch {
	case gotMethod != http.MethodPut:
		t.Errorf("method = %q", gotMethod)
	case gotPath != "/api/v1/tasks/7":
		t.Errorf("path = %q", gotPath)
	case gotQuery != "fields=a,b":
		t.Errorf("query = %q", gotQuery)
	case gotBody != "payload":
		t.Errorf("body = %q", gotBody)
	case gotCustom != "keep-me":
		t.Errorf("X-Custom = %q", gotCustom)
	case gotHost != upstreamHost:
		t.Errorf("upstream Host = %q, want %q", gotHost, upstreamHost)
	case gotXFF != "companion.example.com":
		t.Errorf("X-Forwarded-Host = %q", gotXFF)
	case resp.StatusCode != http.StatusTeapot:
		t.Errorf("status = %d", resp.StatusCode)
	case resp.Header.Get("X-Upstream") != "yes":
		t.Errorf("missing upstream response header")
	case string(body) != "hello from upstream":
		t.Errorf("body = %q", body)
	}
}

func TestProxyUpstreamDownReturns502(t *testing.T) {
	p, err := New("http://127.0.0.1:1", discardLogger()) // nothing listening
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(p)
	defer front.Close()

	resp, err := http.Get(front.URL + "/api/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestNewRejectsBadURL(t *testing.T) {
	for _, u := range []string{"ftp://x", "://nope", "https://"} {
		if _, err := New(u, discardLogger()); err == nil {
			t.Errorf("New(%q) = nil error", u)
		}
	}
}
