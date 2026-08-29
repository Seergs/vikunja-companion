// Package config loads and validates the environment-first configuration for
// both binaries.
package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// lookup returns the trimmed value of an env var and whether it was set.
func lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	return strings.TrimSpace(v), ok
}

// get returns the env var value or def if unset/empty.
func get(key, def string) string {
	if v, ok := lookup(key); ok && v != "" {
		return v
	}
	return def
}

// required returns the env var value or an error if unset/empty.
func required(key string) (string, error) {
	if v, ok := lookup(key); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s is required", key)
}

// parseHTTPURL validates that raw is an absolute http(s) URL.
func parseHTTPURL(key, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%s: must be an http or https URL", key)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%s: missing host", key)
	}
	return strings.TrimRight(raw, "/"), nil
}

// decodeKey decodes a 32-byte key given as base64 (std or raw) or hex.
func decodeKey(key, raw string) ([]byte, error) {
	for _, dec := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		hex.DecodeString,
	} {
		if b, err := dec(raw); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	return nil, fmt.Errorf("%s: must decode (base64 or hex) to exactly 32 bytes", key)
}
