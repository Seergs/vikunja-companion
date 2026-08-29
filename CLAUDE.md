# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

**Fase 2 — push works end to end (unit-tested; not yet run against a live
relay/APNs).** Both binaries build CGO-free and boot.

`cmd/companion`: opens SQLite + migrations, refuses to start unless
`VIKUNJA_UPSTREAM_URL` answers `GET /api/v1/info`, registers a relay token on
first boot (persisted in `meta`; non-fatal if the relay is down), then serves:

- `GET /companion/v1/info` — capability probe (`features: ["push"]`).
- `GET /companion/v1/webhook` — issues the target URL + a stable per-user HMAC
  secret + events for the user to paste into Vikunja, plus `last_delivery_at`.
- `POST /companion/v1/webhooks/vikunja` — inbound: identifies the user by trying
  each stored secret, verifies the HMAC, parses the event, builds notifications
  (`internal/webhook/build.go`), dispatches.
- `POST` / `DELETE /companion/v1/devices` — register/unregister; last-device
  removal deletes the user row (cascades the secret).
- everything else → `internal/proxy`.

`cmd/relay`: `internal/store` (OpenRelay), `internal/relay` server —
`POST /relay/v1/register`, `POST /relay/v1/push` (per-token rate limit,
content-blind APNs envelope, `apns2`-backed sender, 410 on bad device token).

Delivery seam: `internal/notify` (dedupe -> `internal/crypto` sealed box ->
`internal/relay` client). `internal/crypto` also does master-key AEAD for the
stored Vikunja token and webhook secret.

Not yet done: `/companion/v1/settings` (per-type toggles), the post-v1
notifications poller and digest cron, and a live relay/APNs run. No `LICENSE`
(AGPLv3 — drop it in).

`docs/COMPANION.md` is the source of truth for *why* (sections 6 and 11 before
touching `internal/webhook` or `internal/vikunja`);
`docs/webhooks-verified.md` has the concrete Vikunja payloads and routes.

## What this is

Two Go binaries in one repo, both AGPLv3:

- **`cmd/companion`** — self-hosted. A transparent reverse proxy to a Vikunja
  instance (`/api/v1/*` passthrough, verbatim) that also serves its own features
  under `/companion/v1/*`. The user configures *one* URL in the iOS app. State:
  one SQLite file.
