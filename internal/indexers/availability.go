package indexers

import (
	"log/slog"
	"sync"
	"time"
)

// IndexerAvailability tracks per-indexer success/failure state and
// implements a circuit-breaker pattern with exponential backoff. When
// an indexer fails repeatedly, it is temporarily skipped during
// fan-out searches to avoid wasting time and rate-limit budget.
//
// Inspired by Sonarr's IndexerStatusService.RecordFailure/RecordSuccess.
type IndexerAvailability struct {
	mu    sync.RWMutex
	state map[string]*availState
	clock Clock
}

type availState struct {
	failures      int
	lastFailure   time.Time
	disabledUntil time.Time
}

// NewIndexerAvailability creates a tracker using the given clock.
func NewIndexerAvailability(clock Clock) *IndexerAvailability {
	if clock == nil {
		clock = SystemClock{}
	}
	return &IndexerAvailability{
		state: make(map[string]*availState),
		clock: clock,
	}
}

// backoffSteps defines the escalating cooldown durations applied from
// the second consecutive failure onward (a single transient failure is
// tolerated without disabling the indexer — see backoffFor). After
// len(backoffSteps) escalations the cooldown caps at the last value.
var backoffSteps = []time.Duration{
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

func backoffFor(failures int) time.Duration {
	// Tolerate a single transient failure (timeout, blip) without
	// disabling the indexer, so a user who retries immediately isn't
	// met with a skipped indexer. Only escalate to a cooldown once an
	// indexer fails twice in a row.
	if failures <= 1 {
		return 0
	}
	idx := failures - 2
	if idx >= len(backoffSteps) {
		idx = len(backoffSteps) - 1
	}
	return backoffSteps[idx]
}

// RecordSuccess resets the failure counter for an indexer.
func (ia *IndexerAvailability) RecordSuccess(id string) {
	ia.mu.Lock()
	defer ia.mu.Unlock()
	delete(ia.state, id)
}

// RecordFailure increments the failure counter and sets a cooldown.
func (ia *IndexerAvailability) RecordFailure(id string) {
	ia.mu.Lock()
	defer ia.mu.Unlock()
	s := ia.state[id]
	if s == nil {
		s = &availState{}
		ia.state[id] = s
	}
	s.failures++
	now := ia.clock.Now()
	s.lastFailure = now
	s.disabledUntil = now.Add(backoffFor(s.failures))
	slog.Info("indexer circuit breaker: failure recorded",
		"indexer", id,
		"consecutive_failures", s.failures,
		"disabled_until", s.disabledUntil.Format(time.RFC3339),
	)
}

// IsAvailable returns true if the indexer is not in a cooldown period.
func (ia *IndexerAvailability) IsAvailable(id string) bool {
	ia.mu.RLock()
	defer ia.mu.RUnlock()
	s := ia.state[id]
	if s == nil {
		return true
	}
	return ia.clock.Now().After(s.disabledUntil)
}

// FilterAvailable returns only the IDs that are currently available,
// plus a list of skipped IDs (for diagnostics).
func (ia *IndexerAvailability) FilterAvailable(ids []string) (available, skipped []string) {
	ia.mu.RLock()
	defer ia.mu.RUnlock()
	now := ia.clock.Now()
	for _, id := range ids {
		s := ia.state[id]
		if s == nil || now.After(s.disabledUntil) {
			available = append(available, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	return available, skipped
}

// FailureCount returns the current consecutive failure count for an
// indexer. Returns 0 if the indexer has no recorded failures.
func (ia *IndexerAvailability) FailureCount(id string) int {
	ia.mu.RLock()
	defer ia.mu.RUnlock()
	if s := ia.state[id]; s != nil {
		return s.failures
	}
	return 0
}
