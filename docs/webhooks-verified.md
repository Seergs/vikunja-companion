# Vikunja webhooks — verified reference

Concrete facts for implementing `internal/webhook` and the webhook routes in
`internal/vikunja`. Resolves the `docs/COMPANION.md` section 11 checklist.

**Verified against:** `tasks.sergiosuarez.dev`, Vikunja **`v2.5.0`**, on
2026-08-29 — via `GET /api/v1/info`, `GET /api/v1/docs.json` (Swagger 2.0), and
the Vikunja source (`go-vikunja/vikunja@main`: `pkg/models/events.go`,
`pkg/models/webhooks.go`, `pkg/models/listeners.go`, `pkg/config/config.go`).

Items marked ⚠︎ still need a live authenticated call to fully confirm — see
"Left to check with a token" at the bottom.

---

## Instance prerequisites (from `GET /api/v1/info`)

| Field | Value on the test instance | Meaning |
|---|---|---|
| `version` | `v2.5.0` | |
| `webhooks_enabled` | `true` | required for any delivery; Vikunja default is `true` |
| `email_reminders_enabled` | `true` | reminder/overdue crons also write in-app notifications here; on an instance without a mailer they would not, and the webhook is the only signal |
| `max_items_per_page` | `50` | pagination ceiling for the future notifications poller |

Don't pin a Vikunja version. Probe `webhooks_enabled` in `/api/v1/info` and the
event list in `GET /api/v1/user/settings/webhooks/events` at runtime, the same
way the app does capability detection.

---

## Signature

- Header: **`X-Vikunja-Signature`**
- Value: **`hex(HMAC_SHA256(rawBody, webhook.secret))`** — lowercase hex, of the
  exact bytes POSTed. Verify against the raw request body before any parsing.
- Only sent when the webhook has a `secret`. Treat a missing header as a
  verification failure.
- Other headers on the delivery: `User-Agent: Vikunja/<version>`,
  `Content-Type: application/json`.
- Source: `pkg/models/webhooks.go` → `sendWebhookPayload`.

## Envelope

```json
{
  "event_name": "task.overdue",
  "time": "2026-08-29T09:00:00.123456789-06:00",
  "data": { ... }
}
```

- `time` is RFC 3339 with nanoseconds and a numeric offset (Go `time.Time`
  default marshal).
- `data` is an object whose keys are the JSON field names of the event struct
  (below). Nested objects (`task`, `user`, `project`) are reloaded fresh from
  the DB at send time, so they are full objects, not stubs.
- Source: `WebhookPayload` in `pkg/models/listeners.go`.

## The three user-directed events

`PUT /api/v1/user/settings/webhooks` accepts **only** these three (source:
`listeners.go` `RegisterUserDirectedEventForWebhook`, enforced in
`Webhook.Create`: a user-level webhook with any other event → `400`, invalid
field `events`). This is the entire v1 push surface.

### `task.reminder.fired`

```jsonc
"data": {
  "task":     { /* full Task */ },
  "user":     { /* full user.User — the recipient */ },
  "project":  { /* full Project */ },
  "reminder": { "reminder": "<RFC3339>", "relative_period": 0, "relative_to": "" }
}
```

Fires once per recipient (creator + assignees + subscribers) in the minute the
reminder is due, with a −12h/+14h timezone window. A due date alone does **not**
create a reminder.

### `task.overdue`

```jsonc
"data": {
  "task":    { /* full Task */ },
  "user":    { /* recipient */ },
  "project": { /* full Project */ }
}
```

### `tasks.overdue`  ← prefer this one

```jsonc
"data": {
  "tasks":    [ { /* full Task */ }, ... ],
  "user":     { /* recipient */ },
  "projects": { "6": { /* Project */ }, "9": { /* Project */ } }   // map keyed by project id (string in JSON)
}
```

Both overdue events fire from the same cron, which only notifies a given user
when the clock crosses that user's `overdue_tasks_reminders_time` (default
`09:00`, their timezone) — once per day, retrospective. Batch `tasks.overdue`
carries every overdue task in one delivery; `task.overdue` also fires per task.
Register `tasks.overdue` and fall back to formatting a single-task message when
`len(tasks) == 1`; only subscribe to `task.overdue` as well if you specifically
want the per-task deliveries.

Note: `data.user` is the **recipient**, not an actor — there is no `doer` on
these events (unlike `task.created` etc.) and no self-authored noise to filter.

## Object fields we rely on

