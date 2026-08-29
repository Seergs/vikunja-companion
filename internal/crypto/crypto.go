// Package crypto seals notification payloads to a device's X25519 public key
// with a NaCl sealed box (crypto_box_seal: ephemeral X25519 +
// XSalsa20-Poly1305, ~48 bytes overhead), so the relay never sees content.
//
// It also holds the COMPANION_MASTER_KEY at-rest encryption for stored Vikunja
// API tokens.
package crypto

// TODO(fase-2): Seal(plaintext, devicePubKey) ([]byte, error) via
// golang.org/x/crypto/nacl/box.SealAnonymous; EncryptToken / DecryptToken
// using the master key (AEAD).
