package throttle

import (
	"context"
	"sync"
	"time"
)

// Bucket is a simple token-bucket rate limiter. Tokens refill at a
// constant rate (perMinute / 60 tokens per second) up to a maximum
// of `burst` tokens. Acquire blocks until one token is available, or
// returns ctx.Err() if the context is cancelled while waiting.
//
// We hand-roll this rather than reach for golang.org/x/time/rate
// because (a) we want sub-millisecond determinism in tests, (b) the
// reported "wait" duration is part of our metrics surface, and (c) a
// dependency-free implementation keeps the import graph small.
type Bucket struct {
	now func() time.Time

	mu          sync.Mutex
	capacity    float64
	rate        float64 // tokens per second
	tokens      float64
	lastRefill  time.Time
}

// NewBucket returns a Bucket primed with `burst` tokens. Both
// arguments must be positive; callers should pass them through
// Resolve() first to apply defaults.
func NewBucket(perMinute, burst int) *Bucket {
	cfg := Resolve(Config{PerMinute: perMinute, Burst: burst, MaxRetries: -1})
	return newBucketWithClock(cfg.PerMinute, cfg.Burst, time.Now)
}

func newBucketWithClock(perMinute, burst int, now func() time.Time) *Bucket {
	return &Bucket{
		now:        now,
		capacity:   float64(burst),
		rate:       float64(perMinute) / 60.0,
		tokens:     float64(burst),
		lastRefill: now(),
	}
}

// Acquire blocks until a token is available, returning the duration
// it spent waiting and any context error. A nil error means a token
// was successfully consumed; ctx.Err() is returned verbatim on
// cancellation so callers can use errors.Is(err, context.Canceled).
func (b *Bucket) Acquire(ctx context.Context) (time.Duration, error) {
	start := b.now()
	for {
		wait := b.reserve()
		if wait <= 0 {
			return b.now().Sub(start), nil
		}
		// Either sleep for `wait` or honour ctx cancellation.
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return b.now().Sub(start), ctx.Err()
		case <-t.C:
		}
	}
}

// reserve consumes a token, returning 0 if one was available
// immediately or the duration until the next token becomes available.
// The token is "reserved" optimistically — if the caller is later
// cancelled, the bucket is slightly under-utilised, which is the
// preferred direction (we'd rather under-rate than burst past the
// configured cap).
func (b *Bucket) reserve() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.lastRefill = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	missing := 1 - b.tokens
	wait := time.Duration(missing/b.rate*float64(time.Second)) + time.Microsecond
	// Reserve the token now so concurrent callers queue behind us.
	b.tokens--
	return wait
}
