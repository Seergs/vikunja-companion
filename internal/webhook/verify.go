package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// SignatureHeader is the header Vikunja puts the HMAC in.
const SignatureHeader = "X-Vikunja-Signature"

// secretBytes is the length of a generated webhook secret before hex encoding.
const secretBytes = 32

// NewSecret returns a fresh random webhook secret as a 64-character hex string,
// suitable for pasting into Vikunja's webhook form and storing as the HMAC key.
func NewSecret() string {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		panic("webhook: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Sign returns the expected signature for a body under secret: lowercase
// hex(HMAC-SHA256(body, secret)).
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether sig is a valid X-Vikunja-Signature for body under
// secret, comparing in constant time. An empty sig or secret is never valid.
func Verify(body []byte, sig, secret string) bool {
	if sig == "" || secret == "" {
		return false
	}
	want := Sign(body, secret)
	return subtle.ConstantTimeCompare([]byte(sig), []byte(want)) == 1
}
