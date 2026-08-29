// Package store is the SQLite persistence layer for both binaries
// (docs/COMPANION.md §7).
//
// Driver is modernc.org/sqlite (pure Go) so the container stays a distroless
// static binary — do not switch to mattn/go-sqlite3. The connection pool is
// pinned to SetMaxOpenConns(1): this is one small service, not a place for a
// pool.
//
// Companion schema (fase 1): users(user_id, token_enc, ...),
// devices(id, user_id, apns_token, public_key, app_version, ...),
// webhooks(user_id, vikunja_id, secret, events, target_url),
// notifications_sent(dedupe_key, sent_at).
//
// Relay schema: registrations(token, created, last_seen).
package store

// TODO(fase-1): Open(path) with SetMaxOpenConns(1) + embedded migrations;
// companion DAOs. Relay DAO comes in fase 2.
