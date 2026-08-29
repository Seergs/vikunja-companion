// Package webhook verifies and parses Vikunja's user-level webhooks, and hands
// the app the values it needs to register one by hand.
//
// v1 surface is exactly three user-directed events:
//
//	task.reminder.fired
//	task.overdue
//	tasks.overdue
//
// The companion never calls Vikunja's webhook API — a Vikunja API token cannot
// reach /api/v1/user/settings/webhooks*. The user creates the webhook in
// Vikunja's UI using the target URL and per-user HMAC secret this package
// generates; from then on it only verifies inbound deliveries.
//
// Inbound auth: X-Vikunja-Signature = lowercase hex(HMAC-SHA256(rawBody,
// secret)), constant-time compare, reject on mismatch/missing. The target_url is
// identical for every user, so the handler identifies the sender by trying each
// stored HMAC secret against the raw body.
//
// The envelope is {event_name, time, data}. The data shapes and the quirks are
// written up in docs/webhooks-verified.md.
package webhook

// TODO(fase-2): VerifySignature; ParseEvent; the setup helper (generate + store
// a per-user secret, return {target_url, secret, events}); the inbound handler.
