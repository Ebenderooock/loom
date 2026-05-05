// Package scheduler implements background rolling search for missing
// media. It periodically queries the library for movies and episodes
// that have not been acquired yet and fans searches out to the
// configured indexers in small, quota-aware batches.
package scheduler

import "time"

// RollingSearchConfig controls the rolling search scheduler.
type RollingSearchConfig struct {
	Enabled          bool `json:"enabled"`
	IntervalHours    int  `json:"intervalHours"`
	BatchSize        int  `json:"batchSize"`
	MinResearchDays  int  `json:"minResearchDays"`
	MaxSearchesPerDay int `json:"maxSearchesPerDay"`
}

// DefaultRollingSearchConfig returns sensible defaults.
func DefaultRollingSearchConfig() RollingSearchConfig {
	return RollingSearchConfig{
		Enabled:          false,
		IntervalHours:    12,
		BatchSize:        5,
		MinResearchDays:  7,
		MaxSearchesPerDay: 100,
	}
}

// SearchCandidate is a media item eligible for a rolling search.
type SearchCandidate struct {
	MediaType      string     `json:"mediaType"`      // "movie" or "episode"
	MediaID        string     `json:"mediaId"`
	Title          string     `json:"title"`
	Year           int        `json:"year,omitempty"`
	LastSearchedAt *time.Time `json:"lastSearchedAt,omitempty"`
	Priority       int        `json:"priority"`
}

// RollingSearchStatus is the API-facing snapshot of the scheduler state.
type RollingSearchStatus struct {
	Running        bool       `json:"running"`
	LastRunAt      *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt      *time.Time `json:"nextRunAt,omitempty"`
	ItemsSearched  int        `json:"itemsSearched"`
	ItemsInQueue   int        `json:"itemsInQueue"`
	QuotaUsage     map[string]int `json:"quotaUsage"`
}
