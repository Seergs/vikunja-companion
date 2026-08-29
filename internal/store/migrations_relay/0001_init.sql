-- Relay schema. Opaque registration tokens only — no PII, no content.
CREATE TABLE registrations (
    token      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen  TEXT NOT NULL DEFAULT (datetime('now'))
);
