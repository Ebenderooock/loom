// Package searchdebug provides comprehensive search debug logging.
// Each autosearch attempt is recorded with full request context,
// query chain, per-indexer results, evaluation details, and outcome.
package searchdebug

import (
	"time"
)

// Search status lifecycle constants.
const (
	StatusSearching  = "searching"
	StatusEvaluating = "evaluating"
	StatusGrabbing   = "grabbing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

// Entry is a single search debug log entry.
type Entry struct {
	ID               string          `json:"id"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Status           string          `json:"status"`
	SearchRunID      string          `json:"search_run_id,omitempty"`
	MediaType        string          `json:"media_type"`
	MediaID          string          `json:"media_id"`
	Title            string          `json:"title"`
	Year             int             `json:"year"`
	Season           int             `json:"season"`
	Episode          int             `json:"episode"`
	IMDBID           string          `json:"imdb_id"`
	TVDBID           string          `json:"tvdb_id"`
	TMDBID           string          `json:"tmdb_id"`
	QualityProfileID string          `json:"quality_profile_id"`
	Request          interface{}     `json:"request"`
	Tiers            []TierDetail    `json:"tiers"`
	IndexerResults   []IndexerResult `json:"indexer_results"`
	Evaluation       []EvalResult    `json:"evaluation"`
	TotalResults     int             `json:"total_results"`
	TotalRejected    int             `json:"total_rejected"`
	GrabbedTitle     string          `json:"grabbed_title"`
	Outcome          string          `json:"outcome"`
	DurationMS       int64           `json:"duration_ms"`
	ErrorMessage     string          `json:"error_message,omitempty"`
}

// StatusUpdate is a lightweight event sent over SSE when a queue entry changes.
type StatusUpdate struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Outcome       string `json:"outcome,omitempty"`
	Title         string `json:"title"`
	MediaType     string `json:"media_type"`
	Season        int    `json:"season,omitempty"`
	Episode       int    `json:"episode,omitempty"`
	SearchRunID   string `json:"search_run_id,omitempty"`
	TotalResults  int    `json:"total_results"`
	TotalRejected int    `json:"total_rejected"`
	DurationMS    int64  `json:"duration_ms"`
	ErrorMessage  string `json:"error_message,omitempty"`
}

// TierDetail captures what happened at each query tier.
type TierDetail struct {
	TierIndex     int           `json:"tier_index"`
	Queries       []QueryDetail `json:"queries"`
	ResultCount   int           `json:"result_count"`
	AcceptedCount int           `json:"accepted_count"`
	RejectedCount int           `json:"rejected_count"`
	StoppedHere   bool          `json:"stopped_here"`
}

// QueryDetail is a sanitized version of an indexer query.
type QueryDetail struct {
	Term       string `json:"term,omitempty"`
	Mode       string `json:"mode,omitempty"`
	IMDBID     string `json:"imdb_id,omitempty"`
	TVDBID     string `json:"tvdb_id,omitempty"`
	TMDBID     string `json:"tmdb_id,omitempty"`
	Season     int    `json:"season,omitempty"`
	Episode    int    `json:"episode,omitempty"`
	Year       int    `json:"year,omitempty"`
	Categories []int  `json:"categories,omitempty"`
}

// IndexerResult captures per-indexer search results (sanitized — no download URLs).
type IndexerResult struct {
	IndexerID   string        `json:"indexer_id"`
	IndexerName string        `json:"indexer_name"`
	Status      string        `json:"status"` // "ok", "error", "timeout", "skipped"
	ResultCount int           `json:"result_count"`
	LatencyMS   int64         `json:"latency_ms"`
	Error       string        `json:"error,omitempty"`
	Results     []ResultEntry `json:"results,omitempty"`
}

// ResultEntry is a sanitized indexer result (no download URLs, passkeys, or magnets).
type ResultEntry struct {
	Title     string `json:"title"`
	Size      int64  `json:"size"`
	Seeders   *int   `json:"seeders,omitempty"`
	Peers     *int   `json:"peers,omitempty"`
	Quality   string `json:"quality,omitempty"`
	PubDate   string `json:"pub_date,omitempty"`
	Freeleech bool   `json:"freeleech,omitempty"`
	Internal  bool   `json:"internal,omitempty"`
	Scene     bool   `json:"scene,omitempty"`
	IndexerID string `json:"indexer_id"`
}

// EvalResult captures how a single result was evaluated.
type EvalResult struct {
	Title          string  `json:"title"`
	IndexerID      string  `json:"indexer_id"`
	Rejected       bool    `json:"rejected"`
	RejectReason   string  `json:"reject_reason,omitempty"`
	ParsedTitle    string  `json:"parsed_title,omitempty"`
	ParsedSource   string  `json:"parsed_source,omitempty"`
	ParsedRes      int     `json:"parsed_resolution,omitempty"`
	QualityName    string  `json:"quality_name,omitempty"`
	QualityTier    int     `json:"quality_tier"`
	FormatScore    int     `json:"format_score"`
	CompositeScore float64 `json:"composite_score"`
	Size           int64   `json:"size"`
	Seeders        *int    `json:"seeders,omitempty"`
}

// ListParams controls filtering and pagination for listing entries.
type ListParams struct {
	MediaType string
	MediaID   string
	Outcome   string
	Status    string
	Limit     int
	Offset    int
}
