// Package notify is the reusable delivery seam: notification -> dedupe -> seal
// -> relay push (docs/COMPANION.md §6.4, §9).
//
// It must not import internal/vikunja or learn anything Vikunja-specific. The
// event -> notification mapping lives in a Source (v1: internal/webhook);
// post-v1 sources (a /api/v1/notifications poller) plug in here without touching
// sealing or delivery.
package notify

import "context"

// Notification is the content sealed to a device and delivered via the relay.
// It is intentionally tiny — well under the APNs 4 KB limit even sealed.
type Notification struct {
	Title     string
	Body      string
	Deeplink  string
	DedupeKey string
}

// Source produces notifications for delivery. It is the only Vikunja-aware seam
// in the delivery path.
type Source interface {
	// Notifications drains any notifications the source has produced since the
	// last call.
	Notifications(ctx context.Context) ([]Notification, error)
}

// TODO(fase-2): Dispatcher wiring Source -> dedupe (notifications_sent) ->
// crypto.Seal -> relay client, fanning out per device.
