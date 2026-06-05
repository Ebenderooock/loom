package indexers

import (
	"sort"
	"sync"
	"time"
)

// SearchHealthStatus categorizes indexer search health.
type SearchHealthStatus string

const (
	SearchHealthy  SearchHealthStatus = "healthy"
	SearchDegraded SearchHealthStatus = "degraded"
	SearchFailing  SearchHealthStatus = "failing"
	SearchUnknown  SearchHealthStatus = "unknown"
)

// IndexerSearchHealth is the API-facing snapshot of per-indexer search metrics.
type IndexerSearchHealth struct {
	IndexerID     string             `json:"indexer_id"`
	IndexerName   string             `json:"indexer_name"`
	TotalSearches int                `json:"total_searches"`
	SuccessCount  int                `json:"success_count"`
	FailCount     int                `json:"fail_count"`
	SuccessRate   float64            `json:"success_rate"`
	AvgResponseMs int64              `json:"avg_response_ms"`
	LastSearchAt  *time.Time         `json:"last_search_at,omitempty"`
	LastErrorAt   *time.Time         `json:"last_error_at,omitempty"`
	LastError     string             `json:"last_error,omitempty"`
	APICallsToday int                `json:"api_calls_today"`
	Status        SearchHealthStatus `json:"status"`
}

// indexerMetrics stores internal per-indexer search metrics.
type indexerMetrics struct {
	totalSearches int
	successCount  int
	failCount     int
	responseTimes []time.Duration // rolling window of last 100
	lastSearchAt  time.Time
	lastErrorAt   time.Time
	lastError     string
	apiCalls      []time.Time // timestamps for rolling 24h count
}

// SearchHealthTracker tracks per-indexer search metrics in memory.
type SearchHealthTracker struct {
	mu       sync.Mutex
	metrics  map[string]*indexerMetrics
	registry *Registry
}

// NewSearchHealthTracker creates a new tracker backed by the given registry.
func NewSearchHealthTracker(registry *Registry) *SearchHealthTracker {
	return &SearchHealthTracker{
		metrics:  make(map[string]*indexerMetrics),
		registry: registry,
	}
}

const maxResponseTimes = 100

// RecordSearch records the outcome of a single indexer search. Thread-safe.
func (t *SearchHealthTracker) RecordSearch(indexerID string, duration time.Duration, resultCount int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	m := t.metrics[indexerID]
	if m == nil {
		m = &indexerMetrics{}
		t.metrics[indexerID] = m
	}

	m.totalSearches++
	now := time.Now()
	m.lastSearchAt = now

	if err != nil {
		m.failCount++
		m.lastErrorAt = now
		m.lastError = err.Error()
	} else {
		m.successCount++
	}

	// Rolling window of response times (last 100).
	m.responseTimes = append(m.responseTimes, duration)
	if len(m.responseTimes) > maxResponseTimes {
		m.responseTimes = m.responseTimes[len(m.responseTimes)-maxResponseTimes:]
	}

	// Append API call timestamp and prune entries older than 24h.
	m.apiCalls = append(m.apiCalls, now)
	cutoff := now.Add(-24 * time.Hour)
	i := 0
	for i < len(m.apiCalls) && m.apiCalls[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		m.apiCalls = m.apiCalls[i:]
	}
}

// GetHealth computes the snapshot for a single indexer.
func (t *SearchHealthTracker) GetHealth(indexerID string) IndexerSearchHealth {
	t.mu.Lock()
	defer t.mu.Unlock()

	h := IndexerSearchHealth{
		IndexerID: indexerID,
		Status:    SearchUnknown,
	}

	// Resolve name from registry.
	if t.registry != nil {
		if ix, ok := t.registry.Get(indexerID); ok {
			h.IndexerName = ix.Name()
		}
	}

	m := t.metrics[indexerID]
	if m == nil {
		return h
	}

	h.TotalSearches = m.totalSearches
	h.SuccessCount = m.successCount
	h.FailCount = m.failCount

	if m.totalSearches > 0 {
		h.SuccessRate = float64(m.successCount) / float64(m.totalSearches)
	}

	if len(m.responseTimes) > 0 {
		var total time.Duration
		for _, d := range m.responseTimes {
			total += d
		}
		h.AvgResponseMs = (total / time.Duration(len(m.responseTimes))).Milliseconds()
	}

	if !m.lastSearchAt.IsZero() {
		t := m.lastSearchAt
		h.LastSearchAt = &t
	}
	if !m.lastErrorAt.IsZero() {
		t := m.lastErrorAt
		h.LastErrorAt = &t
	}
	h.LastError = m.lastError

	// Count API calls in the last 24h.
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, ts := range m.apiCalls {
		if !ts.Before(cutoff) {
			h.APICallsToday++
		}
	}

	// Determine status.
	switch {
	case m.totalSearches == 0:
		h.Status = SearchUnknown
	case h.SuccessRate > 0.9:
		h.Status = SearchHealthy
	case h.SuccessRate > 0.7:
		h.Status = SearchDegraded
	default:
		h.Status = SearchFailing
	}

	return h
}

// GetAllHealth returns health snapshots for all tracked indexers,
// plus any registered indexers that haven't been searched yet.
// Results are sorted by indexer ID.
func (t *SearchHealthTracker) GetAllHealth() []IndexerSearchHealth {
	// Collect all IDs we know about.
	ids := make(map[string]struct{})

	t.mu.Lock()
	for id := range t.metrics {
		ids[id] = struct{}{}
	}
	t.mu.Unlock()

	// Also include registered indexers without metrics.
	if t.registry != nil {
		for _, ix := range t.registry.List() {
			ids[ix.ID()] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)

	out := make([]IndexerSearchHealth, 0, len(sorted))
	for _, id := range sorted {
		out = append(out, t.GetHealth(id))
	}
	return out
}

// Reset removes metrics for a single indexer.
func (t *SearchHealthTracker) Reset(indexerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.metrics, indexerID)
}

// ResetAll clears all tracked metrics.
func (t *SearchHealthTracker) ResetAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.metrics = make(map[string]*indexerMetrics)
}
