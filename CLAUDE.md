# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

**Fase 0 scaffold.** The repo compiles and both binaries run: they load and
validate their env config (`internal/config`), set up JSON logging, and serve
`/healthz` with graceful shutdown. `internal/config` is real and tested;
`internal/httpx` holds the shared server loop. Every other `internal/*` package
from `docs/COMPANION.md` §7 exists as a **doc-comment-only stub** with a
`TODO(fase-1)` / `TODO(fase-2)` marker — no proxy, no store, no vikunja client,
no webhook/notify/crypto/relay logic yet.

Roadmap:

- **Fase 1** (no live Vikunja needed): `internal/proxy` (verbatim `/api/v1/*`),
  `internal/store` (SQLite schema + migrations), `internal/vikunja`
  (`info`, `user`), `/companion/v1/info`, token→user identity cache, upstream
  reachability check on boot.
- **Fase 2** (blocked on the §11 checklist against a live `/api/v1/docs`):
  `internal/webhook`, `internal/notify`, `internal/crypto`, `internal/relay`.

Not yet present: `LICENSE` (AGPLv3 text — drop it in), `go.sum` (no deps yet).

`docs/COMPANION.md` is the source of truth for *why*. Read §6 (push) and §11
(verification checklist) before touching `internal/webhook` or
`internal/vikunja`.

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
  `NotificationSource` interface (§9) so post-v1 sources slot in without touching
  delivery/crypto.
- **v1 push surface is exactly three user-level webhook events:**
  `task.reminder.fired`, `task.overdue`, `tasks.overdue`. One webhook
  registration per user via `PUT /api/v1/user/settings/webhooks`. No
  project-level webhooks in v1 (that's a post-v1 `/api/v1/notifications` poller).
  Prefer the `tasks.overdue` batch event over N × `task.overdue`.
- **Webhook auth:** verify `X-Vikunja-Signature` = `hex(HMAC-SHA256(rawBody, secret))`
  on every inbound webhook, constant-time compare, reject on mismatch/missing.
- **Self-healing registration:** reconcile the user's webhook on every device
  re-registration and on a slow timer, so a webhook deleted in Vikunja's UI
  comes back.
- **Secrets at rest:** the stored Vikunja API token is encrypted with
  `COMPANION_MASTER_KEY`. Deleting a user's last device deletes both the stored
  token and the webhook registration.

## Commands

Standard Go toolchain — no framework. Targets in `Makefile`:

```
make test            # go test ./...
make build           # CGO_ENABLED=0 -> bin/companion, bin/relay
make vet
make tidy
go test ./internal/webhook/                        # one package
go test -run TestVerifySignature ./internal/webhook/   # one test
go run ./cmd/companion    # reads env; see .env.example
```

- **Builds must stay CGO-free** (`CGO_ENABLED=0`). The SQLite driver is
  `modernc.org/sqlite` (pure Go) specifically to keep the container a
  distroless static binary — do not switch to `mattn/go-sqlite3`.
- The `store` layer pins `SetMaxOpenConns(1)`; it is a single small service,
  not a place to add a connection pool.
- Deployment is one multi-arch image with two binaries
  (`docker-compose.example.yml`); the relay runs via `--entrypoint`. Config is
  env-first (`.env.example`, `docs/COMPANION.md` §7).

## Package notes

- `internal/notify` is the reusable delivery seam — it must not import
  `internal/vikunja` or learn anything Vikunja-specific. The event→notification
  mapping lives in the `Source` (today `internal/webhook/build.go`); post-v1
  sources (a `/api/v1/notifications` poller) plug in here without touching
  sealing or delivery.
- `internal/webhook/handler.go` identifies the sending user by trying each
  stored HMAC secret against the raw body — the webhook `target_url` is
  identical for every user, so there is no per-request user hint before
  verification.
- Webhook event preferences are per-user, stored in the `webhooks.events`
  column; `Registrar.desiredEvents` intersects them with the operator ceiling
  (`COMPANION_WEBHOOK_EVENTS`). Reconcile runs on device (de)registration, on
  settings change, and on an hourly timer.
- `main.Version` is `-ldflags "-X main.Version=..."` injected per binary.
