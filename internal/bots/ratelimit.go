package bots

import (
	"sync"
	"time"
)

// rateLimiter is a simple per-key token bucket used to throttle chat commands
// from a single chat identity, protecting the metadata provider and request
// pipeline from spam.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64 // tokens per second
	now      func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// newRateLimiter allows up to capacity actions, refilling perMinute tokens each
// minute, per key.
func newRateLimiter(capacity, perMinute float64) *rateLimiter {
	return &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: capacity,
		refill:   perMinute / 60.0,
		now:      time.Now,
	}
}

// allow reports whether an action for key may proceed, consuming a token.
func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	b, ok := r.buckets[key]
	if !ok {
		r.buckets[key] = &bucket{tokens: r.capacity - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * r.refill
	if b.tokens > r.capacity {
		b.tokens = r.capacity
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
