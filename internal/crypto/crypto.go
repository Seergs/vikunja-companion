// Package crypto holds authenticated encryption of small secrets at rest (the
// Vikunja API token, the webhook HMAC secret) with the companion master key.
package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Cipher encrypts and decrypts small values at rest with the companion master
// key using XChaCha20-Poly1305. The 24-byte random nonce makes reuse a
// non-issue at the companion's volume; it is prepended to the ciphertext.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher returns a Cipher for a 32-byte master key.
func NewCipher(masterKey []byte) (*Cipher, error) {
	if len(masterKey) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("crypto: master key must be %d bytes, got %d", chacha20poly1305.KeySize, len(masterKey))
	}
	aead, err := chacha20poly1305.NewX(masterKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns nonce || ciphertext||tag.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// ErrDecrypt is returned when a ciphertext is malformed or fails authentication
// (wrong key, corruption, tampering).
var ErrDecrypt = errors.New("crypto: cannot decrypt")

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(ciphertext) < ns+c.aead.Overhead() {
		return nil, ErrDecrypt
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
