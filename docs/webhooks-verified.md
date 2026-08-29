# Vikunja webhooks — verified reference

Concrete facts for implementing `internal/webhook` and the webhook routes in
`internal/vikunja`. Resolves the `docs/COMPANION.md` section 11 checklist.

**Verified against:** `tasks.sergiosuarez.dev`, Vikunja **`v2.5.0`**, on
2026-08-29 — via `GET /api/v1/info`, `GET /api/v1/docs.json` (Swagger 2.0), a
live API token, and the Vikunja source (`go-vikunja/vikunja@main`:
`pkg/models/events.go`, `pkg/models/webhooks.go`, `pkg/models/listeners.go`,
`pkg/models/api_routes.go`, `pkg/models/task_reminder.go`,
`pkg/models/task_overdue_reminder.go`, `pkg/routes/routes.go`,
`pkg/routes/api_tokens.go`, `pkg/config/config.go`).

---

## API tokens cannot manage user-level webhooks (resolved: manual setup)

A Vikunja API token (`tk_…`) **cannot call `/api/v1/user/settings/webhooks*`**.
Not a scope you can grant — a hard exclusion in Vikunja:
`CollectRoutesForAPITokenUsage` (`pkg/models/api_routes.go`) early-returns for
every route whose group name starts with `user_`, so `CanDoAPIRoute` can never
authorise those paths for any token. The auth middleware then 401s
(`code 11`, "invalid token").

Confirmed live: with a token that reads `/projects`, `/labels`,
`/notifications`, `/tasks` fine, both `GET` and `PUT`
`/api/v1/user/settings/webhooks` → **401 `code 11`**. Only a **JWT** (from an
interactive local / LDAP / OIDC login) can manage user-level webhooks.

This changed `docs/COMPANION.md` section 3 + 6.3: the companion was to store the
user's API token and use it to create/reconcile the user-level webhook. It
can't. Poll is not a clean fallback either — the reminder/overdue crons only
write the `/api/v1/notifications` row when
`ServiceEnableEmailReminders && MailerEnabled` **and** the user's own
`email_reminders_enabled` / `overdue_tasks_reminders_enabled` is on
(`task_reminder.go`, `task_overdue_reminder.go`). The webhook events fire
whenever `webhooks.enabled`, independent of any of that — which is exactly why
the webhook was the design's reliable path. On the test account
`email_reminders_enabled` is already `false`, so a poller would see no reminder
notifications at all.

### Decision (2026-08-29): Option E — manual, one-time setup

The user creates the webhook by hand in Vikunja's web UI. The companion never
calls the webhook API. `GET /companion/v1/webhooks/setup` hands the app
`{target_url, secret, events}` to display; the companion stores the secret and
only verifies inbound deliveries. No reconciliation — it can't even detect a
deleted webhook (listing is the same blocked route), so it records
`last_delivery_at` and the app nudges after a long silence. Keeps the
API-token-only identity model intact. Written up in `docs/COMPANION.md` 6.3.

Options that were considered and rejected:

- **A — the iOS app registers the webhook with its own JWT.** Needs the app to
  model JWT auth (today it only does API tokens) and to reconcile on every
  launch. More app work than E for a marginal self-healing gain.
- **B — the companion becomes an OAuth2 client of Vikunja.** Durable delegated
  JWT via `/api/v1/oauth/authorize` + `/oauth/token`, but a big lift and the
  user must register an OAuth app. Overkill for one webhook.
- **C — project-level webhooks.** Dead end: the three user-directed events are
  only dispatched to *user-level* webhooks (`listeners.go` ~L1536), so a project
  webhook subscribed to `task.overdue` never fires.
- **D — poll `/api/v1/notifications`.** Works with an API token, but only sees
  reminder/overdue rows on a mailer-configured instance with the user's email
  reminders enabled — unreliable, and the whole reason the design chose webhooks.

---

## Live check results (with an API token, 2026-08-29)

- `GET /api/v1/user` → `{id, username, name, email, created, updated, settings{…},
  is_admin, auth_provider, …}`. Our `vikunja.User{ID, Username}` is fine.
  `settings.overdue_tasks_reminders_time` is `"9:00"` (string `H:MM`),
  `settings.timezone` `"America/Mexico_City"`.
- `GET /api/v1/notifications?page=&per_page=` → `200`, headers
  `x-pagination-total-pages` + `x-pagination-result-count` present (both `0` on
  an empty account). Also emits `access-control-expose-headers` listing them.
- `GET /api/v1/user/settings/webhooks/events`, `GET /api/v1/user/settings/webhooks`,
  `PUT /api/v1/user/settings/webhooks`, `GET /api/v1/tokens` → all **401 code 11**
  (see blocker above).
- Still unchecked (needs a JWT): the exact `.../webhooks/events` array, one real
  delivery end-to-end, the non-user-directed-event `400`.

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
| `GET /user/settings/webhooks/events` | list user-directed event names | array of strings; expected `["task.overdue","task.reminder.fired","tasks.overdue"]` |
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
`v2.5.0` is unconfirmed. Either way: dedupe on an event fingerprint
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

## Left to check — needs a JWT (API token is not enough, see blocker)

1. `GET /api/v1/user/settings/webhooks/events` — exact JSON array (source says
   `["task.overdue","task.reminder.fired","tasks.overdue"]`).
2. `PUT` a throwaway webhook, then read one real delivery (e.g. set a reminder
   1–2 min out) to confirm the hex signature end-to-end and the exact `data`
   for `task.reminder.fired` / `tasks.overdue`.
3. `PUT` with `events: ["task.created"]` on a user-level webhook → expect `400`
   invalid `events`.

A JWT can be lifted from the browser dev tools (the `Authorization: Bearer`
header on any XHR while using the Vikunja web UI); it is short-lived, fine for a
one-off check.
