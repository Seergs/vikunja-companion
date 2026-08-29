package store

import (
	"context"
	"errors"
	"testing"
)

func seedUser(t *testing.T, db *DB, id int64) {
	t.Helper()
	if err := db.UpsertUserToken(context.Background(), id, []byte("tok")); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	seedUser(t, db, 1)

	if _, err := db.Webhook(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty Webhook = %v, want ErrNotFound", err)
	}

	if err := db.UpsertWebhook(ctx, 1, []byte("cipher"), []string{"task.overdue", "tasks.overdue"}); err != nil {
		t.Fatal(err)
	}
	w, err := db.Webhook(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(w.SecretEnc) != "cipher" || len(w.Events) != 2 || w.Events[0] != "task.overdue" {
		t.Fatalf("webhook = %+v", w)
	}
	if !w.LastDeliveryAt.IsZero() {
		t.Errorf("LastDeliveryAt should be zero, got %v", w.LastDeliveryAt)
	}

	if err := db.UpsertWebhook(ctx, 1, []byte("cipher2"), []string{"task.reminder.fired"}); err != nil {
		t.Fatal(err)
	}
	w, _ = db.Webhook(ctx, 1)
	if string(w.SecretEnc) != "cipher2" || len(w.Events) != 1 {
		t.Fatalf("after update: %+v", w)
	}

	if err := db.TouchWebhookDelivery(ctx, 1); err != nil {
		t.Fatal(err)
	}
	w, _ = db.Webhook(ctx, 1)
	if w.LastDeliveryAt.IsZero() {
		t.Error("LastDeliveryAt still zero after touch")
	}
}

func TestAllWebhookSecrets(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, id := range []int64{1, 2, 3} {
		seedUser(t, db, id)
		if err := db.UpsertWebhook(ctx, id, []byte{byte(id)}, []string{"task.overdue"}); err != nil {
			t.Fatal(err)
		}
	}
	secrets, err := db.AllWebhookSecrets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 3 {
		t.Fatalf("got %d secrets", len(secrets))
	}
}

func TestDeviceUpsertAndList(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	seedUser(t, db, 1)

	id1, err := db.UpsertDevice(ctx, Device{UserID: 1, APNsToken: "a", PublicKey: []byte("k1"), AppVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := db.UpsertDevice(ctx, Device{UserID: 1, APNsToken: "b", PublicKey: []byte("k2")})
	if id1 == id2 {
		t.Fatal("distinct devices got the same id")
	}

	// Re-register "a" with a rotated key — same row.
	idAgain, _ := db.UpsertDevice(ctx, Device{UserID: 1, APNsToken: "a", PublicKey: []byte("k1-new"), AppVersion: "1.1"})
	if idAgain != id1 {
		t.Fatalf("re-register changed id: %d -> %d", id1, idAgain)
	}

	devs, err := db.DevicesForUser(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("got %d devices", len(devs))
	}
	if string(devs[0].PublicKey) != "k1-new" || devs[0].AppVersion != "1.1" {
		t.Errorf("device[0] = %+v", devs[0])
	}
}

func TestDeleteDeviceReportsRemaining(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	seedUser(t, db, 1)
	db.UpsertDevice(ctx, Device{UserID: 1, APNsToken: "a", PublicKey: []byte("k")})
	db.UpsertDevice(ctx, Device{UserID: 1, APNsToken: "b", PublicKey: []byte("k")})

	remaining, err := db.DeleteDevice(ctx, 1, "a")
	if err != nil || remaining != 1 {
		t.Fatalf("remaining = %d, err = %v", remaining, err)
	}
	remaining, _ = db.DeleteDevice(ctx, 1, "b")
	if remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}

func TestMarkSentDedupes(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	fresh, err := db.MarkSent(ctx, "evt-abc")
	if err != nil || !fresh {
		t.Fatalf("first MarkSent: fresh=%v err=%v", fresh, err)
	}
	fresh, err = db.MarkSent(ctx, "evt-abc")
	if err != nil || fresh {
		t.Fatalf("second MarkSent: fresh=%v err=%v (want false)", fresh, err)
	}
	fresh, _ = db.MarkSent(ctx, "evt-xyz")
	if !fresh {
		t.Fatal("different key should be fresh")
	}
}

func TestWebhookDeletedWithUser(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	seedUser(t, db, 1)
	db.UpsertWebhook(ctx, 1, []byte("s"), []string{"task.overdue"})

	if err := db.DeleteUser(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Webhook(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("webhook survived user delete: %v", err)
	}
}
