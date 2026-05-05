package scheduler

import (
	"sort"
	"time"
)

// QuotaStatus represents the current quota state of an indexer.
type QuotaStatus struct {
	IndexerID   string  `json:"indexer_id"`
	Used        int     `json:"used"`
	Max         int     `json:"max"`
	Remaining   int     `json:"remaining"`
	ResetAt     string  `json:"reset_at"`
	PercentUsed float64 `json:"percent_used"`
}

// Remaining returns how many searches are left for this indexer.
func (q *QuotaTracker) Remaining(indexerID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pruneOld(indexerID)
	used := len(q.records[indexerID])
	rem := q.maxPerDay - used
	if rem < 0 {
		rem = 0
	}
	return rem
}

// QuotaStatuses returns quota info for all tracked indexers.
func (q *QuotaTracker) QuotaStatuses() []QuotaStatus {
	q.mu.Lock()
	defer q.mu.Unlock()

	var statuses []QuotaStatus
	for id := range q.records {
		q.pruneOld(id)
		used := len(q.records[id])
		rem := q.maxPerDay - used
		if rem < 0 {
			rem = 0
		}
		pct := 0.0
		if q.maxPerDay > 0 {
			pct = float64(used) / float64(q.maxPerDay) * 100
		}

		// Estimate reset time: earliest record + 24h
		resetAt := time.Now().Add(24 * time.Hour)
		if len(q.records[id]) > 0 {
			resetAt = q.records[id][0].Add(24 * time.Hour)
		}

		statuses = append(statuses, QuotaStatus{
			IndexerID:   id,
			Used:        used,
			Max:         q.maxPerDay,
			Remaining:   rem,
			ResetAt:     resetAt.UTC().Format(time.RFC3339),
			PercentUsed: pct,
		})
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].IndexerID < statuses[j].IndexerID
	})
	return statuses
}

// PrioritizeIndexers returns indexer IDs sorted by remaining quota
// (most remaining first). This spreads searches across indexers.
func (q *QuotaTracker) PrioritizeIndexers(ids []string) []string {
	type entry struct {
		id        string
		remaining int
	}
	entries := make([]entry, 0, len(ids))
	for _, id := range ids {
		rem := q.Remaining(id)
		if rem > 0 {
			entries = append(entries, entry{id: id, remaining: rem})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].remaining > entries[j].remaining
	})
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.id
	}
	return result
}

// ResetDaily clears all records older than 24 hours. Normally handled
// by pruneOld lazily, but this can be called explicitly at midnight.
func (q *QuotaTracker) ResetDaily() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for id := range q.records {
		q.pruneOld(id)
	}
}
