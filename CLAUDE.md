# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

**Notification delivery is delegated to Apprise** (the user runs `apprise-api`,
which fans out to ntfy / Discord / Gotify / etc.) — see
`docs/adrs/0001-push-relay-vs-delegated-delivery.md`. The maintainer relay,
sealed-box E2E crypto, `devices`, and `cmd/relay` were removed. One binary,
CGO-free.

`cmd/companion`: opens SQLite + migrations, refuses to start unless
`VIKUNJA_UPSTREAM_URL` answers `GET /api/v1/info`, then serves:

- `GET /companion/v1/info` — capability probe (`features: ["push", "digest"]`).
- `GET /companion/v1/webhook` — issues the target URL + a stable per-user HMAC
  secret + events for the user to paste into Vikunja, plus `last_delivery_at`.
- `POST /companion/v1/webhooks/vikunja` — inbound: identifies the user by trying
  each stored secret, verifies the HMAC, parses the event, builds notifications
  (`internal/webhook/build.go`), dispatches.
- `GET` / `PUT /companion/v1/settings` — notification prefs keyed by `user_id`
  (`{ digest: { enabled, time }, timezone }`), stored in `user_settings`.
- everything else → `internal/proxy`.

A 5-minute cron (`internal/digest`, started from `main`) sends the **morning
briefing** for **one user** — whoever `COMPANION_VIKUNJA_TOKEN` belongs to
(resolved at startup via `GET /api/v1/user`; unset/rejected → digest off). At
their local send time (default 08:00, 2h window) it fetches tasks due through
today via `GET /api/v1/tasks` and delivers one "N tasks for today · M urgent"
notification (`priority >= 4` = urgent; zero tasks = nothing), idempotent via a
`digest:<user>:<date>` key in `notifications_sent`. `cmd/companion` embeds
`time/tzdata` (CGO-free distroless). The digest is single-user in v1 on purpose;
the webhook path stays multi-user (HMAC-identified).

Delivery seam: `internal/notify` — `Dispatcher.Dispatch(ctx, userID,
notifications)` → dedupe → `Sender.Send`. The `Sender` is `notify.Apprise`
(POSTs `{title, body, type}` to `COMPANION_APPRISE_API_URL`, fleet-wide); when
that env is unset `cmd/companion` falls back to a `logSender`. The only secret
in the DB is the webhook HMAC secret; `internal/crypto` does master-key AEAD for
it. No Vikunja token is ever stored (per-request in the header, digest token in
env).

Not yet done: per-type toggles, per-user token/delivery routing (+ teardown
endpoint, notifications poller), a live apprise-api run, a live digest run
against a real Vikunja (`GET /api/v1/tasks` filter unverified). No `LICENSE`
(AGPLv3 — drop it in).

`docs/COMPANION.md` is the source of truth for *why* (sections 6 and 11 before
touching `internal/webhook` or `internal/vikunja`); `docs/adrs/` holds decision
records; `docs/webhooks-verified.md` has the concrete Vikunja payloads and
routes.

## What this is

One Go binary, AGPLv3:

- **`cmd/companion`** — self-hosted. A transparent reverse proxy to a Vikunja
  instance (`/api/v1/*` passthrough, verbatim) that also serves its own features
  under `/companion/v1/*`. The user configures *one* URL in the iOS app. State:
  one SQLite file.

The companion does not deliver notifications itself: it POSTs each one to the
user's `apprise-api` endpoint, which fans out to whatever service they
configured it for. The iOS app (separate repo) is the only client of
`/companion/v1/*`; it does not receive the notifications — the user's Apprise
targets do.

## Architecture invariants (do not violate)

- **Optional & lightweight.** No Postgres, no message queue, no clustering, no
  HA, no push infrastructure. One box, one SQLite file. A user not running the
  companion must lose nothing.
- **Never a fork.** The companion only calls Vikunja's public HTTP API. All
  Vikunja-version coupling stays in `internal/vikunja`. No schema access.
- **Proxy is API-only and uncached.** Everything not under `/companion/` is
  proxied verbatim (method, path, query, headers, body, status, streaming). No
  caching of authenticated responses; only `Host`/`X-Forwarded-*` rewriting. The
  companion does not serve Vikunja's web SPA.
- **No accounts, no `users` table.** "User" = whoever Vikunja says the
  `Authorization: Bearer` token belongs to. Identify callers on `/companion/v1/*`
  by forwarding the token to `GET /api/v1/user` upstream and caching
  `sha256(token) → {user_id, ttl}`. Tables are keyed by Vikunja `user_id` with
  nothing behind it. The webhook path is multi-user (HMAC-identified); the
  digest cron is deliberately single-user in v1 (`COMPANION_VIKUNJA_TOKEN`) —
  per-user token routing is a documented post-v1 step.
- **No proprietary delivery channel.** The companion never runs its own push
  service. It formats a notification and POSTs it to an Apprise endpoint
  (`COMPANION_APPRISE_*` env). Keep `internal/notify` delivery-agnostic: the
  event→notification mapping stays in the callers (`internal/webhook`,
  `internal/digest`); outbound formatting stays behind the `Sender` interface.
  A generic-webhook or native-push path, if ever added, is just another
  `Sender`.
