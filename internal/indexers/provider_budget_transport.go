package indexers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type budgetContextKey struct{}

type budgetAttempt struct {
	mu     sync.Mutex
	store  *ProviderBudgetStore
	id     string
	first  bool
	ticket *ProviderBudget
	reset  int64
}

func (s *ProviderBudgetStore) admit(ctx context.Context, id string) (context.Context, error) {
	ticket, err := s.reserve(ctx, id)
	if err != nil {
		return ctx, err
	}
	a := &budgetAttempt{
		store: s, id: id, first: true, ticket: ticket,
	}
	if ticket != nil {
		a.reset = ticket.ProviderResetAt
	}
	return context.WithValue(ctx, budgetContextKey{}, a), nil
}

// budgetTransport sits inside throttle so retries and multi-path requests each
// consume an allowance. The query reservation pays for its first HTTP attempt.
type budgetTransport struct{ base http.RoundTripper }

func (t budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	a, _ := req.Context().Value(budgetContextKey{}).(*budgetAttempt)
	if a == nil {
		return t.base.RoundTrip(req)
	}
	a.mu.Lock()
	ticket := a.ticket
	if !a.first {
		var err error
		ticket, err = a.store.reserve(req.Context(), a.id)
		if err != nil {
			a.mu.Unlock()
			return nil, err
		}
	}
	a.first = false
	a.ticket = ticket
	if ticket != nil {
		a.reset = ticket.ProviderResetAt
	}
	a.mu.Unlock()
	resp, err := t.base.RoundTrip(req)
	if resp == nil || ticket == nil {
		return resp, err
	}
	remaining, reset, ok := providerSignal(resp, a.store.clock.Now())
	if ok {
		// A received response still governs admission after caller cancellation.
		observeCtx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
		defer cancel()
		if observeErr := a.store.observe(observeCtx, *ticket, remaining, reset); observeErr != nil {
			_ = resp.Body.Close()
			return nil, observeErr
		}
		a.mu.Lock()
		if a.ticket == ticket {
			a.reset = reset
		}
		a.mu.Unlock()
	}
	return resp, err
}

func providerSignal(resp *http.Response, now time.Time) (remaining, reset int64, ok bool) {
	// X-RateLimit-Reset is Unix seconds; RateLimit-Reset is delta seconds.
	for _, prefix := range []string{"X-RateLimit-", "RateLimit-"} {
		value := resp.Header.Get(prefix + "Remaining")
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 0 || n > 1_000_000_000 {
			continue
		}
		r, err := strconv.ParseInt(resp.Header.Get(prefix+"Reset"), 10, 64)
		candidateReset := int64(0)
		if err == nil && r > 0 {
			if prefix == "RateLimit-" {
				if r <= 366*24*60*60 {
					r += now.Unix()
				} else {
					r = 0
				}
			}
			if r > now.Unix() && r <= now.AddDate(1, 0, 1).Unix() {
				candidateReset = r
			}
		}
		if !ok {
			remaining, reset, ok = n, candidateReset, true
		} else {
			remaining = min(remaining, n)
			if reset == 0 || candidateReset == 0 {
				reset = 0
			} else {
				reset = max(reset, candidateReset)
			}
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		remaining, ok = 0, true
		if v := resp.Header.Get("Retry-After"); v != "" {
			var retryAt int64
			if seconds, err := strconv.ParseInt(v, 10, 64); err == nil && seconds > 0 && seconds <= 366*24*60*60 {
				retryAt = now.Unix() + seconds
			} else if date, err := http.ParseTime(v); err == nil && date.After(now) && date.Before(now.AddDate(1, 0, 1)) {
				retryAt = date.Unix()
			}
			if retryAt > reset {
				reset = retryAt
			}
		}
	}
	return remaining, reset, ok
}

// RecordProviderExhaustion persists a protocol-level exhaustion response when
// HTTP status alone did not carry it (e.g. Newznab's XML error envelope).
func RecordProviderExhaustion(ctx context.Context) error {
	a, _ := ctx.Value(budgetContextKey{}).(*budgetAttempt)
	if a == nil {
		return nil
	}
	a.mu.Lock()
	ticket := a.ticket
	reset := a.reset
	a.mu.Unlock()
	if ticket == nil {
		return nil
	}
	observeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := a.store.observe(observeCtx, *ticket, 0, reset); err != nil {
		return fmt.Errorf("record provider exhaustion: %w", err)
	}
	return nil
}
