package store

import (
	"context"
	"errors"
	"testing"
)

func TestMetaRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if _, err := db.Meta(ctx, "relay_token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Meta on empty = %v, want ErrNotFound", err)
	}
	if err := db.SetMeta(ctx, "relay_token", "rt_1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.Meta(ctx, "relay_token"); v != "rt_1" {
		t.Fatalf("Meta = %q", v)
	}
	if err := db.SetMeta(ctx, "relay_token", "rt_2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.Meta(ctx, "relay_token"); v != "rt_2" {
		t.Fatalf("after update Meta = %q", v)
	}
}
