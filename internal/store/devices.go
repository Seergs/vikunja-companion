package store

import (
	"context"
	"fmt"
)

// Device is a registered iOS device. PublicKey is the raw 32-byte X25519 key.
type Device struct {
	ID         int64
	UserID     int64
	APNsToken  string
	PublicKey  []byte
	AppVersion string
}

// UpsertDevice inserts a device or, if (user_id, apns_token) already exists,
// updates its key/version and bumps last_seen. Returns the device id.
func (db *DB) UpsertDevice(ctx context.Context, d Device) (int64, error) {
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO devices (user_id, apns_token, public_key, app_version) VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, apns_token) DO UPDATE SET
			public_key  = excluded.public_key,
			app_version = excluded.app_version,
			last_seen   = datetime('now')`,
		d.UserID, d.APNsToken, d.PublicKey, d.AppVersion)
	if err != nil {
		return 0, fmt.Errorf("store: upsert device (user %d): %w", d.UserID, err)
	}

	var id int64
	if err := db.sql.QueryRowContext(ctx,
		`SELECT id FROM devices WHERE user_id = ? AND apns_token = ?`, d.UserID, d.APNsToken,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: reading back device id: %w", err)
	}
	return id, nil
}

// DevicesForUser returns all of a user's registered devices.
func (db *DB) DevicesForUser(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT id, apns_token, public_key, app_version FROM devices WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: devices for user %d: %w", userID, err)
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d := Device{UserID: userID}
		if err := rows.Scan(&d.ID, &d.APNsToken, &d.PublicKey, &d.AppVersion); err != nil {
			return nil, fmt.Errorf("store: scanning device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDevice removes one device and returns how many the user has left.
func (db *DB) DeleteDevice(ctx context.Context, userID int64, apnsToken string) (remaining int, err error) {
	if _, err = db.sql.ExecContext(ctx,
		`DELETE FROM devices WHERE user_id = ? AND apns_token = ?`, userID, apnsToken); err != nil {
		return 0, fmt.Errorf("store: delete device (user %d): %w", userID, err)
	}
	if err = db.sql.QueryRowContext(ctx,
		`SELECT count(*) FROM devices WHERE user_id = ?`, userID).Scan(&remaining); err != nil {
		return 0, fmt.Errorf("store: counting remaining devices (user %d): %w", userID, err)
	}
	return remaining, nil
}
