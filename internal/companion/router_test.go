package companion

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/seergs/vikunja-companion/internal/crypto"
	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// fakeDispatch records what Dispatch was called with.
type fakeDispatch struct {
	mu    sync.Mutex
	calls []dispatchCall
}
type dispatchCall struct {
	userID int64
	notifs []notify.Notification
}

func (f *fakeDispatch) Dispatch(_ context.Context, userID int64, n []notify.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, dispatchCall{userID, n})
	return nil
}

type testEnv struct {
	handler   http.Handler
	store     *store.DB
	cipher    *crypto.Cipher
	dispatch  *fakeDispatch
	infoCalls *int32
	userToken string // a bearer token the fake upstream accepts, mapped to user 1
}

func newTestEnv(t *testing.T, proxy http.Handler) *testEnv {
	t.Helper()
	if proxy == nil {
		proxy = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}

	var infoCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/info":
			atomic.AddInt32(&infoCalls, 1)
			_, _ = w.Write([]byte(`{"version":"v2.5.0"}`))
		case "/api/v1/user":
			if r.Header.Get("Authorization") != "Bearer good-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":1,"username":"tester","settings":{"timezone":"America/New_York"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	db, err := store.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	key := make([]byte, 32)
	cipher, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	disp := &fakeDispatch{}

	h := NewRouter(Options{
		Version:       "1.2.3",
		PublicURL:     "https://companion.example.com",
		VikunjaURL:    "https://vikunja.example.com",
		VikunjaClient: vikunja.NewClient(upstream.URL, upstream.Client()),
		Proxy:         proxy,
		Logger:        testLogger(),
		Store:         db,
		Cipher:        cipher,
		Dispatcher:    disp,
		WebhookEvents: []string{"task.reminder.fired", "task.overdue", "tasks.overdue"},
	})
	return &testEnv{handler: h, store: db, cipher: cipher, dispatch: disp, infoCalls: &infoCalls, userToken: "good-token"}
}

// newTestRouter keeps the older info/proxy tests working.
func newTestRouter(t *testing.T, proxy http.Handler) (http.Handler, *int32) {
	e := newTestEnv(t, proxy)
	return e.handler, e.infoCalls
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
	if body.Vikunja.URL != "https://vikunja.example.com" || body.Vikunja.Version != "v2.5.0" {
		t.Errorf("vikunja = %+v", body.Vikunja)
	}
	if len(body.Features) != 2 || body.Features[0] != "push" || body.Features[1] != "digest" {
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
