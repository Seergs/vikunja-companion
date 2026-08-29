package store

import (
	"context"
	"fmt"
)

// MarkSent records that a notification with the given dedupe key has been sent.
// It returns fresh=true when the key was newly inserted (send it), and
// fresh=false when it was already present (a duplicate — drop it).
func (db *DB) MarkSent(ctx context.Context, dedupeKey string) (fresh bool, err error) {
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO notifications_sent (dedupe_key) VALUES (?) ON CONFLICT(dedupe_key) DO NOTHING`,
		dedupeKey)
	if err != nil {
		return false, fmt.Errorf("store: mark sent %q: %w", dedupeKey, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: mark sent %q: %w", dedupeKey, err)
	}
	return n == 1, nil
}

// PruneSentBefore deletes dedupe rows older than the given number of days, to
// keep notifications_sent bounded. Returns the number removed.
func (db *DB) PruneSentBefore(ctx context.Context, days int) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM notifications_sent WHERE sent_at < datetime('now', '-%d days')`, days))
	if err != nil {
		return 0, fmt.Errorf("store: prune notifications_sent: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
