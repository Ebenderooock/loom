package scheduler

import (
	"sync"
	"time"
)

// QuotaTracker tracks per-indexer search counts over a rolling 24-hour
// window. The data is in-memory and resets on restart, which is
// acceptable for a background scheduler that runs infrequently.
type QuotaTracker struct {
	mu        sync.Mutex
	maxPerDay int
	records   map[string][]time.Time // indexerID → timestamps
}

// NewQuotaTracker returns a tracker with the given daily cap per indexer.
func NewQuotaTracker(maxPerDay int) *QuotaTracker {
	if maxPerDay <= 0 {
		maxPerDay = 100
	}
	return &QuotaTracker{
		maxPerDay: maxPerDay,
		records:   make(map[string][]time.Time),
	}
}

// SetMax updates the daily cap. Safe for concurrent use.
func (q *QuotaTracker) SetMax(max int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if max > 0 {
		q.maxPerDay = max
	}
}

// CanSearch reports whether the indexer is under its daily quota.
func (q *QuotaTracker) CanSearch(indexerID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pruneOld(indexerID)
	return len(q.records[indexerID]) < q.maxPerDay
}

// RecordSearch increments the counter for an indexer.
func (q *QuotaTracker) RecordSearch(indexerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.records[indexerID] = append(q.records[indexerID], time.Now())
}

// Usage returns current counts per indexer (pruned to last 24 h).
func (q *QuotaTracker) Usage() map[string]int {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make(map[string]int, len(q.records))
	for id := range q.records {
		q.pruneOld(id)
		out[id] = len(q.records[id])
	}
	return out
}

func (q *QuotaTracker) pruneOld(indexerID string) {
	cutoff := time.Now().Add(-24 * time.Hour)
	ts := q.records[indexerID]
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		q.records[indexerID] = ts[i:]
	}
}
