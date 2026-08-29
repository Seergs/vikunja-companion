package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestSealOpensWithRecipientKey(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte(`{"title":"Reminder","body":"Buy milk"}`)

	sealed, err := Seal(msg, pub[:])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("Buy milk")) {
		t.Fatal("plaintext leaked into sealed box")
	}
	if len(sealed) != len(msg)+box.AnonymousOverhead {
		t.Errorf("overhead = %d, want %d", len(sealed)-len(msg), box.AnonymousOverhead)
	}

	got, ok := box.OpenAnonymous(nil, sealed, pub, priv)
	if !ok {
		t.Fatal("OpenAnonymous failed")
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("got %q, want %q", got, msg)
	}
}

func TestSealRejectsBadKeyLength(t *testing.T) {
	if _, err := Seal([]byte("x"), make([]byte, 31)); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestSealIsNondeterministic(t *testing.T) {
	pub, _, _ := box.GenerateKey(rand.Reader)
	a, _ := Seal([]byte("same"), pub[:])
	b, _ := Seal([]byte("same"), pub[:])
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same message are identical (ephemeral key not random?)")
	}
}

func masterKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher(masterKey(t))
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("tk_cf20c0ea57e04e5e42232c8751314d4d48741442")

	ct, err := c.Encrypt(secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, secret) {
		t.Fatal("plaintext visible in ciphertext")
	}

	pt, err := c.Decrypt(ct)
	if err != nil || !bytes.Equal(pt, secret) {
		t.Fatalf("Decrypt = %q, %v", pt, err)
	}
}

func TestCipherEncryptIsNondeterministic(t *testing.T) {
	c, _ := NewCipher(masterKey(t))
	a, _ := c.Encrypt([]byte("x"))
	b, _ := c.Encrypt([]byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("nonce reuse: two encryptions identical")
	}
}

func TestCipherWrongKeyFails(t *testing.T) {
	c1, _ := NewCipher(masterKey(t))
	c2, _ := NewCipher(masterKey(t))

	ct, _ := c1.Encrypt([]byte("secret"))
	if _, err := c2.Decrypt(ct); err != ErrDecrypt {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestCipherTamperFails(t *testing.T) {
	c, _ := NewCipher(masterKey(t))
	ct, _ := c.Encrypt([]byte("secret"))
	ct[len(ct)-1] ^= 0x01

	if _, err := c.Decrypt(ct); err != ErrDecrypt {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestCipherShortInput(t *testing.T) {
	c, _ := NewCipher(masterKey(t))
	if _, err := c.Decrypt([]byte("too short")); err != ErrDecrypt {
		t.Fatalf("err = %v, want ErrDecrypt", err)
	}
}

func TestNewCipherRejectsBadKey(t *testing.T) {
	if _, err := NewCipher(make([]byte, 16)); err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}
