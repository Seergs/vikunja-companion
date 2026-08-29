// Package webhook verifies, parses, and reconciles Vikunja's user-level
// webhooks.
//
// v1 surface is exactly three user-directed events:
//
//	task.reminder.fired
//	task.overdue
//	tasks.overdue
//
// Inbound auth: X-Vikunja-Signature = lowercase hex(HMAC-SHA256(rawBody,
// secret)), constant-time compare, reject on mismatch/missing. The target_url is
// identical for every user, so the handler identifies the sender by trying each
// stored HMAC secret against the raw body.
//
// The envelope is {event_name, time, data}. The data shapes, the management
// routes, and their quirks are written up in docs/webhooks-verified.md.
package webhook

// TODO(fase-2): VerifySignature; ParseEvent; Registrar (reconcile on
// device (de)registration, on settings change, and on an hourly timer).
