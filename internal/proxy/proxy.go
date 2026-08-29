// Package proxy is the transparent reverse proxy to the upstream Vikunja
// instance (docs/COMPANION.md §4).
//
// Everything not under /companion/ is proxied verbatim: method, path, query,
// headers, body, status, and streaming responses. The only rewriting is
// Host / X-Forwarded-*. Authenticated responses are never cached. The proxy
// does not serve Vikunja's web SPA — its scope is /api/v1/* only.
package proxy

// TODO(fase-1): net/http/httputil.ReverseProxy wrapper over VIKUNJA_UPSTREAM_URL.