- **v1 notification surface is exactly three user-level webhook events:**
  `task.reminder.fired`, `task.overdue`, `tasks.overdue`. No project-level
  webhooks in v1 (that's a post-v1 `/api/v1/notifications` poller). Prefer the
  `tasks.overdue` batch event over N x `task.overdue`.
- **The companion never calls Vikunja's webhook API.** A Vikunja API token
  cannot reach `/api/v1/user/settings/webhooks*` (see `docs/webhooks-verified.md`).
  The **user creates the webhook by hand** in Vikunja's UI; the companion serves
  `GET /companion/v1/webhook` (`{target_url, secret, events, last_delivery_at}`)
  for the app to display, stores the secret, and only verifies inbound
  deliveries. No reconciliation, no self-healing — it records `last_delivery_at`
  and the app nudges after a long silence.
- **Webhook auth:** verify `X-Vikunja-Signature` = lowercase
  `hex(HMAC-SHA256(rawBody, secret))` on every inbound webhook, constant-time
  compare, reject on mismatch/missing.
- **Secrets at rest:** the webhook HMAC secret is the only one in the DB,
  encrypted with `COMPANION_MASTER_KEY`. Vikunja API tokens are never persisted
  (per-request in the header; digest token in env). The Apprise endpoint/token
  are env too. The Vikunja-side webhook is the user's to remove.

## Commands

Standard Go toolchain — no framework. Targets in `Makefile`:

```
make test            # go test ./...
make build           # CGO_ENABLED=0 -> bin/companion
make vet
make tidy
go test ./internal/webhook/                        # one package
go test -run TestVerifySignature ./internal/webhook/   # one test
go run ./cmd/companion    # auto-loads ./.env if present (godotenv); see .env.example
```

- **Builds must stay CGO-free** (`CGO_ENABLED=0`). The SQLite driver is
  `modernc.org/sqlite` (pure Go) specifically to keep the container a
  distroless static binary — do not switch to `mattn/go-sqlite3`.
- The `store` layer pins `SetMaxOpenConns(1)`; it is a single small service,
  not a place to add a connection pool.
- Deployment is one multi-arch image, one binary
  (`docker-compose.example.yml`). Config is env-first (`.env.example`,
  `docs/COMPANION.md` section 7).

## Package notes

- `internal/notify` is the reusable delivery seam (`Dispatcher.Dispatch(ctx,
  userID, notifications)` -> dedupe -> `Sender.Send`). It must not import
  `internal/vikunja` or `internal/webhook` or learn anything Vikunja-specific.
  The event->notification mapping lives in the caller (`internal/webhook/build.go`,
  `internal/digest`); outbound formatting lives behind `Sender`. The one impl is
  `notify.Apprise` (`apprise.go`); `cmd/companion` uses it when
  `COMPANION_APPRISE_API_URL` is set, else a log-only fallback. A post-v1
  `/api/v1/notifications` poller builds its own `[]notify.Notification` and calls
  `Dispatch` the same way.
- `internal/digest` is the second `notify` caller (after `internal/webhook`): a
  cron, not a webhook. `NewRunner` takes the resolved `token` + `userID` from
  `main` (via `COMPANION_VIKUNJA_TOKEN`) and acts only for that one user;
  `Runner.Run(ctx)` is the 5-min loop `main` starts in a goroutine. `Build` maps
  tasks-due-today to the one briefing notification. It may import
  `internal/vikunja` (like `webhook`); `notify` may not. Idempotency is the
  `Runner`'s own `digest:<user>:<date>` MarkSent guard, not the dispatcher's
  dedupe. `ParseHHMM` is shared with the settings handler's input validation.
- `internal/webhook` handler identifies the sending user by trying each stored
  HMAC secret against the raw body — the webhook `target_url` is identical for
  every user, so there is no per-request user hint before verification.
- Webhook setup is manual (see invariants). `GET /companion/v1/webhook`
  generates + stores a per-user secret (encrypted) and returns it with the
  target URL, event list, and `last_delivery_at`, all idempotent — a repeat call
  returns the same secret so the user's Vikunja config stays valid.
  `COMPANION_WEBHOOK_EVENTS` is the operator ceiling for which events that
  endpoint advertises and which the inbound handler forwards.
- `main.Version` is `-ldflags "-X main.Version=..."` injected at build time.
- `internal/companion` assembles `cmd/companion`'s HTTP surface — `NewRouter`
  mounts the `/companion/v1/*` routes and sends everything else to
  `internal/proxy`. The identity cache lives here too. `internal/httpx` holds
  the logger + graceful-shutdown server loop.
- `internal/store` runs plain `.sql` files embedded from `migrations/`, tracked
  in a `schema_migrations` table, each in its own transaction. Normally: add a
  migration, never edit an applied one — but pre-prod the init migrations get
  edited in place and the DB recreated. `Open(":memory:")` works for tests.
  There is no `users` table.
- `config.LoadDotenv` (called first in `main`) loads `./.env` via
  `github.com/joho/godotenv` for local dev only — no-op when the file is absent,
  never overrides a real env var. Config stays env-first; nothing else reads the
  file. `.env` is gitignored and not in the image.
