package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for i := 0; i < 3; i++ {
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open #%d: %v", i, err)
		}
		db.Close()
	}
}

func TestMigrationsApplied(t *testing.T) {
	db := openTemp(t)
	var n int
	if err := db.sql.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no migrations recorded")
	}
	for _, table := range []string{"users", "devices", "webhooks", "notifications_sent"} {
		var name string
		err := db.sql.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

func TestUserTokenRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if _, err := db.UserToken(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UserToken on empty = %v, want ErrNotFound", err)
	}

	if err := db.UpsertUserToken(ctx, 1, []byte("cipher-a")); err != nil {
		t.Fatal(err)
	}
	got, err := db.UserToken(ctx, 1)
	if err != nil || string(got) != "cipher-a" {
		t.Fatalf("got %q, %v", got, err)
	}

	if err := db.UpsertUserToken(ctx, 1, []byte("cipher-b")); err != nil {
		t.Fatal(err)
	}
	got, _ = db.UserToken(ctx, 1)
	if string(got) != "cipher-b" {
		t.Fatalf("after update got %q", got)
	}

	if err := db.DeleteUser(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UserToken(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteUserCascadesDevices(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.UpsertUserToken(ctx, 7, []byte("c")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO devices (user_id, apns_token, public_key) VALUES (7, 'tok', x'00')`); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, 7); err != nil {
		t.Fatal(err)
	}
	var n int
	db.sql.QueryRow(`SELECT count(*) FROM devices WHERE user_id = 7`).Scan(&n)
	if n != 0 {
		t.Fatalf("devices not cascaded: %d rows remain", n)
	}
}
