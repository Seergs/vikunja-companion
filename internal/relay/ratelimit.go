package relay

import (
	"sync"
	"time"
)

// rateLimiter is a per-key token bucket. It protects the shared APNs key from
// abuse — it is not meant to meter a personal task manager's real volume.
type rateLimiter struct {
	ratePerSec float64
	burst      float64
	now        func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(ratePerSec, burst float64) *rateLimiter {
	return &rateLimiter{
		ratePerSec: ratePerSec,
		burst:      burst,
		now:        time.Now,
		buckets:    make(map[string]*bucket),
	}
}

// allow reports whether an event for key may proceed, consuming a token if so.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}

	b.tokens += now.Sub(b.last).Seconds() * l.ratePerSec
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
