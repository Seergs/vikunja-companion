// Package vikunja is the thin HTTP client for Vikunja's public API. All
// Vikunja-version coupling lives here — no schema access, no fork
// (docs/COMPANION.md §1, §3, §6.3).
//
// Fase 1 surface: GET /api/v1/info, GET /api/v1/user (caller identity).
// Fase 2 surface: GET/PUT/POST/DELETE /api/v1/user/settings/webhooks,
// task fetches for the digest cron.
package vikunja

// TODO(fase-1): Client{baseURL, http.Client}; Info(ctx); User(ctx, token).
