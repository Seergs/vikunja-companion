package webhook

import (
	"regexp"
	"testing"
)

func TestNewSecretShape(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{64}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s := NewSecret()
		if !re.MatchString(s) {
			t.Fatalf("secret %q not 64 hex chars", s)
		}
		if seen[s] {
			t.Fatal("NewSecret repeated a value")
		}
		seen[s] = true
	}
}

func TestVerify(t *testing.T) {
	body := []byte(`{"event_name":"task.overdue","data":{}}`)
	secret := "0123456789abcdef0123456789abcdef"
	good := Sign(body, secret)

	cases := []struct {
		name        string
		body        []byte
		sig, secret string
		want        bool
	}{
		{"valid", body, good, secret, true},
		{"wrong secret", body, good, "nope", false},
		{"tampered body", []byte(`{"event_name":"task.created"}`), good, secret, false},
		{"empty sig", body, "", secret, false},
		{"empty secret", body, good, "", false},
		{"garbage sig", body, "zzzz", secret, false},
		{"uppercase sig", body, "ABCDEF", secret, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Verify(tc.body, tc.sig, tc.secret); got != tc.want {
				t.Errorf("Verify = %v, want %v", got, tc.want)
			}
		})
	}
}

// The signature Vikunja sends is hex(HMAC-SHA256(rawBody, secret)) — pin the
// exact algorithm against a known vector.
func TestSignKnownVector(t *testing.T) {
	// echo -n 'hello' | openssl dgst -sha256 -hmac 'key'
	got := Sign([]byte("hello"), "key")
	want := "9307b3b915efb5171ff14d8cb55fbcc798c6c0ef1456d66ded1a6aa723a58b7b"
	if got != want {
		t.Errorf("Sign = %s, want %s", got, want)
	}
}
