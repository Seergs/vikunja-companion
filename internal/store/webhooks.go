package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Webhook is a user's stored webhook state (Option E: manual setup, so this is
// just the HMAC secret plus the events the user wants forwarded).
type Webhook struct {
	UserID         int64
	SecretEnc      []byte
	Events         []string
	LastDeliveryAt time.Time // zero if no delivery has been verified yet
}

// WebhookSecret is one user's encrypted secret, for the inbound handler to try
// against a delivery whose sender is not yet known.
type WebhookSecret struct {
	UserID    int64
	SecretEnc []byte
}

// UpsertWebhook stores (or replaces) a user's webhook secret and event
// preferences. secretEnc is ciphertext.
func (db *DB) UpsertWebhook(ctx context.Context, userID int64, secretEnc []byte, events []string) error {
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO webhooks (user_id, secret_enc, events) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			secret_enc = excluded.secret_enc,
			events     = excluded.events,
			updated_at = datetime('now')`,
		userID, secretEnc, joinEvents(events))
	if err != nil {
		return fmt.Errorf("store: upsert webhook %d: %w", userID, err)
	}
	return nil
}

// Webhook returns a user's webhook state, or ErrNotFound.
func (db *DB) Webhook(ctx context.Context, userID int64) (*Webhook, error) {
	var (
		w        = Webhook{UserID: userID}
		events   string
		lastDlvr sql.NullString
	)
	err := db.sql.QueryRowContext(ctx,
		`SELECT secret_enc, events, last_delivery_at FROM webhooks WHERE user_id = ?`, userID,
	).Scan(&w.SecretEnc, &events, &lastDlvr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: webhook %d: %w", userID, err)
	}
	w.Events = splitEvents(events)
	if lastDlvr.Valid {
		w.LastDeliveryAt = parseDBTime(lastDlvr.String)
	}
	return &w, nil
}

// AllWebhookSecrets returns every stored secret. The inbound webhook target URL
// is identical for all users, so the handler must try each until one verifies.
func (db *DB) AllWebhookSecrets(ctx context.Context) ([]WebhookSecret, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT user_id, secret_enc FROM webhooks`)
	if err != nil {
		return nil, fmt.Errorf("store: all webhook secrets: %w", err)
	}
	defer rows.Close()

	var out []WebhookSecret
	for rows.Next() {
		var s WebhookSecret
		if err := rows.Scan(&s.UserID, &s.SecretEnc); err != nil {
			return nil, fmt.Errorf("store: scanning webhook secret: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// TouchWebhookDelivery records that a verified delivery just arrived for a user.
func (db *DB) TouchWebhookDelivery(ctx context.Context, userID int64) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE webhooks SET last_delivery_at = datetime('now') WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("store: touch webhook delivery %d: %w", userID, err)
	}
	return nil
}

func joinEvents(events []string) string { return strings.Join(events, ",") }

func splitEvents(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
