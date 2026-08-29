package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// MintToken creates and stores a fresh opaque relay registration token.
func (db *DB) MintToken(ctx context.Context) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: mint token: %w", err)
	}
	token := "rt_" + hex.EncodeToString(b)

	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO registrations (token) VALUES (?)`, token); err != nil {
		return "", fmt.Errorf("store: storing token: %w", err)
	}
	return token, nil
}

// ValidToken reports whether token is registered, bumping its last_seen.
func (db *DB) ValidToken(ctx context.Context, token string) (bool, error) {
	res, err := db.sql.ExecContext(ctx,
		`UPDATE registrations SET last_seen = datetime('now') WHERE token = ?`, token)
	if err != nil {
		return false, fmt.Errorf("store: validate token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: validate token: %w", err)
	}
	return n == 1, nil
}
