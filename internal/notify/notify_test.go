package notify

import (
	"context"
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

type sentCall struct {
	userID int64
	n      Notification
}

type captureSender struct {
	mu    sync.Mutex
	got   []sentCall
	failN int // fail the first N sends
}

func (c *captureSender) Send(_ context.Context, userID int64, n Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failN > 0 {
		c.failN--
		return errors.New("send boom")
	}
	c.got = append(c.got, sentCall{userID, n})
	return nil
}

func TestDispatchSendsPerNotification(t *testing.T) {
	s := &captureSender{}
	d := New(newMemDedupe(), s, testLogger())

	notifs := []Notification{
		{Title: "Overdue", Body: "2 tasks", Deeplink: "today", DedupeKey: "batch:1:2026-08-29"},
		{Title: "Reminder", Body: "Buy milk", DedupeKey: "reminder:5:1"},
	}
	if err := d.Dispatch(context.Background(), 7, notifs); err != nil {
		t.Fatal(err)
	}
	if len(s.got) != 2 {
		t.Fatalf("got %d sends, want 2", len(s.got))
	}
	if s.got[0].userID != 7 || s.got[0].n.Title != "Overdue" {
		t.Errorf("send[0] = %+v", s.got[0])
	}
}

func TestDispatchDedupes(t *testing.T) {
	s := &captureSender{}
	d := New(newMemDedupe(), s, testLogger())
	n := Notification{Title: "x", DedupeKey: "same"}

	_ = d.Dispatch(context.Background(), 1, []Notification{n})
	_ = d.Dispatch(context.Background(), 1, []Notification{n})

	if len(s.got) != 1 {
		t.Fatalf("got %d sends, want 1 (deduped)", len(s.got))
	}
}

func TestDispatchContinuesPastSendFailure(t *testing.T) {
	s := &captureSender{failN: 1}
	d := New(newMemDedupe(), s, testLogger())
	notifs := []Notification{
		{Title: "first", DedupeKey: "a"},  // send fails
		{Title: "second", DedupeKey: "b"}, // succeeds
	}
	if err := d.Dispatch(context.Background(), 1, notifs); err != nil {
		t.Fatal(err)
	}
	if len(s.got) != 1 || s.got[0].n.Title != "second" {
		t.Fatalf("got %+v, want one send of 'second'", s.got)
	}
}

func TestDispatchDedupeErrorPropagates(t *testing.T) {
	dedupe := newMemDedupe()
	dedupe.err = errors.New("db down")
	d := New(dedupe, &captureSender{}, testLogger())
	if err := d.Dispatch(context.Background(), 1, []Notification{{DedupeKey: "k"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDispatchNoDedupeKeyAlwaysSends(t *testing.T) {
	s := &captureSender{}
	d := New(newMemDedupe(), s, testLogger())
	n := Notification{Title: "no key"}

	_ = d.Dispatch(context.Background(), 1, []Notification{n})
	_ = d.Dispatch(context.Background(), 1, []Notification{n})

	if len(s.got) != 2 {
		t.Fatalf("got %d sends, want 2 (no dedupe key)", len(s.got))
	}
}
