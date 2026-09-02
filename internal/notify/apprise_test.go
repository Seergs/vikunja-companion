package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type capturedReq struct {
	method string
	path   string
	auth   string
	ctype  string
	body   appriseRequest
}

func appriseServer(t *testing.T, status int) (*httptest.Server, *capturedReq) {
	t.Helper()
	got := &capturedReq{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.ctype = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestAppriseSendStateless(t *testing.T) {
	srv, got := appriseServer(t, http.StatusOK)
	a := NewApprise(srv.URL+"/notify", "ntfy://ntfy.sh/topic", "sekret", srv.Client())

	n := Notification{Title: "Overdue tasks", Body: "You have 2 overdue tasks", Deeplink: "https://vk.example/tasks/9", Level: LevelWarning}
	if err := a.Send(context.Background(), 1, n); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got.method != http.MethodPost || got.path != "/notify" {
		t.Errorf("%s %s", got.method, got.path)
	}
	if got.auth != "Bearer sekret" {
		t.Errorf("auth = %q", got.auth)
	}
	if got.ctype != "application/json" {
		t.Errorf("content-type = %q", got.ctype)
	}
	if got.body.URLs != "ntfy://ntfy.sh/topic" {
		t.Errorf("urls = %q", got.body.URLs)
	}
	if got.body.Title != "Overdue tasks" || got.body.Type != "warning" || got.body.Format != "text" {
		t.Errorf("body = %+v", got.body)
	}
	if got.body.Body != "You have 2 overdue tasks\nhttps://vk.example/tasks/9" {
		t.Errorf("body text = %q (URL deeplink not appended?)", got.body.Body)
	}
}

func TestAppriseSendDropsRelativeDeeplink(t *testing.T) {
	srv, got := appriseServer(t, http.StatusOK)
	a := NewApprise(srv.URL+"/notify", "", "", srv.Client())

	n := Notification{Title: "Daily briefing", Body: "You have 3 tasks due in Vikunja today.", Deeplink: "today"}
	if err := a.Send(context.Background(), 1, n); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.body.Body != "You have 3 tasks due in Vikunja today." {
		t.Errorf("relative deeplink leaked into body: %q", got.body.Body)
	}
}

func TestAppriseSendPersistentConfigNoTokenNoURLs(t *testing.T) {
	srv, got := appriseServer(t, http.StatusOK)
	a := NewApprise(srv.URL+"/notify/vikunja", "", "", srv.Client())

	if err := a.Send(context.Background(), 1, Notification{Title: "Today", Body: "3 tasks for today"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.path != "/notify/vikunja" {
		t.Errorf("path = %q", got.path)
	}
	if got.auth != "" {
		t.Errorf("auth set without a token: %q", got.auth)
	}
	if got.body.URLs != "" {
		t.Errorf("urls sent without config: %q", got.body.URLs)
	}
	if got.body.Type != "info" {
		t.Errorf("default type = %q, want info", got.body.Type)
	}
}

func TestAppriseSendNon2xxIsError(t *testing.T) {
	srv, _ := appriseServer(t, http.StatusFailedDependency)
	a := NewApprise(srv.URL+"/notify", "", "", srv.Client())

	err := a.Send(context.Background(), 1, Notification{Body: "x"})
	if err == nil || !strings.Contains(err.Error(), "424") {
		t.Fatalf("err = %v, want a 424 error", err)
	}
	if !strings.Contains(err.Error(), srv.URL+"/notify") {
		t.Errorf("error should name the target URL: %v", err)
	}
}

func TestAppriseSendTransportErrorIsError(t *testing.T) {
	a := NewApprise("http://127.0.0.1:0/notify", "", "", http.DefaultClient)
	if err := a.Send(context.Background(), 1, Notification{Body: "x"}); err == nil {
		t.Fatal("expected a transport error")
	}
}