- **`cmd/relay`** — operated once by the project maintainer. Content-blind APNs
  forwarder. Holds the Apple `.p8` key; sees only an opaque device token +
  ciphertext. Cannot be self-hosted (Apple only accepts pushes signed by the App
  Store build's developer account). State: SQLite of opaque tokens, no PII.

The iOS app (separate repo) and its Notification Service Extension are the only
clients; the NSE holds the X25519 private key and decrypts payloads on-device.

## Architecture invariants (do not violate)

- **Optional & lightweight.** No Postgres, no message queue, no clustering, no
  HA. One box, one SQLite file per binary. A user not running the companion must
  lose nothing.
- **Never a fork.** The companion only calls Vikunja's public HTTP API. All
  Vikunja-version coupling stays in `internal/vikunja`. No schema access.
- **Proxy is API-only and uncached.** Everything not under `/companion/` is
  proxied verbatim (method, path, query, headers, body, status, streaming). No
  caching of authenticated responses; only `Host`/`X-Forwarded-*` rewriting. The
  companion does not serve Vikunja's web SPA.
- **No accounts.** "User" = whoever Vikunja says the `Authorization: Bearer`
  token belongs to. Identify callers on `/companion/v1/*` by forwarding the token
  to `GET /api/v1/user` upstream and caching `sha256(token) → {user_id, ttl}`.
  Multi-user must work (a `user_id` column, crons loop over users) without ever
  managing users.
- **Relay never sees content.** Notification JSON is sealed with a NaCl sealed
  box (`crypto_box_seal`) to the device's X25519 public key before it leaves the
  companion. Keep the `notify` delivery path encryption-agnostic via the
  `NotificationSource` interface (section 9) so post-v1 sources slot in without touching
  delivery/crypto.
- **v1 push surface is exactly three user-level webhook events:**
  `task.reminder.fired`, `task.overdue`, `tasks.overdue`. No project-level
  webhooks in v1 (that's a post-v1 `/api/v1/notifications` poller). Prefer the
  `tasks.overdue` batch event over N x `task.overdue`.
- **The companion never calls Vikunja's webhook API.** A Vikunja API token
  cannot reach `/api/v1/user/settings/webhooks*` (see `docs/webhooks-verified.md`).
  The **user creates the webhook by hand** in Vikunja's UI; the companion serves
  `GET /companion/v1/webhooks/setup` (`{target_url, secret, events}`) for the app
  to display, stores the secret, and only verifies inbound deliveries. No
  reconciliation, no self-healing — it records `last_delivery_at` and the app
  nudges after a long silence.
- **Webhook auth:** verify `X-Vikunja-Signature` = lowercase
  `hex(HMAC-SHA256(rawBody, secret))` on every inbound webhook, constant-time
  compare, reject on mismatch/missing.
- **Secrets at rest:** the stored Vikunja API token and the webhook HMAC secret
  are encrypted with `COMPANION_MASTER_KEY`. Deleting a user's last device
  deletes the stored token and secret (the Vikunja-side webhook is the user's to
  remove).

## Commands

Standard Go toolchain — no framework. Targets in `Makefile`:

```
make test            # go test ./...
make build           # CGO_ENABLED=0 -> bin/companion, bin/relay
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
- Deployment is one multi-arch image with two binaries
  (`docker-compose.example.yml`); the relay runs via `--entrypoint`. Config is
  env-first (`.env.example`, `docs/COMPANION.md` section 7).

## Package notes

- `internal/notify` is the reusable delivery seam (`Dispatcher.Dispatch(ctx,
  devices, notifications)` -> dedupe -> seal -> relay push). It must not import
  `internal/vikunja` or `internal/webhook` or learn anything Vikunja-specific.
  The event->notification mapping lives in the caller (`internal/webhook/build.go`);
  a post-v1 `/api/v1/notifications` poller builds its own `[]notify.Notification`
  and calls `Dispatch` the same way.
- `internal/webhook` handler identifies the sending user by trying each stored
  HMAC secret against the raw body — the webhook `target_url` is identical for
  every user, so there is no per-request user hint before verification.
- Webhook setup is manual (see invariants). `GET /companion/v1/webhook`
  generates + stores a per-user secret (encrypted) and returns it with the
  target URL, event list, and `last_delivery_at`, all idempotent — a repeat call
  returns the same secret so the user's Vikunja config stays valid.
  `COMPANION_WEBHOOK_EVENTS` is the operator ceiling for which events that
  endpoint advertises and which the inbound handler forwards.
- `main.Version` is `-ldflags "-X main.Version=..."` injected per binary.
- `internal/companion` (not in section 7's list) assembles `cmd/companion`'s HTTP
  surface — `NewRouter` mounts the `/companion/v1/*` routes and sends everything
  else to `internal/proxy`. The identity cache lives here too. `internal/httpx`
  holds the logger + graceful-shutdown server loop shared by both `main`s.
- `internal/store` runs plain `.sql` files embedded from `migrations/`
  (companion, `Open`) or `migrations_relay/` (relay, `OpenRelay`), tracked in a
  `schema_migrations` table, each in its own transaction. Add a migration, never
  edit an applied one. `Open(":memory:")` works for tests. One `DB` type serves
  both schemas; relay-only methods (`MintToken`, `ValidToken`) just aren't
  called on a companion `DB`.
- `config.LoadDotenv` (called first in both `main`s) loads `./.env` via
  `github.com/joho/godotenv` for local dev only — no-op when the file is absent,
  never overrides a real env var. Config stays env-first; nothing else reads the
  file. `.env` is gitignored and not in the image.
