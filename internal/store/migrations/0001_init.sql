-- Companion schema v1 (docs/COMPANION.md §3, §6, §7).

-- One row per Vikunja user the companion acts for. The Vikunja API token is
-- stored encrypted with COMPANION_MASTER_KEY (§3, §8); deleting a user's last
-- device deletes this row and the webhook registration.
CREATE TABLE users (
    user_id    INTEGER PRIMARY KEY,          -- Vikunja user id
    token_enc  BLOB NOT NULL,                -- AEAD-encrypted Vikunja API token
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- One row per registered iOS device (§6.4). public_key is the device's X25519
-- public key; notifications are sealed to it before leaving the companion.
CREATE TABLE devices (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    apns_token  TEXT NOT NULL,
    public_key  BLOB NOT NULL,
    app_version TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, apns_token)
);

CREATE INDEX idx_devices_user ON devices(user_id);

-- One user-level Vikunja webhook registration per user (§6.3). secret is the
-- HMAC key; events is a comma-separated subset of the three v1 events.
CREATE TABLE webhooks (
    user_id    INTEGER PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    vikunja_id INTEGER NOT NULL,
    secret     TEXT NOT NULL,
    events     TEXT NOT NULL,
    target_url TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Delivery dedupe by event fingerprint (§6.4).
CREATE TABLE notifications_sent (
    dedupe_key TEXT PRIMARY KEY,
    sent_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
