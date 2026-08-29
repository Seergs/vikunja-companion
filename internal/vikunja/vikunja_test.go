package vikunja

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/info" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("info must not be authenticated, got %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"version":"v0.24.6","extra":"ignored"}`))
	}))
	defer srv.Close()

	info, err := NewClient(srv.URL, srv.Client()).Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "v0.24.6" {
		t.Errorf("Version = %q", info.Version)
	}
}

func TestUserSendsBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte(`{"id":42,"username":"ada"}`))
	}))
	defer srv.Close()

	user, err := NewClient(srv.URL, srv.Client()).User(context.Background(), "tok-123")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.Username != "ada" {
		t.Errorf("user = %+v", user)
	}
}

func TestUserUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"invalid token"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, srv.Client()).User(context.Background(), "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsUnauthorized(err) {
		t.Errorf("IsUnauthorized = false for %v", err)
	}
}

func TestInfoServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("nope"))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, srv.Client()).Info(context.Background())
	if err == nil || IsUnauthorized(err) {
		t.Fatalf("err = %v", err)
	}
}
