package downloads

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingClient records Status calls and lets tests block or fail them.
type countingClient struct {
	nullClient
	kind  Kind
	calls atomic.Int64
	gate  chan struct{}

	mu    sync.Mutex
	err   error
	items []Item
}

func newCounting(id string, kind Kind) *countingClient {
	return &countingClient{
		nullClient: nullClient{id: id, name: id, protocol: ProtocolTorrent},
		kind:       kind,
		items:      []Item{{ID: "ABC", Title: "a"}, {ID: "def", Title: "d"}},
	}
}

func (c *countingClient) Kind() Kind { return c.kind }

func (c *countingClient) Status(ctx context.Context, _ ...string) ([]Item, error) {
	c.calls.Add(1)
	if c.gate != nil {
		select {
		case <-c.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	return append([]Item(nil), c.items...), nil
}

func (c *countingClient) setErr(err error) {
	c.mu.Lock()
	c.err = err
	c.mu.Unlock()
}

type statusClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *statusClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *statusClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newTestRegistry(t *testing.T, clients ...DownloadClient) (*Registry, *statusClock) {
	t.Helper()
	r := NewRegistry()
	r.SetStatusPolicy(StatusPolicy{
		MinInterval:  5 * time.Second,
		BaseBackoff:  10 * time.Second,
		MaxBackoff:   40 * time.Second,
		FetchTimeout: time.Second,
	})
	clk := &statusClock{now: time.Unix(1_700_000_000, 0)}
	r.status.now = clk.Now
	for _, c := range clients {
		if err := r.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	return r, clk
}

func TestStatusSharesSnapshotAcrossCallers(t *testing.T) {
	c := newCounting("deluge", KindDeluge)
	r, clk := newTestRegistry(t, c)
	ctx := context.Background()

	for range 5 {
		got := r.Status(ctx, nil, FanOutOptions{})
		if len(got.Items) != 2 || len(got.Errors) != 0 {
			t.Fatalf("status = %+v", got)
		}
	}
	if _, err := r.ClientStatus(ctx, c, "abc"); err != nil {
		t.Fatal(err)
	}
	if n := c.calls.Load(); n != 1 {
		t.Fatalf("client calls = %d, want 1 within MinInterval", n)
	}
	clk.Advance(5 * time.Second)
	r.Status(ctx, nil, FanOutOptions{})
	if n := c.calls.Load(); n != 2 {
		t.Fatalf("client calls = %d, want 2 after MinInterval", n)
	}
}

func TestStatusCoalescesConcurrentRequests(t *testing.T) {
	c := newCounting("qb", KindQBittorrent)
	c.gate = make(chan struct{})
	r, _ := newTestRegistry(t, c)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := r.Status(context.Background(), nil, FanOutOptions{}); len(got.Errors) != 0 {
				t.Errorf("errors = %v", got.Errors)
			}
		}()
	}
	// Let all callers join the in-flight request before releasing it.
	deadline := time.Now().Add(time.Second)
	for c.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(c.gate)
	wg.Wait()
	if n := c.calls.Load(); n != 1 {
		t.Fatalf("client calls = %d, want 1 shared request", n)
	}
}

func TestStatusBacksOffAfterFailures(t *testing.T) {
	c := newCounting("deluge", KindDeluge)
	c.setErr(errors.New("connection refused"))
	r, clk := newTestRegistry(t, c)
	ctx := context.Background()

	got := r.Status(ctx, nil, FanOutOptions{})
	if !strings.Contains(got.Errors["deluge"], "connection refused") {
		t.Fatalf("errors = %v", got.Errors)
	}
	// Cooling off: errors are reported without contacting the client,
	// and items are never presented as an empty successful queue.
	for range 3 {
		got = r.Status(ctx, nil, FanOutOptions{})
		if !strings.Contains(got.Errors["deluge"], "retrying in") || len(got.Items) != 0 {
			t.Fatalf("cooldown status = %+v", got)
		}
	}
	if n := c.calls.Load(); n != 1 {
		t.Fatalf("calls during cooldown = %d, want 1", n)
	}
	var cse *ClientStatusError
	if _, err := r.ClientStatus(ctx, c); !errors.As(err, &cse) || cse.Failures != 1 {
		t.Fatalf("ClientStatus err = %v", err)
	}

	// Backoff doubles: 10s, then 20s, capped at 40s.
	for i, wait := range []time.Duration{10, 20, 40, 40} {
		clk.Advance(wait*time.Second - time.Millisecond)
		r.Status(ctx, nil, FanOutOptions{})
		if n := c.calls.Load(); n != int64(i+1) {
			t.Fatalf("step %d: calls = %d, want %d before cooldown ends", i, n, i+1)
		}
		clk.Advance(time.Millisecond)
		r.Status(ctx, nil, FanOutOptions{})
		if n := c.calls.Load(); n != int64(i+2) {
			t.Fatalf("step %d: calls = %d, want %d after cooldown", i, n, i+2)
		}
	}

	c.setErr(nil)
	clk.Advance(40 * time.Second)
	if got = r.Status(ctx, nil, FanOutOptions{}); len(got.Errors) != 0 || len(got.Items) != 2 {
		t.Fatalf("recovered status = %+v", got)
	}
}

func TestBuiltinClientsBypassSharedStatus(t *testing.T) {
	c := newCounting("builtin", KindBuiltinTorrent)
	c.setErr(errors.New("engine busy"))
	r, _ := newTestRegistry(t, c)
	for range 3 {
		r.Status(context.Background(), nil, FanOutOptions{})
	}
	c.setErr(nil)
	for range 3 {
		if got := r.Status(context.Background(), nil, FanOutOptions{}); len(got.Errors) != 0 {
			t.Fatalf("errors = %v", got.Errors)
		}
	}
	if n := c.calls.Load(); n != 6 {
		t.Fatalf("builtin calls = %d, want every call live", n)
	}
}

func TestInvalidateStatusRefreshesAndClearsBackoff(t *testing.T) {
	c := newCounting("tr", KindTransmission)
	r, _ := newTestRegistry(t, c)
	ctx := context.Background()

	r.Status(ctx, nil, FanOutOptions{})
	r.InvalidateStatus("tr")
	r.Status(ctx, nil, FanOutOptions{})
	if n := c.calls.Load(); n != 2 {
		t.Fatalf("calls = %d, want refresh after invalidate", n)
	}

	c.setErr(errors.New("timeout"))
	r.InvalidateStatus("tr")
	r.Status(ctx, nil, FanOutOptions{})
	c.setErr(nil)
	r.InvalidateStatus("tr")
	if got := r.Status(ctx, nil, FanOutOptions{}); len(got.Errors) != 0 {
		t.Fatalf("errors after invalidate = %v", got.Errors)
	}
	if n := c.calls.Load(); n != 4 {
		t.Fatalf("calls = %d, want 4", n)
	}
}

func TestReplaceDropsCachedStatus(t *testing.T) {
	c := newCounting("tr", KindTransmission)
	c.setErr(errors.New("bad password"))
	r, _ := newTestRegistry(t, c)
	r.Status(context.Background(), nil, FanOutOptions{})

	fixed := newCounting("tr", KindTransmission)
	if err := r.Replace(fixed); err != nil {
		t.Fatal(err)
	}
	if got := r.Status(context.Background(), nil, FanOutOptions{}); len(got.Errors) != 0 {
		t.Fatalf("errors after replace = %v", got.Errors)
	}
	if fixed.calls.Load() != 1 {
		t.Fatalf("replacement client was not queried")
	}
}

func TestClientStatusFiltersSnapshotAndCopies(t *testing.T) {
	c := newCounting("dl", KindDeluge)
	r, _ := newTestRegistry(t, c)
	items, err := r.ClientStatus(context.Background(), c, "abc")
	if err != nil || len(items) != 1 || items[0].ID != "ABC" {
		t.Fatalf("filtered = %+v, %v", items, err)
	}
	items[0].Title = "mutated"
	all, _ := r.ClientStatus(context.Background(), c)
	for _, it := range all {
		if it.Title == "mutated" {
			t.Fatal("caller mutation leaked into shared snapshot")
		}
	}
}

func TestStatusWaiterCancellationDoesNotFailSharedFetch(t *testing.T) {
	c := newCounting("qb", KindQBittorrent)
	c.gate = make(chan struct{})
	r, _ := newTestRegistry(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.ClientStatus(ctx, c)
		done <- err
	}()
	for c.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller err = %v", err)
	}
	close(c.gate)
	items, err := r.ClientStatus(context.Background(), c)
	if err != nil || len(items) != 2 {
		t.Fatalf("shared fetch result = %+v, %v", items, err)
	}
	if n := c.calls.Load(); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
}
