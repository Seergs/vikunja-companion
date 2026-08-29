package companion

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/seergs/vikunja-companion/internal/vikunja"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newTestRouter(t *testing.T, proxy http.Handler) (http.Handler, *int32) {
	t.Helper()
	if proxy == nil {
		proxy = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
	var infoCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&infoCalls, 1)
		w.Write([]byte(`{"version":"v0.24.6"}`))
	}))
	t.Cleanup(upstream.Close)

	h := NewRouter(Options{
		Version:       "1.2.3",
		VikunjaURL:    "https://vikunja.example.com",
		VikunjaClient: vikunja.NewClient(upstream.URL, upstream.Client()),
		Proxy:         proxy,
		Logger:        testLogger(),
	})
	return h, &infoCalls
}

func TestRouterInfo(t *testing.T) {
	h, _ := newTestRouter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("proxy hit for %s", r.URL.Path)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/companion/v1/info", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var body infoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Companion.Version != "1.2.3" {
		t.Errorf("companion.version = %q", body.Companion.Version)
	}
	if body.Vikunja.URL != "https://vikunja.example.com" || body.Vikunja.Version != "v0.24.6" {
		t.Errorf("vikunja = %+v", body.Vikunja)
	}
	if len(body.Features) != 1 || body.Features[0] != "push" {
		t.Errorf("features = %v", body.Features)
	}
}

func TestRouterInfoCachesUpstreamVersion(t *testing.T) {
	h, infoCalls := newTestRouter(t, nil)
	for i := 0; i < 4; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/companion/v1/info", nil))
	}
	if got := atomic.LoadInt32(infoCalls); got != 1 {
		t.Errorf("upstream /info called %d times, want 1 (cached)", got)
	}
}

func TestRouterUnknownCompanionPath404(t *testing.T) {
	h, _ := newTestRouter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("proxy must not see %s", r.URL.Path)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/companion/v1/bogus", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestRouterProxiesEverythingElse(t *testing.T) {
	var proxied string
	h, _ := newTestRouter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if proxied != "/api/v1/tasks" {
		t.Errorf("proxied path = %q", proxied)
	}
}

func TestRouterHealthz(t *testing.T) {
	h, _ := newTestRouter(t, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Errorf("healthz: %d %q", rec.Code, rec.Body.String())
	}
}
