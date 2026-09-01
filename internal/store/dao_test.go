package store

import (
	"context"
	"errors"
	"testing"
)

func TestWebhookRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

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
