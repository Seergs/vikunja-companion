// Package store is the companion's SQLite persistence layer.
//
// Driver is modernc.org/sqlite (pure Go) so the container stays a distroless
// static binary — do not switch to mattn/go-sqlite3. The connection pool is
// pinned to SetMaxOpenConns(1): this is one small service, not a place for a
// pool.
//
// Schema (no users table — see 0001_init.sql):
// webhooks(user_id, secret_enc, events, last_delivery_at),
// user_settings(user_id, digest_enabled, digest_time, timezone),
// notifications_sent(dedupe_key, sent_at).
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("store: not found")

//go:embed migrations/*.sql
var companionMigrations embed.FS

const migrationsDir = "migrations"

// DB is a thin wrapper over *sql.DB carrying the companion's DAOs.
type DB struct {
	sql *sql.DB
}

// Open opens the companion database at path, applies pending migrations, and
// returns a ready DB. path may be ":memory:".
func Open(path string) (*DB, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(ON)"
	} else if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("store: creating %s: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// Single writer, single connection — see package doc.
	sqlDB.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: ping %s: %w", path, err)
	}

	db := &DB{sql: sqlDB}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the underlying database.
func (db *DB) Close() error { return db.sql.Close() }

// parseDBTime parses a timestamp written by SQLite's datetime('now')
// ("2006-01-02 15:04:05", UTC). Returns the zero time on any failure.
func parseDBTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// migrate applies every embedded migration not yet recorded in
// schema_migrations, in filename order, each in its own transaction.
func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`,
	); err != nil {
		return fmt.Errorf("store: creating schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(companionMigrations, migrationsDir)
	if err != nil {
		return fmt.Errorf("store: reading migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		if err := db.sql.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = ?)`, name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("store: checking migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		body, err := companionMigrations.ReadFile(migrationsDir + "/" + name)
		if err != nil {
			return fmt.Errorf("store: reading migration %s: %w", name, err)
		}

		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: applying migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: recording migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %s: %w", name, err)
		}
	}
	return nil
}
