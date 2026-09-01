// Package notify is the reusable delivery seam: notification -> dedupe -> send
// to the user's configured channel.
//
// It must not import internal/vikunja or internal/webhook, or learn anything
// Vikunja-specific. The event -> notification mapping lives in the caller (v1:
// internal/webhook/build.go and internal/digest); post-v1 sources call Dispatch
// the same way without touching delivery.
//
// The companion does not deliver notifications itself. It POSTs each one to an
// Apprise endpoint (apprise-api). Dispatch hands the notification to a Sender
// that does the POST. userID is passed through for a future per-user routing
// step; the v1 Sender uses one operator-configured endpoint for everyone.
package notify

import (
	"context"
	"fmt"
	"log/slog"
)

// Level is the severity of a notification. It maps onto whatever the delivery
// channel calls it (Apprise "type", etc.). The zero value is LevelInfo.
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
)

// Notification is one delivered message. It is intentionally tiny.
type Notification struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Deeplink string `json:"deeplink,omitempty"`
	Level    Level  `json:"level,omitempty"`

	// DedupeKey identifies the logical event. A notification whose key has been
	// seen before is dropped.
	DedupeKey string `json:"-"`
}

// Deduper records and checks event fingerprints (internal/store).
type Deduper interface {
	MarkSent(ctx context.Context, dedupeKey string) (fresh bool, err error)
}

// Sender delivers one notification for a user. It resolves the user's delivery
// destination (v1: an apprise-api endpoint) and POSTs to it. A user with no
// destination configured is not an error — Send returns nil.
type Sender interface {
	Send(ctx context.Context, userID int64, n Notification) error
}

// Dispatcher runs the delivery pipeline. Construct it with New.
type Dispatcher struct {
	dedupe Deduper
	sender Sender
	log    *slog.Logger
}

// New returns a Dispatcher.
func New(dedupe Deduper, sender Sender, log *slog.Logger) *Dispatcher {
	return &Dispatcher{dedupe: dedupe, sender: sender, log: log}
}

// Dispatch delivers each notification for userID: it drops duplicates by
// DedupeKey, then hands the rest to the Sender. A send failure is logged and
// does not stop the remaining notifications.
func (d *Dispatcher) Dispatch(ctx context.Context, userID int64, notifications []Notification) error {
	for _, n := range notifications {
		if n.DedupeKey != "" {
			fresh, err := d.dedupe.MarkSent(ctx, n.DedupeKey)
			if err != nil {
				return fmt.Errorf("notify: dedupe %q: %w", n.DedupeKey, err)
			}
			if !fresh {
				d.log.Debug("notify: duplicate dropped", "dedupe_key", n.DedupeKey)
				continue
			}
		}

		if err := d.sender.Send(ctx, userID, n); err != nil {
			d.log.Warn("notify: send failed", "user", userID, "dedupe_key", n.DedupeKey, "err", err)
		}
	}
	return nil
}
