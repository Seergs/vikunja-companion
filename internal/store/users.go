package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("store: not found")

// UpsertUserToken stores (or replaces) the encrypted Vikunja API token for a
// user. tokenEnc is ciphertext — encryption is the caller's responsibility.
func (db *DB) UpsertUserToken(ctx context.Context, userID int64, tokenEnc []byte) error {
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO users (user_id, token_enc) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			token_enc  = excluded.token_enc,
			updated_at = datetime('now')`,
		userID, tokenEnc)
	if err != nil {
		return fmt.Errorf("store: upsert user %d: %w", userID, err)
	}
	return nil
}

// UserToken returns the encrypted Vikunja API token for a user, or ErrNotFound.
func (db *DB) UserToken(ctx context.Context, userID int64) ([]byte, error) {
	var tokenEnc []byte
	err := db.sql.QueryRowContext(ctx,
		`SELECT token_enc FROM users WHERE user_id = ?`, userID,
	).Scan(&tokenEnc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: user token %d: %w", userID, err)
	}
	return tokenEnc, nil
}

// DeleteUser removes a user and, via ON DELETE CASCADE, their devices and
// webhook registration.
func (db *DB) DeleteUser(ctx context.Context, userID int64) error {
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM users WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: delete user %d: %w", userID, err)
	}
	return nil
}