**Task** (`models.Task`, 36 fields — relevant subset): `id`, `title`,
`identifier` (e.g. `PROJ-12`), `index`, `done`, `due_date`, `project_id`,
`priority`. Deep link target = `id`.

**Project** (`models.Project`): `id`, `title`, `identifier`, `is_archived`,
`parent_project_id`.

**user.User**: `id`, `username`, `name`, `email`. Match deliveries to a stored
user by `id`.

## Webhook management routes

All under `/api/v1`, auth `Authorization: Bearer <token>`.

| Method + path | Purpose | Notes |
|---|---|---|
| `GET /user/settings/webhooks` | list the user's webhooks | returns an array of `models.Webhook`; `secret` is **write-only, never returned** |
| `PUT /user/settings/webhooks` | create one | body `models.Webhook`; returns `200` + the created webhook. `UserID` is set from auth — don't send it |
| `POST /user/settings/webhooks/{id}` | **update `events` only** | "You cannot change other values of a webhook" — `target_url` and `secret` are immutable |
| `DELETE /user/settings/webhooks/{id}` | delete one | returns `models.Message` |
| `GET /user/settings/webhooks/events` ⚠︎ | list user-directed event names | array of strings; expected `["task.overdue","task.reminder.fired","tasks.overdue"]` |
| `GET /webhooks/events` | list **all** webhook events | not what you want for a user-level webhook |

Create body that works:

```json
{
  "target_url": "https://<COMPANION_PUBLIC_URL>/companion/v1/webhooks/vikunja",
  "events": ["task.reminder.fired", "task.overdue", "tasks.overdue"],
  "secret": "<32+ random bytes, hex>"
}
```

Validation in `Webhook.Create`: `events` is required and non-empty; every entry
must be a known event; for a user-level webhook every entry must be
user-directed; exactly one of project/user scope (the route fixes this).

### Consequences for reconciliation

- The `secret` cannot be read back. The companion must persist the secret it
  generated. If it's ever lost, the only recovery is `DELETE` + `PUT` a new one.
- `POST .../{id}` changes `events` and nothing else. To change `target_url`
  (e.g. `COMPANION_PUBLIC_URL` moved) you must delete and recreate.
- Reconcile logic: find the row whose `target_url` matches ours →
  - none → `PUT` a fresh one, store id + secret;
  - found, `events` differ from desired → `POST .../{id}`;
  - found, `target_url` ours but we have no stored secret → `DELETE` + `PUT`.

## Routability

If Vikunja can't reach `COMPANION_PUBLIC_URL` over a routable IP, deliveries are
dropped by the SSRF-safe HTTP client. Instance-side knobs (defaults in
`pkg/config/config.go`):

- `webhooks.enabled` (default `true`)
- `webhooks.allownonroutableips` (default `false`) — needs to be `true` if the
  companion is only reachable on a private IP. Better: give it a real hostname.
- `webhooks.timeoutseconds` (default `30`)

## Retries

The design doc says Vikunja doesn't retry. **On `main` this has changed** — the
webhook delivery listener now has per-delivery retry via watermill middleware
(`pkg/models/listeners.go`, `WebhookDeliveryListener`). Whether that shipped in
`v2.5.0` is ⚠︎ unconfirmed. Either way: dedupe on an event fingerprint
(`notifications_sent`) so a retry or a poller-reconciled duplicate is dropped.

## Notifications endpoint (for the post-v1 poller)

- `GET /api/v1/notifications?page=&per_page=` → array of
  `notifications.DatabaseNotification` `{ id, name, notification, created,
  read_at }`. `notification` is a free-form object whose shape depends on
  `name`.
- Pagination headers: `x-pagination-total-pages`, `x-pagination-result-count`.
- `POST /api/v1/notifications` marks all read; `POST /api/v1/notifications/{id}`
  toggles one. The poller must never mark read — that's the user's state.

## `GET /api/v1/user`

Returns `v1.UserWithSettings` — includes `id`, `username`, `name`, `email`, plus
`settings`. Our `vikunja.User{ID, Username}` subset is correct.

---

## Left to check with a token

A read-only API token from the test instance would confirm:

1. `GET /api/v1/user/settings/webhooks/events` — exact JSON array.
2. `PUT` a throwaway webhook, then read one real delivery (e.g. set a reminder
   1–2 min out) to confirm the hex signature end-to-end and the exact `data`
   for `task.reminder.fired` / `tasks.overdue`.
3. `PUT` with `events: ["task.created"]` → expect `400` invalid `events`.
4. `GET /api/v1/notifications` — one real page, to see `notification` bodies.
