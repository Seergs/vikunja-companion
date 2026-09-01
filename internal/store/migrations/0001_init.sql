-- Companion schema v1.
--
-- There is no users table: the companion identifies callers per-request from
-- their Vikunja bearer token and never stores it (the digest cron uses
-- COMPANION_VIKUNJA_TOKEN from env). Rows below are keyed by the Vikunja
-- user_id but hold no credential that identifies the user.

-- One row per user, holding the HMAC secret for the webhook the user created by
-- hand in Vikunja (the companion cannot manage it via the API). events is a
-- comma-separated subset of the three v1 events the user wants forwarded.
CREATE TABLE webhooks (
    user_id          INTEGER PRIMARY KEY,           -- Vikunja user id
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
