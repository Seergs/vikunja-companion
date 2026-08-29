// Package webhook verifies, parses, and reconciles Vikunja's user-level
// webhooks (docs/COMPANION.md §6.2–§6.4).
//
// v1 surface is exactly three user-directed events:
//
//	task.reminder.fired
//	task.overdue
//	tasks.overdue
//
// Inbound auth: X-Vikunja-Signature = hex(HMAC-SHA256(rawBody, secret)),
// constant-time compare, reject on mismatch/missing. The target_url is identical
// for every user, so the handler identifies the sender by trying each stored
// HMAC secret against the raw body.
//
// NOTE (docs/COMPANION.md §11): the exact JSON shape of `data` for each event
// is assumed, not verified. Confirm against a live /api/v1/docs before
// implementing parsing.
package webhook

// TODO(fase-2): VerifySignature; ParseEvent; Registrar (reconcile on
// device (de)registration, on settings change, and on an hourly timer).
