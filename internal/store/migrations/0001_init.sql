-- Companion schema v1.

-- One row per Vikunja user the companion acts for. The Vikunja API token is
-- stored encrypted with COMPANION_MASTER_KEY; deleting a user's last device
-- deletes this row and the webhook registration.
CREATE TABLE users (
    user_id    INTEGER PRIMARY KEY,          -- Vikunja user id
    token_enc  BLOB NOT NULL,                -- AEAD-encrypted Vikunja API token
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- One row per registered iOS device. public_key is the device's X25519 public
-- key; notifications are sealed to it before leaving the companion.
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

-- One row per user, holding the HMAC secret for the webhook the user created by
-- hand in Vikunja (the companion cannot manage it via the API). events is a
-- comma-separated subset of the three v1 events the user wants forwarded.
CREATE TABLE webhooks (
    user_id          INTEGER PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    secret_enc       BLOB NOT NULL,                 -- HMAC key, encrypted with COMPANION_MASTER_KEY
    events           TEXT NOT NULL,
    last_delivery_at TEXT,                          -- updated on every verified inbound delivery
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Delivery dedupe by event fingerprint.
CREATE TABLE notifications_sent (
    dedupe_key TEXT PRIMARY KEY,
    sent_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
