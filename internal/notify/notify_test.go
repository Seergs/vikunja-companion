package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type memDedupe struct {
	mu   sync.Mutex
	seen map[string]bool
	err  error
}

func newMemDedupe() *memDedupe { return &memDedupe{seen: map[string]bool{}} }

func (m *memDedupe) MarkSent(_ context.Context, k string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen[k] {
		return false, nil
	}
	m.seen[k] = true
	return true, nil
}

type fakeSealer struct{ failFor string }

func (f *fakeSealer) Seal(plaintext, pub []byte) ([]byte, error) {
	if string(pub) == f.failFor {
		return nil, errors.New("bad key")
	}
	return append([]byte("sealed:"), plaintext...), nil
}

type capturePusher struct {
	mu    sync.Mutex
	got   []Push
	failN int // fail the first N pushes
}

func (c *capturePusher) Push(_ context.Context, p Push) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failN > 0 {
		c.failN--
		return errors.New("push boom")
	}
	c.got = append(c.got, p)
	return nil
}

func TestDispatchSealsAndPushesPerDevice(t *testing.T) {
	push := &capturePusher{}
	d := New(newMemDedupe(), &fakeSealer{}, push, testLogger())

	devices := []Device{
		{APNsToken: "tokA", PublicKey: []byte("keyA")},
		{APNsToken: "tokB", PublicKey: []byte("keyB")},
	}
	n := Notification{Title: "Overdue", Body: "2 tasks", Deeplink: "today", DedupeKey: "batch:1:2026-08-29"}

	if err := d.Dispatch(context.Background(), devices, []Notification{n}); err != nil {
		t.Fatal(err)
	}
	if len(push.got) != 2 {
		t.Fatalf("got %d pushes, want 2", len(push.got))
	}
	for _, p := range push.got {
		if p.CollapseID != n.DedupeKey {
			t.Errorf("collapse id = %q", p.CollapseID)
		}
		payload := p.Ciphertext[len("sealed:"):]
		var back Notification
		if err := json.Unmarshal(payload, &back); err != nil {
			t.Fatal(err)
		}
		if back.Title != "Overdue" || back.Body != "2 tasks" || back.Deeplink != "today" {
			t.Errorf("payload = %+v", back)
		}
		if back.DedupeKey != "" {
			t.Error("DedupeKey must not be in the sealed payload")
		}
	}
}

func TestDispatchDedupes(t *testing.T) {
	push := &capturePusher{}
	dedupe := newMemDedupe()
	d := New(dedupe, &fakeSealer{}, push, testLogger())
	devices := []Device{{APNsToken: "t", PublicKey: []byte("k")}}
	n := Notification{Title: "x", DedupeKey: "same"}

	_ = d.Dispatch(context.Background(), devices, []Notification{n})
	_ = d.Dispatch(context.Background(), devices, []Notification{n})

	if len(push.got) != 1 {
		t.Fatalf("got %d pushes, want 1 (deduped)", len(push.got))
	}
}

func TestDispatchNoDevicesIsNoop(t *testing.T) {
	dedupe := newMemDedupe()
	d := New(dedupe, &fakeSealer{}, &capturePusher{}, testLogger())
	if err := d.Dispatch(context.Background(), nil, []Notification{{DedupeKey: "k"}}); err != nil {
		t.Fatal(err)
	}
	if len(dedupe.seen) != 0 {
		t.Error("no-device dispatch should not consume the dedupe key")
	}
}

func TestDispatchContinuesPastPerDeviceFailures(t *testing.T) {
	push := &capturePusher{failN: 1}
	d := New(newMemDedupe(), &fakeSealer{failFor: "badkey"}, push, testLogger())
	devices := []Device{
		{APNsToken: "a", PublicKey: []byte("badkey")}, // seal fails
		{APNsToken: "b", PublicKey: []byte("ok")},     // push fails (failN)
		{APNsToken: "c", PublicKey: []byte("ok")},     // succeeds
	}
	if err := d.Dispatch(context.Background(), devices, []Notification{{Title: "x", DedupeKey: "k"}}); err != nil {
		t.Fatal(err)
	}
	if len(push.got) != 1 || push.got[0].APNsToken != "c" {
		t.Fatalf("got %+v, want one push to c", push.got)
	}
}

func TestDispatchDedupeErrorPropagates(t *testing.T) {
	dedupe := newMemDedupe()
	dedupe.err = errors.New("db down")
	d := New(dedupe, &fakeSealer{}, &capturePusher{}, testLogger())
	err := d.Dispatch(context.Background(), []Device{{APNsToken: "t", PublicKey: []byte("k")}}, []Notification{{DedupeKey: "k"}})
	if err == nil {
		t.Fatal("expected error")
	}
}
