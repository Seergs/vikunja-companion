-- Companion schema v2: per-user notification preferences.

-- One row per user, holding what the iOS app lets them configure. Absent row =
-- defaults (digest on at 08:00). timezone caches the user's Vikunja
-- settings.timezone so the digest cron does not need an API call to decide
-- whether a user's local send time has arrived.
CREATE TABLE user_settings (
    user_id        INTEGER PRIMARY KEY,             -- Vikunja user id
    digest_enabled INTEGER NOT NULL DEFAULT 1,
    digest_time    TEXT    NOT NULL DEFAULT '08:00',  -- "HH:MM" in the user's timezone
    timezone       TEXT    NOT NULL DEFAULT '',       -- IANA name, cached from Vikunja
    updated_at     TEXT    NOT NULL DEFAULT (datetime('now'))
);
