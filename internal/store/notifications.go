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

// NotificationSent reports whether a notification with the given dedupe key has
// already been recorded. It is a read-only pre-check; MarkSent still does the
// recording (and its own duplicate check) at delivery time.
func (db *DB) NotificationSent(ctx context.Context, dedupeKey string) (bool, error) {
	var exists bool
	err := db.sql.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM notifications_sent WHERE dedupe_key = ?)`, dedupeKey,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: notification sent %q: %w", dedupeKey, err)
	}
	return exists, nil
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
