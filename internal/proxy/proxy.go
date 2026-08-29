// Package proxy is the transparent reverse proxy to the upstream Vikunja
// instance (docs/COMPANION.md §4).
//
// Everything not under /companion/ is proxied verbatim: method, path, query,
// headers, body, status, and streaming responses. The only rewriting is
// Host / X-Forwarded-*. Authenticated responses are never cached. The proxy
// does not serve Vikunja's web SPA — its scope is /api/v1/* only.
package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// New returns a reverse proxy that forwards every request verbatim to
// upstreamURL, which must be an absolute http(s) URL.
func New(upstreamURL string, log *slog.Logger) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("proxy: parsing upstream URL: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("proxy: upstream URL must be http or https, got %q", target.Scheme)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("proxy: upstream URL is missing a host")
	}

	rp := &httputil.ReverseProxy{
		// Flush every write immediately so server-sent events and other
		// streaming responses pass through without buffering.
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)         // scheme, host, path join, query — verbatim
			pr.SetXForwarded()        // X-Forwarded-For/Host/Proto
			pr.Out.Host = target.Host // send the upstream's Host header
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Warn("proxy upstream error", "method", r.Method, "path", r.URL.Path, "err", err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	return rp, nil
}
