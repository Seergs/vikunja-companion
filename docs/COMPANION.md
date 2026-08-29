# Vikunja Companion Service — Design

Design doc for **`vikunja-companion`**, an optional, self-hosted, lightweight Go
service that sits in front of a Vikunja instance and adds features the native
iOS client can't get from Vikunja alone — starting with real push
notifications.

Status: **design only**, nothing built yet. This doc is the "why".

---

## 1. Context & philosophy

The iOS app points at a user's own Vikunja instance and re-fetches
everything from the server — no backend of its own. That keeps the app simple
but leaves a few things impossible or clumsy:

- **Push notifications.** Vikunja only does email + in-app notifications. With
  the app fully closed there is no way to learn a task went overdue.
- **Anything that needs a process running while the app is closed** — a morning
  digest, background curation, unread badges.

`vikunja-companion` is the opt-in answer. Design constraints, in priority order:

1. **Optional.** A user who doesn't run it loses nothing — the app detects its
   absence and hides the extra features.
2. **Lightweight.** One small Go service, one SQLite file, one container. No
   message queue, no Postgres, no clustering.
3. **One URL.** The companion is a **transparent reverse proxy** to Vikunja for
   everything under `/api/v1/*`, and serves its own features under
   `/companion/v1/*`. The user puts **one** URL in the app (the companion's),
   not two.
4. **No trust required for what it can't verify.** The one piece that genuinely
   can't be self-hosted (APNs) is a content-blind relay; everything it forwards
   is end-to-end encrypted.
5. **Never a fork.** It calls Vikunja's public HTTP API like any other client.
   A Vikunja API change is contained the same way it is in the app — to the
   layer that speaks HTTP.

License: **AGPLv3** (both binaries), matching Vikunja and, for the
relay, guaranteeing the running build matches the source.

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
   │          │           │  ┌───────────────────────────────────┐  │        │
   │          │◄─────────►│  │ /companion/v1/*  (own features)    │  │        │ user-level
   │  APNs    │           │  │  info · devices · settings         │  │        │ webhook
   │  (system)│           │  │  webhooks/vikunja  ◄───────────────┼──┼────────┘ (task.overdue,
   └────┬─────┘           │  └───────────────┬───────────────────┘  │           reminder.fired)
        │                 │       SQLite     │  encrypt payload     │
        │                 └─────────────────┼──────────────────────┘
        │                                   │  POST {apns_token, ciphertext}
        │                                   ▼
        │                        ┌────────────────────┐
        └────────────────────────│   push relay       │  operated by the project maintainer
              decrypted in NSE   │  (content-blind)   │  holds the APNs .p8, sees only
                                 │  APNs .p8 lives    │  device token + ciphertext
                                 │  here only         │
                                 └─────────┬──────────┘
                                           ▼
                                      Apple APNs
```

Three moving parts:

| Part | Who hosts it | State | Sees task content? |
|---|---|---|---|
| `companion` binary | the user (self-hosted) | SQLite: devices, tokens, cursors | yes — it's the user's own box |
| `relay` binary | the project maintainer (one shared instance) | SQLite: opaque registration tokens only | **no** — ciphertext + APNs device token only |
| iOS app + Notification Service Extension | the user's device | keychain: X25519 private key | yes — decrypts in the NSE |

Why the relay can't be self-hosted: Apple only accepts pushes signed with the
APNs key of the Apple Developer account that ships the App Store build. A
self-hoster doesn't have that key. Precedent: Nextcloud's `push.nextcloud.com`.
The escape hatch for purists is in 6.6.

---

## 3. Identity model

The companion has **no accounts, no login, no registration** of its own.

- The app authenticates to Vikunja with a long-lived **API token** (the app
  only models API-token auth today). Every proxied request already carries it.
- To identify a caller on a `/companion/v1/*` route, the companion takes the
  same `Authorization: Bearer <token>` and calls `GET /api/v1/user` upstream,
  then caches `sha256(token) → {user_id, ttl}` (a few minutes).
- "User" = whatever Vikunja says that token belongs to. Multi-user falls out
  for free: the `devices` table has a `user_id` column and the crons loop over
  users. The companion never manages users — it just doesn't assume there's
  only one (a household sharing one Vikunja shares one companion).

The companion **stores the Vikunja API token** (encrypted at rest with a
companion master key from env) because it needs to act as the user between app
sessions — to read tasks for the digest cron and to poll
`/api/v1/notifications` post-v1. It is **not** used to manage the webhook: a
Vikunja API token cannot call `/api/v1/user/settings/webhooks*` at all (see 6.3
and `webhooks-verified.md`). This is the same token already flowing through the
proxy; storing it is a sensitivity point called out in 8.

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
  "features":  ["push"]
}
```

The app already probes `GET /api/v1/info` during onboarding to confirm it's a
real Vikunja instance. It gains a parallel probe of `/companion/v1/info`:

- reachable → set a `hasCompanion` flag on the `InstanceAccount`, unlock the
  feature UI gated on each entry in `features`.
- 404 / not reachable → behave exactly as today (bare Vikunja).

This is the same runtime capability-detection pattern the app already uses for
Vikunja server features (`CapabilityProvider.supports(_:)`), extended to "is
there a companion, and what does it offer".

---

## 6. Feature: push notifications (v1)

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
- Runs every minute but only notifies a given user **when the clock crosses
  that user's `overdue_tasks_reminders_time`** (default **09:00**, in the
  user's timezone). Once per day.
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
  treats them as an email feature. Many self-hosters run without SMTP. For
  them, the **webhook is the only way** to ever learn about a reminder or an
  overdue task. Webhooks fire whenever `webhooks.enabled` is true (the default).

Project-level events (comments, assignments, mentions) are **not** in v1. The
future path for those is a `/api/v1/notifications` poller (9), not
project-webhook registration.

### 6.3 Webhook registration — manual, one-time (decision, 2026-08-29)

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

1. The app offers a **"Set up push" screen**. It calls
   `GET /companion/v1/webhooks/setup` (authenticated with the user's token),
   which returns `{ target_url, secret, events }` — the companion generates and
   stores the `secret` (HMAC key) keyed by `user_id`, idempotently.
2. The user opens Vikunja → **Settings → Webhooks → Create**, and pastes:
   - **Target URL:** `{COMPANION_PUBLIC_URL}/companion/v1/webhooks/vikunja`
   - **Secret:** the value from step 1
   - **Events:** the three (`task.reminder.fired`, `task.overdue`,
     `tasks.overdue`)
3. The app confirms by asking the companion whether a verified delivery has
   arrived (a test can be forced by setting a reminder ~1 min out).

Consequences of no reconciliation:

- If the user deletes the webhook in Vikunja, push stops and the companion
  **cannot detect it** (listing webhooks is the same blocked route). Mitigation:
  the companion records `last_delivery_at` and exposes it in
  `/companion/v1/info`; the app shows a soft warning after a long silence
  ("no signal from Vikunja in N days — check Settings → Webhooks").
- `events` drift (per-type toggles) also can't be pushed to Vikunja. v1: the app
  tells the user which events to tick; the companion just filters what it
  forwards.

Prerequisites, surfaced by the companion in `/companion/v1/info` and in its
startup logs if unmet:

- `webhooks.enabled` = true on the Vikunja instance (default true). Read from
  `GET /api/v1/info` → `webhooks_enabled`.
- If the companion is on a non-routable IP relative to Vikunja, the instance
  needs `webhooks.allownonroutableips` = true. Better: give the companion a
  real hostname.

### 6.4 Delivery pipeline

```
Vikunja ──POST──► /companion/v1/webhooks/vikunja
                    │  verify HMAC (X-Vikunja-Signature = hex(HMAC-SHA256(rawBody, secret)))
                    │  parse {event_name, time, data}
                    ▼
                  resolve target user(s) from data
                    │  drop self-authored noise where applicable
                    ▼
                  build notification {title, body, deeplink, dedupe_key}
                    │  dedupe against notifications_sent (event fingerprint)
                    ▼
                  for each of the user's devices:
                    seal payload to device.public_key  (NaCl sealed box)
                    POST relay /relay/v1/push {apns_token, ciphertext, collapse_id}
```

- `tasks.overdue` (batch) is preferred over N × `task.overdue` to avoid a
  9 a.m. notification storm; use `task.overdue` only when the batch has one
  task.
- Deep links use the app's existing URL scheme (`VikunjaWidgetConfig` scheme):
  reminder/overdue → the task; batch overdue → the Today screen.
- **Payload shapes** are verified — see `webhooks-verified.md`. Header is
  `X-Vikunja-Signature` = lowercase `hex(HMAC-SHA256(rawBody, secret))`; body is
  `{event_name, time, data}`; for these three events `data` carries `task` /
  `user` / `project` (`user` is the **recipient**, not a `doer` — there is no
  self-authored noise to drop), and the batch carries `tasks` / `user` /
  `projects` (a map keyed by project id).

### 6.5 End-to-end encryption

The relay must never see task content. Scheme:

- On first launch of the feature, the app generates an **X25519 keypair** in
  the keychain (in the shared access group, so the Notification Service
  Extension can read the private key — the app already uses a shared keychain
  group for the widget).
- The app registers the **public key** with the companion (6.4 device
  registration).
- The companion encrypts each notification's JSON with a **NaCl sealed box**
  (`crypto_box_seal`: ephemeral X25519 + XSalsa20-Poly1305, ~48 bytes
  overhead) to that public key.
- APNs payload carries a generic visible fallback plus the ciphertext:
  ```json
  { "aps": { "mutable-content": 1,
             "alert": { "title": "Vikunja", "body": "New notification" },
             "sound": "default" },
    "e": "<base64 ciphertext>" }
  ```
- The **Notification Service Extension** decrypts `e` and rewrites
  `bestAttemptContent.title` / `.body` / `.userInfo`. If it can't (timeout,
  missing key), the generic fallback shows.
- Notification JSON is tiny (title, body, deeplink, task id) — well under the
  APNs 4 KB limit even sealed.

The relay sees: an opaque APNs device token, a ciphertext, and timing. Not the
task, not the Vikunja URL, not the user's identity.

### 6.6 The push relay

A second binary in the same repo (7). Stateless except for a registry of
opaque tokens.

- `POST /relay/v1/register` → mint a random token, store `{token, created,
  last_seen}`. No PII. The companion calls this once on first boot and caches
  the token (or the maintainer pre-issues one via env).
- `POST /relay/v1/push` (auth: relay token) → body `{apns_token, ciphertext,
  collapse_id?, priority?, expiration?}` → forwards to APNs with the `.p8`.
- Rate-limited per token; the point is to protect the shared APNs key from
  abuse, not to meter legitimate use (a personal task manager's volume is
  negligible).
- Config: `RELAY_APNS_KEY_PATH`, `RELAY_APNS_KEY_ID`, `RELAY_APNS_TEAM_ID`,
  `RELAY_APNS_TOPIC` (the app's bundle id), `RELAY_DB_PATH`.

**Escape hatch — fully self-hosted push:** a user with their own Apple Developer
account can build the app from source with their own bundle id and set
`COMPANION_APNS_*` on the companion directly. The companion then talks to APNs
itself and the relay is bypassed entirely. Documented, not the default.

### 6.7 iOS app changes implied

Tracked here so they're not a surprise:

- New **Notification Service Extension** target (mirrors the widget extension
  setup: shared keychain access group, `VikunjaWidgetKit`-style thin
  composition). Decrypts the sealed payload.
- `CompanionServiceProtocol` in `VikunjaCore/Protocols/`; concrete impl in
  `VikunjaNetworking`; wired in `AppContainer`. Features never import it
  directly.
- Register for remote notifications; on APNs token grant, call
  `POST /companion/v1/devices { apns_token, public_key, app_version }`.
- `InstanceAccount` gains a `hasCompanion` flag (or a small companion-info
  value); onboarding + `Settings`' connection form probe `/companion/v1/info`
  alongside `/api/v1/info`.
- A **Notifications** screen (likely folded into `Features/Settings`, or a new
  `Features/Notifications`) — visible only when `hasCompanion` — with per-type
  toggles (reminders, overdue) that narrow what the companion forwards.
- A **"Set up push"** step: fetch `GET /companion/v1/webhooks/setup`, show the
  `target_url` + `secret` + events for the user to paste into Vikunja → Settings
  → Webhooks, then confirm a delivery arrived. Also surface the "no signal in N
  days" warning from `/companion/v1/info`'s `last_delivery_at`.
- Keychain: X25519 private key in the shared access group.

---

## 7. Repo layout & deployment

Single repo, `vikunja-companion`, two binaries:

```
vikunja-companion/
├── cmd/companion/        # the self-hosted proxy + features service
├── cmd/relay/            # the content-blind APNs relay (maintainer-operated)
├── internal/
│   ├── proxy/            # reverse proxy to Vikunja
│   ├── vikunja/          # thin API client (info, user, tasks)
│   ├── webhook/          # HMAC verify, event parsing, setup-helper
│   ├── notify/           # event → notification, dedupe, NotificationSource seam
│   ├── crypto/           # NaCl sealed box
│   ├── relay/            # relay client (companion side) + relay server
│   ├── store/            # SQLite (companion) / SQLite (relay)
│   └── config/           # env + optional config.yaml
├── docs/
├── docker-compose.example.yml
└── LICENSE               # AGPLv3
```

- Multi-arch container image, e.g. `ghcr.io/<maintainer>/vikunja-companion`.
- `docker-compose.example.yml` shows companion in front of an existing Vikunja.
- The relay is deployed once by the maintainer; its image is the same repo with
  a different entrypoint.

### Companion config

```
COMPANION_PUBLIC_URL          # public HTTPS base; used as the webhook target_url
VIKUNJA_UPSTREAM_URL          # the real Vikunja
COMPANION_DB_PATH=/data/companion.db
COMPANION_MASTER_KEY          # 32 bytes; encrypts stored Vikunja tokens at rest
COMPANION_RELAY_URL           # default: the project relay
COMPANION_RELAY_TOKEN         # optional; auto-registered on first boot if unset
COMPANION_WEBHOOK_EVENTS=task.reminder.fired,task.overdue,tasks.overdue
COMPANION_APNS_KEY_PATH       # optional: BYO APNs, bypasses the relay
COMPANION_APNS_KEY_ID
COMPANION_APNS_TEAM_ID
COMPANION_APNS_TOPIC
COMPANION_LOG_LEVEL=info
```

---

## 8. Security

- **HMAC** on every inbound Vikunja webhook (`X-Vikunja-Signature`), constant
  time compare, reject on mismatch or missing secret.
- **Rate limiting** on `/companion/v1/*` (per token) and on the relay (per
  registration token).
- **Secrets at rest**: the Vikunja API token and the webhook HMAC secret are
  encrypted with `COMPANION_MASTER_KEY`. Deleting a user's last device deletes
  both. The Vikunja-side webhook is the user's to remove (the companion can't).
- **E2E encryption** means a compromised relay leaks nothing about content;
  sealed-box ephemeral keys give per-message forward secrecy.
- **No caching** of authenticated proxied responses.
- The proxy refuses to start if `VIKUNJA_UPSTREAM_URL` isn't reachable / isn't
  a Vikunja instance (`/api/v1/info` probe).
- Relay stores **no PII** — opaque tokens and APNs device tokens only.

---

## 9. Roadmap (post-v1)

The `notify` package defines a `NotificationSource` interface from day one so
these slot in without touching delivery/encryption:

- **More notification types via a `/api/v1/notifications` poller.** Per-user,
  single endpoint, cursor on `id` (never mark read — that's the user's state).
  Covers comments, assignments, `@mentions`, relations, project shares —
  reliably, no per-project webhooks. Low frequency (30–60 s). Also acts as a
  reconciliation safety net for webhook deliveries missed while the companion
  was down (Vikunja does not retry webhooks).
- **Optional project-level webhook auto-registration** for users who want
  real-time comment/assignment push instead of poll latency. Off by default.
- **Daily digest.** Companion cron at a user-configured time (default 08:00,
  their tz) fetches tasks and sends one *forward-looking* push — "8 tasks
  today, 1 urgent" — bucketed with the **same rule as
  `VikunjaCore.TodayDigest`** so app, widget and digest agree. Complements
  Vikunja's retrospective overdue notification. Tappable → Today screen.
- **AI assistant (opt-in).** Provider key at instance level (or BYO per user).
  Generates *suggestions* the user accepts/rejects — never auto-applies:
  suggested labels, better titles/descriptions, "this task is thin, add
  detail?", relation suggestions, subtask splits. New `/companion/v1/suggestions`
  namespace + `suggestions` table. On-demand first, background pass later.
  Loud disclaimer: task content goes to the AI provider. App shows the UI only
  if `features` includes `"ai"`.
- **Web push / Android (FCM)** later — same relay pattern, different transport.
- Possible: unread badge sync, server-side saved smart-lists, quick-add NLP
  parsing.

---

## 10. Non-goals

- Not a Vikunja fork or replacement; no schema access, API only.
- No offline task store / sync backend (that's a separate app-side effort).
- No own user accounts, registration, or password management.
- No HA / clustering / horizontal scale. One box, one SQLite file.
- Not a general-purpose API gateway; it proxies Vikunja and nothing else.
- Does not serve Vikunja's web frontend.

---

## 11. To verify against a live instance (`/api/v1/docs`)

Done, 2026-08-29, against Vikunja `v2.5.0` + `go-vikunja/vikunja@main` source.
Results and the concrete payload shapes live in
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
- Still needs a token: live event-list JSON, one real delivery end-to-end,
      the `400` rejection, a real notifications page.
- Retries: newer Vikunja retries deliveries (was "never" in this doc);
      unconfirmed for `v2.5.0`. Dedupe regardless.
