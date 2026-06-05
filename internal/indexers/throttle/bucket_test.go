package throttle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a deterministic clock used in bucket tests. Callers
// advance time via Advance(); the bucket's time.Now closure reads
// from the same atomic.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestBucket_AcquireWhenTokenAvailable(t *testing.T) {
	clk := newFakeClock()
	b := newBucketWithClock(60, 5, clk.Now)
	wait, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if wait != 0 {
		t.Fatalf("expected zero wait, got %v", wait)
	}
}

func TestBucket_BurstThenBlocks(t *testing.T) {
	clk := newFakeClock()
	b := newBucketWithClock(60, 3, clk.Now)
	for i := 0; i < 3; i++ {
		if _, err := b.Acquire(context.Background()); err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
	}
	// reserve() should now return a non-zero wait. We test it
	// directly to avoid hanging Acquire.
	w := b.reserve()
	if w <= 0 {
		t.Fatalf("expected positive wait when bucket empty, got %v", w)
	}
	// At 60/min = 1/sec, refill ought to be ~1s.
	if w < 500*time.Millisecond || w > 2*time.Second {
		t.Fatalf("wait %v outside plausible range", w)
	}
}

func TestBucket_RefillsOverTime(t *testing.T) {
	clk := newFakeClock()
	b := newBucketWithClock(60, 1, clk.Now)
	if _, err := b.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Advance > 1s to allow a single refill.
	clk.Advance(2 * time.Second)
	w := b.reserve()
	if w != 0 {
		t.Fatalf("expected zero wait after refill, got %v", w)
	}
}

func TestBucket_AcquireRespectsContext(t *testing.T) {
	clk := newFakeClock()
	b := newBucketWithClock(1, 1, clk.Now) // 1/min = very slow
	if _, err := b.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var (
		wg     sync.WaitGroup
		gotErr atomic.Value
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := b.Acquire(ctx)
		gotErr.Store(err)
	}()
	// Give the goroutine a moment to enter the timer.
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()
	if v := gotErr.Load(); v == nil || !errors.Is(v.(error), context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", v)
	}
}
