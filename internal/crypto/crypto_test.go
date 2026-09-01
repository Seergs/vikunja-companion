package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

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
