// Package notify is the reusable delivery seam: notification -> dedupe -> seal
// -> relay push.
//
// It must not import internal/vikunja or internal/webhook, or learn anything
// Vikunja-specific. The event -> notification mapping lives in the caller (v1:
// internal/webhook/build.go); post-v1 sources (a /api/v1/notifications poller)
// call Dispatch the same way without touching sealing or delivery.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// Notification is the content delivered to a device. It is intentionally tiny —
// well under the APNs 4 KB limit even sealed.
type Notification struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Deeplink string `json:"deeplink,omitempty"`

	// DedupeKey identifies the logical event. A notification whose key has been
	// seen before is dropped. Not part of the sealed payload.
	DedupeKey string `json:"-"`
}

// Device is one target for a user's notifications.
type Device struct {
	APNsToken string
	PublicKey []byte // raw 32-byte X25519 key
}

// Sealer encrypts a payload to a device's public key (internal/crypto).
type Sealer interface {
	Seal(plaintext, devicePublicKey []byte) ([]byte, error)
}

// Deduper records and checks event fingerprints (internal/store).
type Deduper interface {
	MarkSent(ctx context.Context, dedupeKey string) (fresh bool, err error)
}

// Push is a single sealed delivery for the relay.
type Push struct {
	APNsToken  string
	Ciphertext []byte
	CollapseID string
}

// Pusher forwards a sealed payload toward APNs (internal/relay client).
type Pusher interface {
	Push(ctx context.Context, p Push) error
}

// Dispatcher runs the delivery pipeline. Construct it with New.
type Dispatcher struct {
	dedupe Deduper
	sealer Sealer
	pusher Pusher
	log    *slog.Logger
}

// New returns a Dispatcher.
func New(dedupe Deduper, sealer Sealer, pusher Pusher, log *slog.Logger) *Dispatcher {
	return &Dispatcher{dedupe: dedupe, sealer: sealer, pusher: pusher, log: log}
}

// Dispatch delivers each notification to every device: it drops duplicates by
// DedupeKey, seals the payload per device, and pushes it. A per-device push
// failure is logged and does not stop the rest.
func (d *Dispatcher) Dispatch(ctx context.Context, devices []Device, notifications []Notification) error {
	if len(devices) == 0 {
		return nil
	}
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

		payload, err := json.Marshal(n)
		if err != nil {
			return fmt.Errorf("notify: marshal notification: %w", err)
		}

		for _, dev := range devices {
			ciphertext, err := d.sealer.Seal(payload, dev.PublicKey)
			if err != nil {
				d.log.Warn("notify: seal failed, skipping device", "err", err)
				continue
			}
			if err := d.pusher.Push(ctx, Push{
				APNsToken:  dev.APNsToken,
				Ciphertext: ciphertext,
				CollapseID: n.DedupeKey,
			}); err != nil {
				d.log.Warn("notify: push failed", "err", err)
			}
		}
	}
	return nil
}
