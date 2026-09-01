# ADR 0001 — Notification delivery: delegate to user-run services, not a maintainer relay

- **Status:** Accepted (2026-09-01)
- **Supersedes:** the push design in earlier revisions of `docs/COMPANION.md`
  (maintainer-operated APNs relay + end-to-end-encrypted payloads)

## Context

The companion turns Vikunja events (`task.reminder.fired`, `task.overdue`,
`tasks.overdue`) and a daily digest into notifications that must reach the user
while the iOS app is closed. The question is how the notification leaves the
companion and gets to the device.

Constraints that shape the answer:

- The companion is **self-hosted and optional**. Every user runs their own on
  their own box.
- The iOS app is distributed through Apple's App Store under one Apple Developer
  account (the maintainer's).
- **APNs only accepts a push signed with the APNs key (`.p8`) of the Apple
  Developer account that published the app.** A self-hoster does not have, and
  cannot be given, that key. This is a hard platform rule, not a design choice.
- Project philosophy: lightweight, no maintainer-operated infrastructure that a
  user cannot see or replace, no single point of failure, no ongoing cost the
  maintainer must carry for other people's installs.

### Option A — maintainer-operated APNs relay (the original plan)

A second binary, run once by the maintainer, holding the `.p8`. The companion
seals each notification payload to the device's X25519 public key (NaCl sealed
box) so the relay is content-blind, then POSTs `{apns_token, ciphertext}` to the
relay, which wraps it in an APNs envelope and forwards it to Apple. The iOS app
gains a Notification Service Extension that decrypts on-device. Precedent:
Nextcloud's `push.nextcloud.com`, Home Assistant's cloud push, ntfy's own iOS
push.

What this buys: notifications that look fully native — the Vikunja app's name
and icon, tap-through into a specific screen, no extra app to install.

What it costs:

- **A single point of failure** owned by the maintainer. If the relay is down,
  every self-hoster's notifications stop. If the `.p8` leaks, pushes to every
  install can be forged until the key is rotated (which itself breaks every
  install until they update).
- **A permanent operational and financial burden** on the maintainer for
  infrastructure that exists solely for other people's self-hosted installs.
- **An abuse surface.** The relay must be reachable by arbitrary self-hosted
  companions the maintainer does not control, so registration cannot be
  meaningfully gated. Anyone who can reach it can mint a token and attempt
  pushes; the practical damage is bounded (device tokens are unguessable,
  payloads can't be forged past the NSE) but the maintainer still absorbs the
  APNs-reputation risk and the noise.
- **Apple-only.** Android/desktop/web would each need their own transport bolted
  onto the same relay.
- **App and protocol complexity:** an NSE target, a shared-keychain X25519
  keypair, a sealed-box crypto path, a `devices` table, device registration and
  last-device teardown, relay registration and token persistence.

### Option B — delegate to a notification service the user already runs

The companion does not deliver notifications itself. It POSTs each notification
to a service the user operates. Rather than carry a formatter per service, it
targets **[Apprise](https://github.com/caronc/apprise)** — one integration that
already normalises ntfy, Discord, Slack, Telegram, Gotify, Pushover, Matrix,
email, and ~100 more. The user runs
[`apprise-api`](https://github.com/caronc/apprise-api) (a small container)
configured for their service(s); the companion sends `{title, body, type}` to
it. Configuration is env (`COMPANION_APPRISE_API_URL`, optional
`COMPANION_APPRISE_URLS`, optional `COMPANION_APPRISE_TOKEN`) — fleet-wide for
v1. Per-user routing (config in `user_settings`) is a post-v1 step.

A generic JSON webhook (POST straight to a Discord/Slack/homemade URL, no
apprise-api) is a natural second `Sender` but is out of scope for v1.

What this buys:

- **No maintainer infrastructure.** Nothing to run, secure, pay for, or keep
  up. No `.p8`, no relay, no abuse surface.
- **No single point of failure** outside each user's own control.
- **Cross-platform for free** — Apprise's targets already cover iOS, Android,
  desktop, and web.
- **Stronger privacy option:** a user who self-hosts apprise-api plus a
  self-hosted ntfy/Gotify keeps task content entirely within their own
  infrastructure, with no third party (not even the maintainer) in the path.
  End-to-end payload encryption becomes unnecessary rather than load-bearing.
- **Much less code:** no relay binary, no APNs client, no sealed-box crypto, no
  NSE, no device/keypair management. The delivery seam is dedupe + one `Sender`
  interface.

What it costs:

- **Not "native".** The notification arrives from the destination app (ntfy,
  Discord, …) with that app's branding, not the Vikunja app's. A deep link can
  ride in the body but the notification chrome isn't ours.
- **Two things to set up.** The user runs `apprise-api` and configures it for
  their service, then installs that service's app (e.g. ntfy). The target
  audience (people who self-host a task manager) very often already runs one.
- **Apprise has a coarse model** — title, body, type. No universal click-action
  or per-notification icon. Fine for v1's title+body notifications.
- **Fleet-wide in v1.** The endpoint is env config, so a companion shared by
  several people sends everyone's notifications to the same target. Per-user
  routing (and the SSRF allowlist a user-controlled URL then needs) is post-v1.

## Decision

**Adopt Option B, targeting Apprise.** v1 delivery is a single `Sender` that
POSTs `{title, body, type}` to an operator-configured `apprise-api` endpoint
(env config, fleet-wide). No per-service formatters in the companion.

The maintainer-operated relay, the end-to-end sealed-box encryption, the
`devices` table and endpoints, the bring-your-own-APNs configuration, and the
`cmd/relay` binary are removed.

A generic JSON webhook `Sender` (no apprise-api) and native push are both **not
foreclosed** — each is an additional `Sender` behind the same interface, added
if demand appears, without revisiting this decision. If native push is built,
the relay's content-blind + sealed-box design from Option A is the reference.

Removing the relay also removed the reason the Vikunja API token was worth
storing per user (sealed delivery needed the companion to act as each user).
With that gone, the token follows the same env-config path:

- **No Vikunja API token is stored.** Per-request `/companion/v1/*` calls carry
  it in the header; the one background job that needs it — the digest cron —
  reads `COMPANION_VIKUNJA_TOKEN`. The `users` table (its only purpose was the
  encrypted token) is dropped.
- The **digest becomes single-user** in v1 (that env token's user). Inbound
  webhook notifications stay multi-user — they identify the user by HMAC secret.
- Per-user token routing (token in `user_settings`, encrypted at rest) returns
  post-v1, together with the `/api/v1/notifications` poller that also needs it.

## Consequences

- The companion ships as **one binary**. `internal/crypto` keeps only AEAD for
  the webhook HMAC secret — the sole remaining secret at rest.
- `internal/notify` becomes delivery-agnostic:
  `Dispatcher.Dispatch(ctx, userID, notifications)` → dedupe → `Sender.Send`.
  The `userID` is passed through now so per-user routing can land later without
  a signature change.
- The `devices`, `meta`, and `users` tables are gone. Pre-prod, so the init
  migrations were edited in place rather than adding a drop migration — the DB
  is recreated from scratch.
- The iOS app drops the Notification Service Extension, the X25519 keychain
  keypair, and APNs registration. The Apprise endpoint and the digest token are
  operator env config, not app settings.
- `docs/COMPANION.md` is updated to describe delegated delivery as the design,
  not a footnote.
- Interim state: the `Sender` is a stub that logs. The ingest, dedupe, digest,
  and settings paths are already in place; the env-backed Apprise `Sender` is
  the next unit of work.
