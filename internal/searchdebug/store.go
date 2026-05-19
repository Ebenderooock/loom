package searchdebug

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Store persists search debug log entries.
type Store struct {
	db *sql.DB
}

// NewStore creates a new search debug log store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// NewID returns a new UUID for a debug log entry.
func NewID() string {
	return uuid.New().String()
}

// Create inserts a new search debug log entry.
func (s *Store) Create(ctx context.Context, e *Entry) error {
	reqJSON, _ := json.Marshal(e.Request)
	tiersJSON, _ := json.Marshal(e.Tiers)
	indexerJSON, _ := json.Marshal(e.IndexerResults)
	evalJSON, _ := json.Marshal(e.Evaluation)

	now := time.Now()
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	if e.Status == "" {
		e.Status = StatusCompleted
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO search_debug_log (
			id, created_at, updated_at, status, search_run_id,
			media_type, media_id, title, year, season, episode,
			imdb_id, tvdb_id, tmdb_id, quality_profile_id,
			request_json, tiers_json, indexer_results_json, evaluation_json,
			total_results, total_rejected, grabbed_title, outcome, duration_ms, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.CreatedAt, e.UpdatedAt, e.Status, e.SearchRunID,
		e.MediaType, e.MediaID, e.Title, e.Year, e.Season, e.Episode,
		e.IMDBID, e.TVDBID, e.TMDBID, e.QualityProfileID,
		string(reqJSON), string(tiersJSON), string(indexerJSON), string(evalJSON),
		e.TotalResults, e.TotalRejected, e.GrabbedTitle, e.Outcome, e.DurationMS, e.ErrorMessage,
	)
	return err
}

// Update updates an existing entry's mutable fields.
func (s *Store) Update(ctx context.Context, e *Entry) error {
	tiersJSON, _ := json.Marshal(e.Tiers)
	indexerJSON, _ := json.Marshal(e.IndexerResults)
	evalJSON, _ := json.Marshal(e.Evaluation)

	e.UpdatedAt = time.Now()

	_, err := s.db.ExecContext(ctx, `
		UPDATE search_debug_log SET
			updated_at = ?, status = ?,
			tiers_json = ?, indexer_results_json = ?, evaluation_json = ?,
			total_results = ?, total_rejected = ?, grabbed_title = ?,
			outcome = ?, duration_ms = ?, error_message = ?
		WHERE id = ?`,
		e.UpdatedAt, e.Status,
		string(tiersJSON), string(indexerJSON), string(evalJSON),
		e.TotalResults, e.TotalRejected, e.GrabbedTitle,
		e.Outcome, e.DurationMS, e.ErrorMessage,
		e.ID,
	)
	return err
}

// Get retrieves a single debug log entry by ID.
func (s *Store) Get(ctx context.Context, id string) (*Entry, error) {
	var e Entry
	var reqJSON, tiersJSON, indexerJSON, evalJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at, status, search_run_id,
			media_type, media_id, title, year, season, episode,
			imdb_id, tvdb_id, tmdb_id, quality_profile_id,
			request_json, tiers_json, indexer_results_json, evaluation_json,
			total_results, total_rejected, grabbed_title, outcome, duration_ms, error_message
		FROM search_debug_log WHERE id = ?`, id,
	).Scan(
		&e.ID, &e.CreatedAt, &e.UpdatedAt, &e.Status, &e.SearchRunID,
		&e.MediaType, &e.MediaID, &e.Title, &e.Year, &e.Season, &e.Episode,
		&e.IMDBID, &e.TVDBID, &e.TMDBID, &e.QualityProfileID,
		&reqJSON, &tiersJSON, &indexerJSON, &evalJSON,
		&e.TotalResults, &e.TotalRejected, &e.GrabbedTitle, &e.Outcome, &e.DurationMS, &e.ErrorMessage,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(reqJSON), &e.Request)
	json.Unmarshal([]byte(tiersJSON), &e.Tiers)
	json.Unmarshal([]byte(indexerJSON), &e.IndexerResults)
	json.Unmarshal([]byte(evalJSON), &e.Evaluation)

	return &e, nil
}

// List returns debug log entries with optional filtering.
func (s *Store) List(ctx context.Context, p ListParams) ([]Entry, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	where := "1=1"
	args := []any{}
	if p.MediaType != "" {
		where += " AND media_type = ?"
		args = append(args, p.MediaType)
	}
	if p.MediaID != "" {
		where += " AND media_id = ?"
		args = append(args, p.MediaID)
	}
	if p.Outcome != "" {
		where += " AND outcome = ?"
		args = append(args, p.Outcome)
	}
	if p.Status != "" {
		if p.Status == "active" {
			where += " AND status NOT IN ('completed', 'failed', 'cancelled')"
		} else {
			where += " AND status = ?"
			args = append(args, p.Status)
		}
	}

	// Count total matching.
	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM search_debug_log WHERE "+where, countArgs...,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch page — list view omits large JSON blobs for performance.
	args = append(args, p.Limit, p.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, updated_at, status, search_run_id,
			media_type, media_id, title, year, season, episode,
			imdb_id, tvdb_id, tmdb_id, quality_profile_id,
			total_results, total_rejected, grabbed_title, outcome, duration_ms, error_message
		FROM search_debug_log
		WHERE `+where+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(
			&e.ID, &e.CreatedAt, &e.UpdatedAt, &e.Status, &e.SearchRunID,
			&e.MediaType, &e.MediaID, &e.Title, &e.Year, &e.Season, &e.Episode,
			&e.IMDBID, &e.TVDBID, &e.TMDBID, &e.QualityProfileID,
			&e.TotalResults, &e.TotalRejected, &e.GrabbedTitle, &e.Outcome, &e.DurationMS, &e.ErrorMessage,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	if out == nil {
		out = []Entry{}
	}
	return out, total, rows.Err()
}

// ListActive returns entries that are not in a terminal state.
func (s *Store) ListActive(ctx context.Context) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, updated_at, status, search_run_id,
			media_type, media_id, title, year, season, episode,
			imdb_id, tvdb_id, tmdb_id, quality_profile_id,
			total_results, total_rejected, grabbed_title, outcome, duration_ms, error_message
		FROM search_debug_log
		WHERE status NOT IN ('completed', 'failed', 'cancelled')
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(
			&e.ID, &e.CreatedAt, &e.UpdatedAt, &e.Status, &e.SearchRunID,
			&e.MediaType, &e.MediaID, &e.Title, &e.Year, &e.Season, &e.Episode,
			&e.IMDBID, &e.TVDBID, &e.TMDBID, &e.QualityProfileID,
			&e.TotalResults, &e.TotalRejected, &e.GrabbedTitle, &e.Outcome, &e.DurationMS, &e.ErrorMessage,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if out == nil {
		out = []Entry{}
	}
	return out, rows.Err()
}

// MarkStale transitions non-terminal entries older than maxAge to "failed".
// Called on startup to clean up entries from crashed searches.
func (s *Store) MarkStale(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	res, err := s.db.ExecContext(ctx, `
		UPDATE search_debug_log
		SET status = 'failed', outcome = 'stale', error_message = 'search did not complete (process restart or timeout)', updated_at = CURRENT_TIMESTAMP
		WHERE status NOT IN ('completed', 'failed', 'cancelled')
		AND updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Prune deletes entries older than the given duration. Returns count deleted.
func (s *Store) Prune(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM search_debug_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stats returns aggregate statistics about search outcomes.
func (s *Store) Stats(ctx context.Context) (*StatsResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT outcome, COUNT(*) as cnt
		FROM search_debug_log
		WHERE created_at > ?
		GROUP BY outcome
		ORDER BY cnt DESC`,
		time.Now().Add(-7*24*time.Hour),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &StatsResult{
		OutcomeCounts: make(map[string]int),
	}
	for rows.Next() {
		var outcome string
		var count int
		if err := rows.Scan(&outcome, &count); err != nil {
			return nil, err
		}
		result.OutcomeCounts[outcome] = count
		result.TotalSearches += count
	}

	// Top reject reasons.
	rejectRows, err := s.db.QueryContext(ctx, `
		SELECT evaluation_json FROM search_debug_log
		WHERE created_at > ? AND outcome IN ('all_rejected', 'no_results')
		ORDER BY created_at DESC LIMIT 100`,
		time.Now().Add(-7*24*time.Hour),
	)
	if err != nil {
		return result, nil
	}
	defer rejectRows.Close()

	rejectCounts := make(map[string]int)
	for rejectRows.Next() {
		var evalJSON string
		if err := rejectRows.Scan(&evalJSON); err != nil {
			continue
		}
		var evals []EvalResult
		if err := json.Unmarshal([]byte(evalJSON), &evals); err != nil {
			continue
		}
		for _, ev := range evals {
			if ev.Rejected && ev.RejectReason != "" {
				rejectCounts[ev.RejectReason]++
			}
		}
	}
	for reason, count := range rejectCounts {
		result.TopRejectReasons = append(result.TopRejectReasons, RejectStat{
			Reason: reason,
			Count:  count,
		})
	}

	return result, nil
}

// StatsResult holds aggregate search statistics.
type StatsResult struct {
	TotalSearches    int            `json:"total_searches"`
	OutcomeCounts    map[string]int `json:"outcome_counts"`
	TopRejectReasons []RejectStat   `json:"top_reject_reasons,omitempty"`
}

// RejectStat aggregates rejection reasons.
type RejectStat struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}
