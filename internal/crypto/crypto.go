// Package crypto holds the two independent primitives the companion needs:
//
//   - Seal: a NaCl sealed box (crypto_box_seal — ephemeral X25519 +
//     XSalsa20-Poly1305) that encrypts a notification payload to a device's
//     X25519 public key, so the relay never sees content.
//   - Cipher: authenticated encryption of small secrets at rest (the Vikunja
//     API token, the webhook HMAC secret) with the companion master key.
package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/nacl/box"
)

// PublicKeySize is the length of an X25519 public key.
const PublicKeySize = 32

// Seal encrypts plaintext to recipientPublicKey (a 32-byte X25519 public key)
// as an anonymous NaCl sealed box. The output is
// ephemeralPublicKey(32) || box, ~48 bytes larger than plaintext. Only the
// holder of the matching private key (the device's Notification Service
// Extension) can open it.
func Seal(plaintext, recipientPublicKey []byte) ([]byte, error) {
	if len(recipientPublicKey) != PublicKeySize {
		return nil, fmt.Errorf("crypto: recipient public key must be %d bytes, got %d", PublicKeySize, len(recipientPublicKey))
	}
	var pk [PublicKeySize]byte
	copy(pk[:], recipientPublicKey)

	sealed, err := box.SealAnonymous(nil, plaintext, &pk, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("crypto: seal: %w", err)
	}
	return sealed, nil
}

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
