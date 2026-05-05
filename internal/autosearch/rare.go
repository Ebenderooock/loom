package autosearch

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// RareContentConfig controls how rare/old content is detected and searched.
type RareContentConfig struct {
	Enabled            bool `json:"enabled"`
	CutoffYears        int  `json:"cutoff_years"`
	MinFailedSearches  int  `json:"min_failed_searches"`
	LowerQualityFloor  bool `json:"lower_quality_floor"`
	UseAlternateTitles bool `json:"use_alternate_titles"`
}

// DefaultRareContentConfig returns sensible defaults.
func DefaultRareContentConfig() RareContentConfig {
	return RareContentConfig{
		Enabled:            true,
		CutoffYears:        5,
		MinFailedSearches:  3,
		LowerQualityFloor:  true,
		UseAlternateTitles: true,
	}
}

// RareCandidate represents a media item identified as rare/old content.
type RareCandidate struct {
	MediaType       string     `json:"media_type"`
	MediaID         string     `json:"media_id"`
	Title           string     `json:"title"`
	Year            int        `json:"year"`
	FailedSearches  int        `json:"failed_searches"`
	LastSearchedAt  *time.Time `json:"last_searched_at,omitempty"`
	AlternateTitles []string   `json:"alternate_titles,omitempty"`
	IsRare          bool       `json:"is_rare"`
	Reason          string     `json:"reason"`
}

// RareContentStrategy detects and handles rare/old content searches.
type RareContentStrategy struct {
	db     *sql.DB
	config RareContentConfig
	logger *slog.Logger
}

// NewRareContentStrategy creates a new rare content handler.
func NewRareContentStrategy(db *sql.DB, logger *slog.Logger) *RareContentStrategy {
	return &RareContentStrategy{
		db:     db,
		config: DefaultRareContentConfig(),
		logger: logger,
	}
}

// UpdateConfig replaces the current configuration.
func (r *RareContentStrategy) UpdateConfig(cfg RareContentConfig) {
	r.config = cfg
}

// Config returns the current configuration.
func (r *RareContentStrategy) Config() RareContentConfig {
	return r.config
}

// IsRare determines if a media item qualifies as "rare" content.
func (r *RareContentStrategy) IsRare(year int, failedSearches int) (bool, string) {
	if !r.config.Enabled {
		return false, "rare content detection disabled"
	}

	cutoffYear := time.Now().Year() - r.config.CutoffYears
	isOld := year > 0 && year <= cutoffYear
	hasFailedSearches := failedSearches >= r.config.MinFailedSearches

	if isOld && hasFailedSearches {
		return true, "old content with repeated search failures"
	}
	if hasFailedSearches && failedSearches >= r.config.MinFailedSearches*2 {
		return true, "excessive search failures regardless of age"
	}
	return false, ""
}

// BuildSearchTerms generates alternative search terms for rare content.
func (r *RareContentStrategy) BuildSearchTerms(title string, year int, alternateTitles []string) []string {
	terms := []string{title}

	if year > 0 {
		terms = append(terms, title+" "+string(rune('0'+year/1000%10))+string(rune('0'+year/100%10))+string(rune('0'+year/10%10))+string(rune('0'+year%10)))
	}

	if r.config.UseAlternateTitles {
		for _, alt := range alternateTitles {
			if alt != title {
				terms = append(terms, alt)
			}
		}
	}

	return terms
}

// SuggestQualityFloor returns whether a lower quality floor should be
// used for this content. For rare items, accept 720p even if 1080p
// is preferred.
func (r *RareContentStrategy) SuggestQualityFloor(isRare bool) string {
	if isRare && r.config.LowerQualityFloor {
		return "720p"
	}
	return ""
}

// DetectRareCandidates scans the database for media matching rare criteria.
func (r *RareContentStrategy) DetectRareCandidates(ctx context.Context) ([]RareCandidate, error) {
	cutoffYear := time.Now().Year() - r.config.CutoffYears

	rows, err := r.db.QueryContext(ctx, `
		SELECT sh.movie_id, sh.title, sh.year, COUNT(*) as failed_count,
		       MAX(sh.created_at) as last_search
		FROM search_history sh
		WHERE sh.status IN ('not_found', 'error')
		GROUP BY sh.movie_id
		HAVING failed_count >= ?
		ORDER BY failed_count DESC
	`, r.config.MinFailedSearches)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []RareCandidate
	for rows.Next() {
		var c RareCandidate
		var lastSearch time.Time
		if err := rows.Scan(&c.MediaID, &c.Title, &c.Year, &c.FailedSearches, &lastSearch); err != nil {
			r.logger.Warn("scan rare candidate failed", "error", err)
			continue
		}
		c.LastSearchedAt = &lastSearch
		c.MediaType = "movie"

		isRare, reason := r.IsRare(c.Year, c.FailedSearches)
		c.IsRare = isRare
		c.Reason = reason

		if isRare || c.Year <= cutoffYear {
			candidates = append(candidates, c)
		}
	}
	return candidates, rows.Err()
}
