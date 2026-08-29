package companion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/seergs/vikunja-companion/internal/vikunja"
)

type fakeResolver struct {
	calls int
	user  *vikunja.User
	err   error
}

func (f *fakeResolver) User(_ context.Context, _ string) (*vikunja.User, error) {
	f.calls++
	return f.user, f.err
}

func TestIdentityCacheHitsUpstreamOnce(t *testing.T) {
	f := &fakeResolver{user: &vikunja.User{ID: 5, Username: "ada"}}
	c := NewIdentityCache(f, time.Minute)

	for i := 0; i < 3; i++ {
		id, err := c.Resolve(context.Background(), "tok")
		if err != nil {
			t.Fatal(err)
		}
		if id.UserID != 5 || id.Username != "ada" {
			t.Fatalf("id = %+v", id)
		}
	}
	if f.calls != 1 {
		t.Errorf("resolver called %d times, want 1", f.calls)
	}
}

func TestIdentityCacheExpires(t *testing.T) {
	f := &fakeResolver{user: &vikunja.User{ID: 5}}
	c := NewIdentityCache(f, time.Minute)
	now := time.Now()
	c.now = func() time.Time { return now }

	if _, err := c.Resolve(context.Background(), "tok"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := c.Resolve(context.Background(), "tok"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Errorf("resolver called %d times, want 2", f.calls)
	}
}

func TestIdentityCacheDistinctTokens(t *testing.T) {
	f := &fakeResolver{user: &vikunja.User{ID: 5}}
	c := NewIdentityCache(f, time.Minute)
	c.Resolve(context.Background(), "a")
	c.Resolve(context.Background(), "b")
	if f.calls != 2 {
		t.Errorf("calls = %d, want 2", f.calls)
	}
}

func TestIdentityCacheNoToken(t *testing.T) {
	c := NewIdentityCache(&fakeResolver{}, time.Minute)
	if _, err := c.Resolve(context.Background(), ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestIdentityCachePropagatesUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewIdentityCache(vikunja.NewClient(srv.URL, srv.Client()), time.Minute)
	_, err := c.Resolve(context.Background(), "bad")
	if !vikunja.IsUnauthorized(err) {
		t.Fatalf("err = %v, want unauthorized", err)
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc": "abc",
		"bearer xyz": "xyz",
		"BEARER  p ": "p",
		"Basic zzz":  "",
		"":           "",
		"Bearer":     "",
	}
	for header, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if got := bearerToken(r); got != want {
			t.Errorf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
