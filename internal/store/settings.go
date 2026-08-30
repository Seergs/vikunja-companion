package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Default per-user settings, applied when a user has no user_settings row.
const (
	DefaultDigestEnabled = true
	DefaultDigestTime    = "08:00"
)

// Settings is a user's notification preferences. A user with no stored row gets
// the Default* values above.
type Settings struct {
	DigestEnabled bool
	DigestTime    string // "HH:MM" in the user's timezone
	Timezone      string // IANA name, cached from Vikunja; "" if not yet known
}

// UserSettings returns a user's preferences, or the defaults when no row exists.
func (db *DB) UserSettings(ctx context.Context, userID int64) (Settings, error) {
	s := Settings{DigestEnabled: DefaultDigestEnabled, DigestTime: DefaultDigestTime}
	err := db.sql.QueryRowContext(ctx,
		`SELECT digest_enabled, digest_time, timezone FROM user_settings WHERE user_id = ?`, userID,
	).Scan(&s.DigestEnabled, &s.DigestTime, &s.Timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("store: user settings %d: %w", userID, err)
	}
	return s, nil
}

// PutDigestSettings stores a user's digest enable flag and send time, leaving
// the cached timezone untouched.
func (db *DB) PutDigestSettings(ctx context.Context, userID int64, enabled bool, hhmm string) error {
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO user_settings (user_id, digest_enabled, digest_time) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			digest_enabled = excluded.digest_enabled,
			digest_time    = excluded.digest_time,
			updated_at     = datetime('now')`,
		userID, enabled, hhmm)
	if err != nil {
		return fmt.Errorf("store: put digest settings %d: %w", userID, err)
	}
	return nil
}

// SetUserTimezone caches the user's Vikunja timezone so the digest cron can
// decide locally whether their send time has arrived.
func (db *DB) SetUserTimezone(ctx context.Context, userID int64, tz string) error {
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO user_settings (user_id, timezone) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			timezone   = excluded.timezone,
			updated_at = datetime('now')`,
		userID, tz)
	if err != nil {
		return fmt.Errorf("store: set user timezone %d: %w", userID, err)
	}
	return nil
}

// DigestTarget is one candidate for the morning digest: a user with at least
// one registered device, plus their (coalesced) settings.
type DigestTarget struct {
	UserID   int64
	Enabled  bool
	Time     string
	Timezone string
}

// ListDigestTargets returns every user that has a device, with their digest
// settings folded in (defaults where no user_settings row exists).
func (db *DB) ListDigestTargets(ctx context.Context) ([]DigestTarget, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT u.user_id,
		       COALESCE(s.digest_enabled, 1),
		       COALESCE(s.digest_time, ?),
		       COALESCE(s.timezone, '')
		FROM users u
		LEFT JOIN user_settings s ON s.user_id = u.user_id
		WHERE EXISTS (SELECT 1 FROM devices d WHERE d.user_id = u.user_id)
		ORDER BY u.user_id`,
		DefaultDigestTime)
	if err != nil {
		return nil, fmt.Errorf("store: list digest targets: %w", err)
	}
	defer rows.Close()

	var out []DigestTarget
	for rows.Next() {
		var t DigestTarget
		if err := rows.Scan(&t.UserID, &t.Enabled, &t.Time, &t.Timezone); err != nil {
			return nil, fmt.Errorf("store: scanning digest target: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
