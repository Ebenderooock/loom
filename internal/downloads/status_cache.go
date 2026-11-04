package downloads

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StatusPolicy controls how the registry shares Status() calls against
// external download clients. Every consumer (UI polling, the monitor,
// workflow reconciliation, imports) reads the same per-client snapshot,
// so a remote client sees at most one status request per MinInterval
// regardless of how many browser tabs or background jobs are active.
// Failures back off exponentially so an overloaded or rate-limiting
// client is given room to recover instead of being hammered.
//
// In-process clients (the built-in torrent engine) have no request
// budget and bypass this policy entirely.
type StatusPolicy struct {
	// MinInterval is how long a successful snapshot is reused.
	MinInterval time.Duration
	// BaseBackoff is the cooldown after the first consecutive failure.
	BaseBackoff time.Duration
	// MaxBackoff caps the exponential failure cooldown.
	MaxBackoff time.Duration
	// FetchTimeout bounds a shared fetch when the caller's context has
	// no deadline.
	FetchTimeout time.Duration
}

// DefaultStatusPolicy is used by NewRegistry.
var DefaultStatusPolicy = StatusPolicy{
	MinInterval:  5 * time.Second,
	BaseBackoff:  10 * time.Second,
	MaxBackoff:   5 * time.Minute,
	FetchTimeout: 15 * time.Second,
}

// usesSharedStatus reports whether a kind talks to an external service
// and therefore needs request coalescing and failure backoff.
func usesSharedStatus(k Kind) bool {
	switch k {
	case KindBuiltinTorrent, KindNull:
		return false
	}
	return true
}

// ClientStatusError reports that an external client's last status
// request failed and the registry is waiting before trying again.
type ClientStatusError struct {
	Err        error
	RetryAt    time.Time
	LastOK     time.Time
	Failures   int
	coolingOff bool
}

func (e *ClientStatusError) Error() string {
	if !e.coolingOff {
		return e.Err.Error()
	}
	wait := time.Until(e.RetryAt).Round(time.Second)
	if wait < time.Second {
		wait = time.Second
	}
	return fmt.Sprintf("%s (retrying in %s)", e.Err.Error(), wait)
}

func (e *ClientStatusError) Unwrap() error { return e.Err }

type statusCall struct {
	done  chan struct{}
	items []Item
	err   error
}

type statusEntry struct {
	mu        sync.Mutex
	client    DownloadClient
	items     []Item
	fetchedAt time.Time
	lastOK    time.Time
	lastErr   error
	failures  int
	retryAt   time.Time
	inflight  *statusCall
}

type statusCache struct {
	mu      sync.Mutex
	policy  StatusPolicy
	now     func() time.Time
	entries map[string]*statusEntry
}

func newStatusCache(p StatusPolicy) *statusCache {
	return &statusCache{policy: p, now: time.Now, entries: map[string]*statusEntry{}}
}

// entry returns the cache slot for c, discarding any slot that belonged
// to a previous client instance with the same ID (e.g. after an edit).
func (sc *statusCache) entry(c DownloadClient) *statusEntry {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	e, ok := sc.entries[c.ID()]
	if !ok || e.client != c {
		e = &statusEntry{client: c}
		sc.entries[c.ID()] = e
	}
	return e
}

func (sc *statusCache) forget(id string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.entries, id)
}

// invalidate drops the cached snapshot so the next read is fresh. A
// pending failure cooldown is kept unless clearBackoff is set.
func (sc *statusCache) invalidate(id string, clearBackoff bool) {
	sc.mu.Lock()
	e := sc.entries[id]
	sc.mu.Unlock()
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fetchedAt = time.Time{}
	if clearBackoff {
		e.failures = 0
		e.retryAt = time.Time{}
		e.lastErr = nil
	}
}

// status returns the client's items, sharing a recent snapshot or an
// in-flight request with other callers. ids filters the snapshot.
func (sc *statusCache) status(ctx context.Context, c DownloadClient, ids []string) ([]Item, error) {
	e := sc.entry(c)
	e.mu.Lock()
	now := sc.now()
	if e.lastErr == nil && !e.fetchedAt.IsZero() && now.Sub(e.fetchedAt) < sc.policy.MinInterval {
		items := filterItems(e.items, ids)
		e.mu.Unlock()
		return items, nil
	}
	if e.lastErr != nil && now.Before(e.retryAt) {
		err := &ClientStatusError{Err: e.lastErr, RetryAt: e.retryAt, LastOK: e.lastOK, Failures: e.failures, coolingOff: true}
		e.mu.Unlock()
		return nil, err
	}
	call := e.inflight
	if call == nil {
		call = &statusCall{done: make(chan struct{})}
		e.inflight = call
		go sc.fetch(ctx, e, call)
	}
	e.mu.Unlock()

	select {
	case <-call.done:
		if call.err != nil {
			return nil, call.err
		}
		return filterItems(call.items, ids), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fetch performs the single shared request. It is detached from the
// initiating caller's cancellation so one impatient caller cannot fail
// the request for everyone waiting on it, but it inherits a deadline.
func (sc *statusCache) fetch(parent context.Context, e *statusEntry, call *statusCall) {
	timeout := sc.policy.FetchTimeout
	if dl, ok := parent.Deadline(); ok {
		if d := time.Until(dl); d > 0 && (timeout <= 0 || d < timeout) {
			timeout = d
		}
	}
	ctx := context.WithoutCancel(parent)
	cancel := func() {}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	items, err := e.client.Status(ctx)
	cancel()

	e.mu.Lock()
	defer e.mu.Unlock()
	now := sc.now()
	if err != nil {
		e.failures++
		e.lastErr = err
		e.retryAt = now.Add(sc.backoff(e.failures))
		call.err = &ClientStatusError{Err: err, RetryAt: e.retryAt, LastOK: e.lastOK, Failures: e.failures}
	} else {
		e.items = items
		e.fetchedAt = now
		e.lastOK = now
		e.lastErr = nil
		e.failures = 0
		e.retryAt = time.Time{}
		call.items = items
	}
	e.inflight = nil
	close(call.done)
}

func (sc *statusCache) backoff(failures int) time.Duration {
	d := sc.policy.BaseBackoff
	for i := 1; i < failures && d < sc.policy.MaxBackoff; i++ {
		d *= 2
	}
	if sc.policy.MaxBackoff > 0 && d > sc.policy.MaxBackoff {
		d = sc.policy.MaxBackoff
	}
	return d
}

// filterItems returns a copy of items (restricted to ids when given)
// so callers may mutate the result without corrupting the snapshot.
func filterItems(items []Item, ids []string) []Item {
	if len(ids) == 0 {
		out := make([]Item, len(items))
		copy(out, items)
		return out
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[strings.ToLower(id)] = struct{}{}
	}
	out := make([]Item, 0, len(ids))
	for _, it := range items {
		if _, ok := want[strings.ToLower(it.ID)]; ok {
			out = append(out, it)
		}
	}
	return out
}
