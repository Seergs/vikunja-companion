// Package companion assembles the HTTP surface of cmd/companion: the
// /companion/v1/* feature routes and the catch-all reverse proxy to Vikunja
// (docs/COMPANION.md §3–§5).
package companion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/seergs/vikunja-companion/internal/vikunja"
)

// ErrNoToken is returned when a request carries no bearer token.
var ErrNoToken = errors.New("companion: missing bearer token")

// userResolver looks up the Vikunja user a token belongs to. Satisfied by
// *vikunja.Client.
type userResolver interface {
	User(ctx context.Context, token string) (*vikunja.User, error)
}

// Identity is a resolved caller (docs/COMPANION.md §3).
type Identity struct {
	UserID   int64
	Username string
}

// IdentityCache maps a bearer token to its Vikunja user by forwarding it to
// GET /api/v1/user upstream and caching sha256(token) -> {user, ttl} for a few
// minutes. It never manages users — "user" is whatever Vikunja says.
type IdentityCache struct {
	resolver userResolver
	ttl      time.Duration
	now      func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	id      Identity
	expires time.Time
}

// NewIdentityCache returns a cache with the given entry TTL.
func NewIdentityCache(r userResolver, ttl time.Duration) *IdentityCache {
	return &IdentityCache{
		resolver: r,
		ttl:      ttl,
		now:      time.Now,
		entries:  make(map[string]cacheEntry),
	}
}

// Resolve returns the identity for a bearer token, consulting the cache first.
// A cache miss calls Vikunja; vikunja.IsUnauthorized(err) reports a rejected
// token.
func (c *IdentityCache) Resolve(ctx context.Context, token string) (Identity, error) {
	if token == "" {
		return Identity{}, ErrNoToken
	}
	key := hashToken(token)

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.now().Before(e.expires) {
		c.mu.Unlock()
		return e.id, nil
	}
	c.mu.Unlock()

	user, err := c.resolver.User(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	id := Identity{UserID: user.ID, Username: user.Username}

	c.mu.Lock()
	c.entries[key] = cacheEntry{id: id, expires: c.now().Add(c.ttl)}
	c.mu.Unlock()
	return id, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// bearerToken extracts the token from an Authorization: Bearer header.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
