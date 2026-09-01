# Vikunja Companion Service — Design

Design doc for **`vikunja-companion`**, an optional, self-hosted, lightweight Go
service that sits in front of a Vikunja instance and adds features the native
iOS client can't get from Vikunja alone — starting with task notifications that
reach the user while the app is closed.

This doc is the "why". Implementation state lives in the repo's `CLAUDE.md`.

---

## 1. Context & philosophy

The iOS app points at a user's own Vikunja instance and re-fetches everything
from the server — no backend of its own. That keeps the app simple but leaves a
few things impossible or clumsy:

- **Notifications while the app is closed.** Vikunja does email + in-app
  notifications only. With the app fully closed there is no way to learn a task
  went overdue or that a reminder fired.
- **Anything that needs a process running while the app is closed** — a morning
  digest, background curation, unread badges.

`vikunja-companion` is the opt-in answer. Design constraints, in priority order:

1. **Optional.** A user who doesn't run it loses nothing — the app detects its
   absence and hides the extra features.
2. **Lightweight.** One small Go service, one SQLite file, one container. No
   message queue, no Postgres, no clustering, no push infrastructure.
3. **One URL.** The companion is a **transparent reverse proxy** to Vikunja for
   everything under `/api/v1/*`, and serves its own features under
   `/companion/v1/*`. The user puts **one** URL in the app (the companion's),
   not two.
4. **No proprietary delivery channel.** The companion does not run its own push
   service and is not tied to Apple. It **delegates delivery to
   [Apprise](https://github.com/caronc/apprise)** — one integration that already
   speaks ntfy, Discord, Slack, Telegram, Gotify, Pushover, email, and ~100
   other services. The user runs
   [`apprise-api`](https://github.com/caronc/apprise-api) (a small container)
   pointed at whatever service they use; the companion POSTs each notification
   to it. The rationale and the alternative that was rejected (a
   maintainer-operated APNs relay) are in
   [`adrs/0001-push-relay-vs-delegated-delivery.md`](adrs/0001-push-relay-vs-delegated-delivery.md).
5. **Never a fork.** It calls Vikunja's public HTTP API like any other client. A
   Vikunja API change is contained the same way it is in the app — to the layer
   that speaks HTTP.

License: **AGPLv3**, matching Vikunja.

---

## 2. Topology

```
                        ┌─────────────────────────────────────────┐
                        │            vikunja-companion            │
                        │              (self-hosted)              │
 ┌──────────┐  one URL  │  ┌───────────────┐   ┌───────────────┐  │   ┌──────────┐
 │  iOS app │──────────►│  │ reverse proxy │──►│ /api/v1/*     │──┼──►│ Vikunja  │
 │          │           │  │  /api/v1/*    │   │  passthrough  │  │   │ upstream │
 │          │           │  └───────────────┘   └───────────────┘  │   └──────────┘
 │          │◄─────────►│  ┌───────────────────────────────────┐  │        │
 │          │           │  │ /companion/v1/*  (own features)    │  │        │ user-level
 └──────────┘           │  │  info · settings                  │  │        │ webhook
                        │  │  webhook  · webhooks/vikunja  ◄────┼──┼────────┘ (task.overdue,
                        │  └───────────────┬───────────────────┘  │           reminder.fired)
                        │       SQLite     │  format notification │
                        └─────────────────┼──────────────────────┘
                                          │  HTTP POST
                                          ▼
                              ┌────────────────────────┐
                              │  apprise-api           │  the user runs it, configured
                              │  (the user runs it)     │  for ntfy / Discord / Gotify /
                              └───────────┬────────────┘  Telegram / email / ~100 more
                                          ▼
                                  the user's devices
```

Two moving parts:

| Part | Who hosts it | State | Sees task content? |
|---|---|---|---|
| `companion` binary | the user (self-hosted) | SQLite: tokens, webhook secrets, per-user settings, dedupe keys | yes — it's the user's own box |
| `apprise-api` | the user | its own | yes — the user configures which service(s) it fans out to |

There is no project-operated infrastructure. Nothing the user cannot see or
replace.

---

## 3. Identity model

The companion has **no accounts, no login, no registration** of its own.

- The app authenticates to Vikunja with a long-lived **API token** (the app only
  models API-token auth today). Every proxied request already carries it.
- To identify a caller on a `/companion/v1/*` route, the companion takes the
  same `Authorization: Bearer <token>` and calls `GET /api/v1/user` upstream,
  then caches `sha256(token) → {user_id, ttl}` (a few minutes). That route needs
  the token's `other` permission group (see `webhooks-verified.md`) — the app's
  connect flow must tell the user to grant it.
- "User" = whatever Vikunja says a token belongs to. The companion never manages
  users; every table is keyed by the Vikunja `user_id` with no `users` table
  behind it.

**The companion does not store any Vikunja API token.** Per-request
`/companion/v1/*` calls carry the token in the header and it is never persisted.
The one job that needs to act as a user with no request in flight — the **digest
cron** — reads its token from `COMPANION_VIKUNJA_TOKEN` (env). That makes the
digest single-user in v1: only that token's user gets a morning briefing.

Inbound webhook notifications stay **multi-user** — they identify the user by
which stored HMAC secret verifies the delivery (6.3), not by a token.

Per-user digest routing (token in `user_settings`, encrypted at rest) is a
post-v1 step, together with the `/api/v1/notifications` poller that also needs a
stored token.

---

## 4. The reverse proxy

- Everything **not** under `/companion/` is proxied verbatim to
  `VIKUNJA_UPSTREAM_URL`: method, path, query, headers, body, status, streaming
  responses.
- No caching of authenticated responses. No request rewriting beyond
  `Host`/`X-Forwarded-*`.
- TLS terminates at the companion (or at a proxy in front of it). The companion
  needs a public HTTPS URL anyway — Vikunja must reach it to deliver webhooks.
- Scope is **API only**. The companion does not serve Vikunja's web SPA. The
  only client is the iOS app.

---

## 5. Capability discovery

`GET /companion/v1/info` (no auth):

```json
{
  "companion": { "version": "0.1.0" },
  "vikunja":   { "url": "https://vikunja.example.com", "version": "0.24.x" },
  "features":  ["push", "digest"]
}
```

The app already probes `GET /api/v1/info` during onboarding to confirm it's a
real Vikunja instance. It gains a parallel probe of `/companion/v1/info`:

- reachable → set a `hasCompanion` flag on the `InstanceAccount`, unlock the
  feature UI gated on each entry in `features`.
- 404 / not reachable → behave exactly as today (bare Vikunja).

`"push"` means the companion can turn Vikunja events into notifications and POST
them to the user's Apprise endpoint; `"digest"` gates the morning-briefing
settings screen.

This is the same runtime capability-detection pattern the app already uses for
Vikunja server features (`CapabilityProvider.supports(_:)`), extended to "is
there a companion, and what does it offer".

---

## 6. Feature: notifications (v1)

### 6.1 What Vikunja actually emits

Verified against Vikunja source (`pkg/models/task_reminder.go`,
`task_overdue_reminder.go`, `listeners.go`). Two cron jobs, **both run every
minute**:

**Reminder cron** → notification + webhook `task.reminder.fired`
- A *reminder* is something the user set on the task: an absolute time, or
  relative to due/start/end date ("2h before due"). **Setting a due date alone
  does not create a reminder** — the web client optionally auto-adds one, but
  that's client behaviour.
- Fires for the task's **creator + assignees + subscribers** in the minute the
  reminder comes due (with a −12h/+14h window for timezones).

**Overdue cron** → notification + webhooks `task.overdue` (per task) and
`tasks.overdue` (one batch per user)
- Runs every minute but only notifies a given user **when the clock crosses that
  user's `overdue_tasks_reminders_time`** (default **09:00**, in the user's
  timezone). Once per day.
- At that time: undone tasks whose `due_date` has passed → **one** notification
  ("you have N overdue tasks"; single-task variant if N == 1). Retrospective —
  it's "what already slipped", not "what's due today".

**Everything else** (task assigned, comment, `@mention`, relation, project
shared, team add) lands in `GET /api/v1/notifications` **reliably** via event
listeners, independent of mailer config.

### 6.2 The decision: user-level webhook, three events

Vikunja has **user-level webhooks** — `PUT /api/v1/user/settings/webhooks` —
that fire across *all* the user's projects with **one registration**. But they
only accept **user-directed events**, and there are exactly three
(`pkg/models/listeners.go`):

```
task.reminder.fired
task.overdue
tasks.overdue
```

That's the whole v1 surface, and it's deliberate:

- It's exactly reminders + overdue — the notifications that matter with the app
  closed, and (see below) the *only* ones a webhook can carry.
- One webhook per user. No per-project enumeration, no "new project"
  reconciliation, no project-webhook lifecycle. Stays lightweight.
- **Critical:** the reminder/overdue crons write the in-app
  (`/api/v1/notifications`) row **only when a mailer is configured** — Vikunja
  treats them as an email feature. Many self-hosters run without SMTP. For them,
  the **webhook is the only way** to ever learn about a reminder or an overdue
  task. Webhooks fire whenever `webhooks.enabled` is true (the default).

Project-level events (comments, assignments, mentions) are **not** in v1. The
future path for those is a `/api/v1/notifications` poller (9), not
project-webhook registration.

### 6.3 Webhook registration — manual, one-time

**The user registers the webhook by hand in Vikunja's web UI.** The companion
does not create, read, update, or reconcile it.

Why: a Vikunja API token (`tk_…`) — the only credential the companion holds —
**cannot touch `/api/v1/user/settings/webhooks*`**. It is not a grantable scope;
every multi-segment `/api/v1/user/...` route is excluded from API-token auth in
Vikunja's code (`CanDoAPIRoute`). Only a JWT from an interactive login can
manage user-level webhooks, and the companion has no login. Details and the
alternatives considered (app-side JWT, OAuth2 client) are in
`webhooks-verified.md`.

Flow:

1. The app offers a **"Set up notifications" screen**. It calls
   `GET /companion/v1/webhook` (authenticated with the user's token), which
   returns `{ target_url, secret, events, last_delivery_at }` — the companion
   generates and stores the `secret` (HMAC key) keyed by `user_id`, idempotently
   (a repeat call returns the same secret so the user's Vikunja config stays
   valid).
2. The user opens Vikunja → **Settings → Webhooks → Create**, and pastes:
   - **Target URL:** `{COMPANION_PUBLIC_URL}/companion/v1/webhooks/vikunja`
   - **Secret:** the value from step 1
   - **Events:** the three (`task.reminder.fired`, `task.overdue`,
     `tasks.overdue`)
3. The app confirms by re-reading `last_delivery_at` (a test can be forced by
   setting a reminder ~1 min out).

Consequences of no reconciliation:

- If the user deletes the webhook in Vikunja, notifications stop and the
  companion **cannot detect it** (listing webhooks is the same blocked route).
  Mitigation: the companion records `last_delivery_at` and exposes it in
  `GET /companion/v1/webhook`; the app shows a soft warning after a long silence
  ("no signal from Vikunja in N days — check Settings → Webhooks").
- `events` drift (per-type toggles) also can't be pushed to Vikunja. v1: the app
  tells the user which events to tick; the companion just filters what it
  forwards. `COMPANION_WEBHOOK_EVENTS` is the operator ceiling.

Prerequisites, surfaced by the companion in its startup logs if unmet:

- `webhooks.enabled` = true on the Vikunja instance (default true). Read from
  `GET /api/v1/info` → `webhooks_enabled`.
- If the companion is on a non-routable IP relative to Vikunja, the instance
  needs `webhooks.allownonroutableips` = true. Better: give the companion a real
  hostname.

### 6.4 Delivery pipeline

```
Vikunja ──POST──► /companion/v1/webhooks/vikunja
                    │  identify the user: try each stored HMAC secret against the raw body
                    │  verify X-Vikunja-Signature = hex(HMAC-SHA256(rawBody, secret)), constant-time
                    │  parse {event_name, time, data}
                    ▼
                  build notification {title, body, deeplink, dedupe_key}   (internal/webhook)
                    │  dedupe against notifications_sent (event fingerprint)
                    ▼
                  hand to the Sender (internal/notify)
                    │  build {title, body, type}  (type: overdue → warning, else info)
                    ▼
                  HTTP POST to the configured apprise-api  (+ bearer if set)
```

- `tasks.overdue` (batch) is preferred over N × `task.overdue` to avoid a 9 a.m.
  notification storm; use `task.overdue` only when the batch has one task.
- Deep links are relative (`task/12`, `today`). Apprise has no universal
  click-action field, so v1 appends the link as a trailing line in the body; the
  app can still parse it.
- **Payload shapes** are verified — see `webhooks-verified.md`. Header is
  `X-Vikunja-Signature` = lowercase `hex(HMAC-SHA256(rawBody, secret))`; body is
  `{event_name, time, data}`; for these three events `data` carries `task` /
  `user` / `project` (`user` is the **recipient**, not a `doer` — there is no
  self-authored noise to drop), and the batch carries `tasks` / `user` /
  `projects` (a map keyed by project id).

The `internal/notify` seam (`Dispatcher.Dispatch(ctx, userID, notifications)` →
dedupe → `Sender.Send`) is delivery-agnostic. It knows nothing about Vikunja or
about Apprise — the event→notification mapping lives in `internal/webhook` and
`internal/digest`, and the outbound formatting lives behind the `Sender`
interface.

### 6.5 Delivery via Apprise

The companion targets **[Apprise](https://github.com/caronc/apprise)** and only
Apprise for v1. Apprise already normalises ntfy, Discord, Slack, Telegram,
Gotify, Pushover, Matrix, email, and ~100 more into one interface, so the
companion carries **one outbound code path** instead of a formatter per service.

The operator runs [`apprise-api`](https://github.com/caronc/apprise-api) (a small
container) configured for their service(s), and points the companion at it with
**env config** — fleet-wide, not per user:

```
COMPANION_APPRISE_API_URL    # apprise-api notify endpoint; unset -> delivery disabled (logs only)
COMPANION_APPRISE_URLS       # Apprise service URL(s), comma-separated (stateless form); optional
COMPANION_APPRISE_TOKEN      # bearer for the apprise-api endpoint; optional
```

- `COMPANION_APPRISE_API_URL` is either the persistent-config form
  `.../notify/{key}` (service URLs saved in apprise-api under `{key}`) or the
  stateless form `.../notify` (then set `COMPANION_APPRISE_URLS`).
- The companion POSTs `{"title", "body", "type"}` (`type` = `warning` for
  overdue, `info` otherwise; `"urls"` added when set). With
  `COMPANION_APPRISE_API_URL` unset it logs instead — the same as today's stub.

**Fleet-wide, not per user.** A companion serving one person (the normal case)
is unaffected. On a shared companion every user's notifications go to the same
Apprise target; per-user delivery routing is a post-v1 item (it moves into
`user_settings` then — `internal/notify` already passes `userID` to the
`Sender`).

The endpoint URL is operator-supplied, so there is no SSRF concern in v1 (it is
not user-controlled). That changes if per-user routing lands — see 9.

A **generic JSON webhook** kind (POST straight to a Discord/Slack/homemade URL,
skipping apprise-api) is an easy follow-up if anyone wants to avoid the extra
container — same `Sender` interface — but it is not in v1.

> Delivery is **not implemented yet** — the `Sender` is currently a stub that
> logs each notification. The webhook-ingest, dedupe, digest, and settings paths
> are in place; the Apprise `Sender` reading the env config above is the next
> step.

### 6.6 Why delegate instead of running a relay

Apple only accepts pushes signed with the APNs key of the Apple Developer
account that ships the App Store build, so "native" push for a self-hosted app
requires a piece of shared, maintainer-operated infrastructure holding that key.
That infrastructure — a content-blind APNs relay — was the original v1 plan. It
was rejected: it is a single point of failure and cost the maintainer must carry
forever, it is an abuse surface (anyone who can reach it can attempt a push), and
it is Apple-only. Delegating to a channel the user already runs removes all of
that, at the cost of the notification showing the channel app's branding rather
than the Vikunja app's. Full reasoning:
[`adrs/0001-push-relay-vs-delegated-delivery.md`](adrs/0001-push-relay-vs-delegated-delivery.md).

Native push, a generic webhook, or any other transport can still be added later
as an **additional** `Sender` without changing this design.

### 6.7 iOS app changes implied

Tracked here so they're not a surprise:

- `CompanionServiceProtocol` in `VikunjaCore/Protocols/`; concrete impl in
  `VikunjaNetworking`; wired in `AppContainer`. Features never import it
  directly.
- `InstanceAccount` gains a `hasCompanion` flag (or a small companion-info
  value); onboarding + `Settings`' connection form probe `/companion/v1/info`
  alongside `/api/v1/info`.
- A **Notifications** screen (folded into `Features/Settings`, or a new
  `Features/Notifications`) — visible only when `hasCompanion`:
  - A **"Set up Vikunja webhook"** step: fetch `GET /companion/v1/webhook`, show
    the `target_url` + `secret` + events for the user to paste into Vikunja →
    Settings → Webhooks, then confirm a delivery arrived via `last_delivery_at`.
    Also surface the "no signal in N days" warning.
  - **Per-type toggles** (reminders, overdue) that narrow what the companion
    forwards.
  - The Apprise endpoint is operator env config, not an app setting — the screen
    can show whether delivery is configured (from `/companion/v1/info`) but does
    not edit it.
- A **Morning briefing** settings screen (visible only when `/companion/v1/info`
  `features` includes `"digest"`): an enable toggle and a time picker, saved via
  `PUT /companion/v1/settings`.
- No Notification Service Extension, no keychain keypair, no APNs registration —
  the app does not receive the notifications itself; the operator's Apprise
  targets do.

### 6.8 Morning briefing (daily digest)

A forward-looking counterpart to the retrospective overdue notification: one
notification each morning — **"8 tasks for today · 1 urgent"** — deep-linking to
the Today screen. Built, unit-tested; shares the `notify` delivery seam with the
webhook path.

**Pull, not a webhook.** Vikunja emits no "your day" event, so a cron in
`cmd/companion` (`internal/digest`) drives it. It acts for one user — whoever
`COMPANION_VIKUNJA_TOKEN` belongs to (resolved once at startup via
`GET /api/v1/user`). No token, or one Vikunja rejects → the digest is off.

- Wakes every 5 minutes. Computes the local wall clock from that user's cached
  Vikunja `settings.timezone` and checks whether their send time (default
  **08:00**) has passed within the last **2 hours** (the window keeps a
  companion that was down all morning from firing a stale briefing at 15:00).
- In the window and not already sent today: fetch undone tasks due through end
  of the local day via `GET /api/v1/tasks`, count them, and deliver one
  `notify.Notification` (`Deeplink: "today"`). **Urgent** = Vikunja
  `priority >= 4`. **Zero tasks → no notification.**
- Idempotent per day through a `digest:<user_id>:<local_date>` key in
  `notifications_sent`; restarts and the exact tick cadence do not matter.

**Config:** `COMPANION_DIGEST_ENABLED` (default true) is the kill switch;
`COMPANION_VIKUNJA_TOKEN` is what enables it at all. The enable flag, send time,
and cached timezone live in `user_settings`, set from the app via
`GET`/`PUT /companion/v1/settings` (`{ digest: { enabled, time }, timezone }`;
`time` is validated as 24-hour `HH:MM`). Those are stored per `user_id`, so a
second user can save settings — they just will not fire until per-user digest
routing lands (3).

CGO-free builds embed the IANA tz database (`import _ "time/tzdata"` in
`cmd/companion`) — distroless has no system zoneinfo.

---

## 7. Repo layout & deployment

Single repo, `vikunja-companion`, one binary:

```
vikunja-companion/
├── cmd/companion/        # the self-hosted proxy + features service
├── internal/
│   ├── companion/        # /companion/v1/* HTTP surface + identity cache
│   ├── proxy/            # reverse proxy to Vikunja
│   ├── vikunja/          # thin API client (info, user, tasks)
│   ├── webhook/          # HMAC verify, event parsing, setup helper, build
│   ├── digest/           # morning-briefing builder + cron runner
│   ├── notify/           # notification → dedupe → Sender seam
│   ├── crypto/           # AEAD for secrets at rest
│   ├── store/            # SQLite + migrations
│   ├── httpx/            # shared logger + graceful-shutdown server loop
│   └── config/           # env-first config
├── docs/
│   └── adrs/             # architecture decision records
├── docker-compose.example.yml
└── LICENSE               # AGPLv3
```

- Multi-arch container image, e.g. `ghcr.io/<maintainer>/vikunja-companion`.
- `docker-compose.example.yml` shows the companion in front of an existing
  Vikunja.

### Companion config

```
COMPANION_PUBLIC_URL          # public HTTPS base; used as the webhook target_url
VIKUNJA_UPSTREAM_URL          # the real Vikunja
COMPANION_DB_PATH=/data/companion.db
COMPANION_MASTER_KEY          # 32 bytes; encrypts the webhook HMAC secret at rest
COMPANION_WEBHOOK_EVENTS=task.reminder.fired,task.overdue,tasks.overdue
COMPANION_VIKUNJA_TOKEN       # the digest cron acts as this user; unset -> digest off (see 6.8)
COMPANION_DIGEST_ENABLED=true
COMPANION_APPRISE_API_URL     # apprise-api notify endpoint; unset -> delivery disabled (see 6.5)
COMPANION_APPRISE_URLS        # Apprise service URL(s), comma-separated; optional
COMPANION_APPRISE_TOKEN       # bearer for the apprise-api endpoint; optional
COMPANION_LOG_LEVEL=info
```

---

## 8. Security

- **HMAC** on every inbound Vikunja webhook (`X-Vikunja-Signature`), constant
  time compare, reject on mismatch or missing secret.
- **Rate limiting** on `/companion/v1/*` (per token).
- **Secrets at rest**: the only secret in the DB is the webhook HMAC secret,
  encrypted with `COMPANION_MASTER_KEY`. Vikunja API tokens are never
  stored — per-request ones stay in the header, the digest token is env
  (`COMPANION_VIKUNJA_TOKEN`). The Apprise endpoint and its token are env too.
- **Outbound delivery**: the companion POSTs to the operator-configured
  `apprise-api` URL. Not user-controlled in v1, so no SSRF surface; that
  changes if per-user routing lands (see 9).
- **No caching** of authenticated proxied responses.
- The proxy refuses to start if `VIKUNJA_UPSTREAM_URL` isn't reachable / isn't a
  Vikunja instance (`/api/v1/info` probe).
- No project-operated component: nothing outside the user's control sees task
  content or metadata.

---

## 9. Roadmap (post-v1)

The `notify` package keeps the event→notification mapping in its callers and the
outbound formatting behind the `Sender` interface, so these slot in without
touching delivery:

- **Apprise `Sender`** — the immediate next step; makes v1 actually deliver
  (see 6.5).
- **Per-user routing** — move both the Vikunja token (for the digest) and the
  Apprise config out of env and into `user_settings`, encrypted at rest, set
  from the app. Makes the digest and delivery multi-user. Adds a
  `COMPANION_DELIVERY_ALLOW_HOSTS` SSRF allowlist once the Apprise URL is
  user-controlled, and a `DELETE /companion/v1/settings` to drop a user's stored
  token + webhook secret.
- **Generic JSON webhook `Sender`** — POST straight to a Discord/Slack/homemade
  URL, no apprise-api container. Same interface (see 6.5).
- **Native push** — as another `Sender`, for users who want the Vikunja app's
  own branding on the notification (see 6.6).
- **More notification types via a `/api/v1/notifications` poller.** Needs the
  per-user stored token above. Single endpoint, cursor on `id` (never mark
  read — that's the user's state). Covers comments, assignments, `@mentions`,
  relations, project shares — reliably, no per-project webhooks. Low frequency
  (30–60 s). Also a reconciliation safety net for webhook deliveries missed
  while the companion was down.
- **Optional project-level webhook auto-registration** for users who want
  real-time comment/assignment notifications instead of poll latency. Off by
  default.
- **AI assistant (opt-in).** Provider key at instance level (or BYO per user).
  Generates *suggestions* the user accepts/rejects — never auto-applies. New
  `/companion/v1/suggestions` namespace + `suggestions` table. App shows the UI
  only if `features` includes `"ai"`. Loud disclaimer: task content goes to the
  AI provider.
- Possible: unread badge sync, server-side saved smart-lists, quick-add NLP
  parsing.

---

## 10. Non-goals

- Not a Vikunja fork or replacement; no schema access, API only.
- No offline task store / sync backend (that's a separate app-side effort).
- No own user accounts, registration, or password management.
- No push infrastructure operated by the maintainer.
- No HA / clustering / horizontal scale. One box, one SQLite file.
- Not a general-purpose API gateway; it proxies Vikunja and nothing else.
- Does not serve Vikunja's web frontend.

---

## 11. Verified against a live instance (`/api/v1/docs`)

Done against Vikunja `v2.5.0` + `go-vikunja/vikunja@main` source. Results and the
concrete payload shapes live in
[`webhooks-verified.md`](webhooks-verified.md). Summary:

- [x] `data` shape for the three events — keys are `task` / `user` / `project`
      (+ `reminder`), and `tasks` / `user` / `projects` for the batch. `user` is
      the recipient; there is no `doer` on these events.
- [x] `X-Vikunja-Signature` = **lowercase `hex(HMAC-SHA256(rawBody, secret))`**.
- [x] User-webhook routes confirmed. `POST .../{id}` updates **`events` only** —
      `target_url` and `secret` are immutable, `secret` is never returned.
- [x] User-webhook create rejects non-user-directed events (`400`, field
      `events`) — enforced in `Webhook.Create`.
- [x] Don't pin a version — probe `webhooks_enabled` + `.../webhooks/events` at
      runtime. Test instance: `webhooks_enabled: true`.
- [x] `GET /api/v1/notifications` — `page`/`per_page`, headers
      `x-pagination-total-pages` / `x-pagination-result-count`.
- [x] Routability: SSRF-safe client drops non-routable targets unless
      `webhooks.allownonroutableips`.
- Still needs a token: live event-list JSON, one real delivery end-to-end, the
      `400` rejection, a real notifications page.
- Retries: newer Vikunja retries deliveries (was "never" in this doc);
      unconfirmed for `v2.5.0`. Dedupe regardless.
