package store

import (
	"context"
	"path/filepath"
	"regexp"
	"testing"
)

func openTempRelay(t *testing.T) *DB {
	t.Helper()
	db, err := OpenRelay(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatalf("OpenRelay: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRelayTokenLifecycle(t *testing.T) {
	db := openTempRelay(t)
	ctx := context.Background()

	tok, err := db.MintToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^rt_[0-9a-f]{48}$`).MatchString(tok) {
		t.Fatalf("token shape: %q", tok)
	}

	ok, err := db.ValidToken(ctx, tok)
	if err != nil || !ok {
		t.Fatalf("ValidToken(minted) = %v, %v", ok, err)
	}
	ok, _ = db.ValidToken(ctx, "rt_deadbeef")
	if ok {
		t.Fatal("unknown token reported valid")
	}
}

func TestRelayTokensAreDistinct(t *testing.T) {
	db := openTempRelay(t)
	ctx := context.Background()
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tok, err := db.MintToken(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
	}
}

func TestRelaySchemaHasNoCompanionTables(t *testing.T) {
	db := openTempRelay(t)
	var name string
	err := db.sql.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&name)
	if err == nil {
		t.Fatal("relay DB should not have the companion 'users' table")
	}
}
