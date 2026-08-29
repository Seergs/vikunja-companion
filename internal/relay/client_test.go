package relay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/relay/v1/register" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"token":"rt_abc123"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", srv.Client())
	tok, err := c.Register(context.Background())
	if err != nil || tok != "rt_abc123" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if c.Token() != "rt_abc123" {
		t.Errorf("client did not adopt token: %q", c.Token())
	}
}

func TestPushEncodesAndAuthenticates(t *testing.T) {
	var got pushBody
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "rt_xyz", srv.Client())
	err := c.Push(context.Background(), PushRequest{
		APNsToken:  "devicetoken",
		Ciphertext: []byte{0x00, 0x01, 0x02, 0xff},
		CollapseID: "batch:1:2026-08-29",
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer rt_xyz" {
		t.Errorf("auth = %q", auth)
	}
	if got.APNsToken != "devicetoken" || got.CollapseID != "batch:1:2026-08-29" {
		t.Errorf("body = %+v", got)
	}
	raw, _ := base64.StdEncoding.DecodeString(got.Ciphertext)
	if len(raw) != 4 || raw[3] != 0xff {
		t.Errorf("ciphertext round-trip wrong: %v", raw)
	}
}

func TestPushWithoutTokenFails(t *testing.T) {
	c := NewClient("http://unused", "", nil)
	if err := c.Push(context.Background(), PushRequest{APNsToken: "t"}); err == nil {
		t.Fatal("expected error without token")
	}
}

func TestPushSurfacesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "rt", srv.Client())
	err := c.Push(context.Background(), PushRequest{APNsToken: "t", Ciphertext: []byte("x")})
	if err == nil {
		t.Fatal("expected error")
	}
}
