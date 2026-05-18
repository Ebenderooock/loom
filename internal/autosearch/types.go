// Package autosearch implements the automated search decision engine.
// Given a media item and its quality profile, it searches indexers,
// parses and scores each result against the profile's quality tiers
// and custom formats, then grabs the best qualifying release.
package autosearch

import (
	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
)

// SearchRequest describes what to search for and which quality
// profile to evaluate results against.
type SearchRequest struct {
	MediaType        string   `json:"media_type"`         // "movie", "series", or "episode"
	MediaID          string   `json:"media_id"`           // movie or series UUID
	Title            string   `json:"title"`              // primary title (used for query building)
	AlternateTitles  []string `json:"alternate_titles,omitempty"` // scene/alternate titles
	Year             int      `json:"year,omitempty"`      // release year for movies (for ±1 verification and text query)
	QualityProfileID string   `json:"quality_profile_id"` // profile to score against
	IMDBID           string   `json:"imdb_id,omitempty"`
	TMDBID           string   `json:"tmdb_id,omitempty"`
	TVDBID           string   `json:"tvdb_id,omitempty"`
	Season           int      `json:"season,omitempty"`
	Episode          int      `json:"episode,omitempty"`
	DailyDate        string   `json:"daily_date,omitempty"` // "YYYY-MM-DD" for daily shows
	Runtime          int      `json:"runtime,omitempty"`    // minutes; used for per-minute size limits
}

// SearchResult is returned by Engine.SearchAndGrab.
type SearchResult struct {
	Grabbed    *GrabbedRelease `json:"grabbed,omitempty"`
	Considered int             `json:"considered"` // total indexer results
	Rejected   int             `json:"rejected"`   // filtered out
	Reason     string          `json:"reason,omitempty"`
	TopRejects []RejectStat    `json:"top_rejects,omitempty"`
}

// GrabbedRelease describes the release that was sent to a download client.
type GrabbedRelease struct {
	Title                string                    `json:"title"`
	IndexerID            string                    `json:"indexer_id"`
	Size                 int64                     `json:"size"`
	QualityTier          int                       `json:"quality_tier"`
	FormatScore          int                       `json:"format_score"`
	CompositeScore       float64                   `json:"composite_score"`
	FormatMatches        []customformats.FormatMatch `json:"format_matches,omitempty"`
	ClientID             string                    `json:"client_id"`
	DownloadID           string                    `json:"download_id"`
	SeedRatioLimit       *float64                  `json:"seed_ratio_limit,omitempty"`
	SeedTimeLimitMinutes *int                      `json:"seed_time_limit_minutes,omitempty"`
}

// ScoredRelease is a search result after parsing, quality matching,
// and scoring. Used internally for ranking and selection.
type ScoredRelease struct {
	Result         indexers.Result
	Parsed         *parser.Release
	QualityDef     *movies.QualityDefinition // matched quality definition, nil if unmatched
	QualityTier    int                       // position in profile (lower = better)
	FormatScore    int                       // sum of custom format scores from profile
	FormatMatches  []customformats.FormatMatch
	TiebreakerScore float64                  // seeders + age + size + freeleech bonuses
	Rejected       bool
	RejectReason   string
}

// CompositeScore returns the overall score used for ranking.
// Ranking is hierarchical: quality tier dominates, then format score,
// then tiebreaker. We use weighted buckets so that a higher quality
// tier always beats a lower one regardless of format/tiebreaker scores.
func (sr *ScoredRelease) CompositeScore() float64 {
	// Quality tier: higher tier number = worse quality, so invert.
	// Max reasonable tier depth is ~20, so 1000 * (20 - tier) gives
	// quality a dominant range of 0–20000.
	qualityWeight := float64(20-sr.QualityTier) * 1000

	// Format score can be large (e.g., -10000 for "Avoid CAM"), so
	// it sits at the second tier of importance (range ≈ -10000 to +10000).
	formatWeight := float64(sr.FormatScore)

	// Tiebreaker is 0–100 range from seeders/age/size/freeleech.
	return qualityWeight + formatWeight + sr.TiebreakerScore
}

// RejectStat aggregates rejection reasons for the response.
type RejectStat struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// EvaluateRequest is accepted by the /evaluate endpoint: indexer results
// are scored against a quality profile without grabbing anything.
type EvaluateRequest struct {
	SearchRequest
	Results []indexers.Result `json:"results"`
}

// EvaluatedResult is a single result scored by the evaluate endpoint.
type EvaluatedResult struct {
	// Original indexer result fields (echoed back).
	IndexerID   string   `json:"indexer_id"`
	Title       string   `json:"title"`
	Link        string   `json:"link"`
	SizeBytes   int64    `json:"size_bytes"`
	Seeders     int      `json:"seeders"`
	Leechers    int      `json:"leechers"`
	PublishDate string   `json:"publish_date,omitempty"`
	Categories  []int    `json:"categories,omitempty"`
	MagnetURI   string   `json:"magnet_uri,omitempty"`
	Infohash    string   `json:"infohash,omitempty"`
	InfoURL     string   `json:"info_url,omitempty"`
	Freeleech   bool     `json:"freeleech,omitempty"`

	// Evaluation fields.
	Rejected       bool                       `json:"rejected"`
	RejectReason   string                     `json:"reject_reason,omitempty"`
	QualityName    string                     `json:"quality_name,omitempty"`
	QualityTier    int                        `json:"quality_tier"`
	FormatScore    int                        `json:"format_score"`
	FormatMatches  []customformats.FormatMatch `json:"format_matches,omitempty"`
	CompositeScore float64                    `json:"composite_score"`
	ParsedTitle    string                     `json:"parsed_title,omitempty"`
	ParsedYear     int                        `json:"parsed_year,omitempty"`
	ParsedSource   string                     `json:"parsed_source,omitempty"`
	ParsedRes      int                        `json:"parsed_resolution,omitempty"`
}

// EvaluateResponse is the response from the /evaluate endpoint.
type EvaluateResponse struct {
	Results []EvaluatedResult `json:"results"`
	Total   int               `json:"total"`
	Passed  int               `json:"passed"`
}
