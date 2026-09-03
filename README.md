# vikunja-companion

An optional, self-hosted sidecar for [Vikunja](https://vikunja.io) that adds
notifications Vikunja can't send on its own, so you hear about your tasks while
the app is closed.

One Go binary, one SQLite file, one container. If you don't run it, you lose
nothing.

## What it does

- **Morning digest.** A cron sends one "N tasks for today · M urgent" briefing at
  your local send time (default 08:00).
- **Webhook notifications.** You add a webhook in Vikunja's UI; the companion
  verifies inbound deliveries (HMAC) and turns `task.reminder.fired`,
  `task.overdue` and `tasks.overdue` into notifications.
- **Delivery via Apprise.** The companion has no push infrastructure. It POSTs
  each notification to your [`apprise-api`](https://github.com/caronc/apprise-api),
  which fans out to ntfy / Discord / Gotify / Telegram / email / ~100 more.

## Run it

```sh
cp .env.example .env      # fill in the required vars
make run-companion        # local dev, auto-loads ./.env
```

Or with Docker, see `docker-compose.example.yml`.

Required config:

| Var | Meaning |
| --- | --- |
| `COMPANION_PUBLIC_URL` | Public HTTPS URL of the companion |
| `VIKUNJA_UPSTREAM_URL` | The Vikunja instance to talk to |
| `COMPANION_MASTER_KEY` | 32-byte key (`openssl rand -hex 32`) encrypting the webhook secret at rest |

For notifications to actually be sent, set `COMPANION_APPRISE_API_URL` (unset =
they're only logged). For the digest, set `COMPANION_VIKUNJA_TOKEN` (unset =
digest off). Full list in `.env.example`.

## Develop

```sh
make test
make build     # CGO_ENABLED=0 -> bin/companion
make vet
```
