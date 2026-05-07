package scheduler

import (
	"context"
	"database/sql"
	"time"
)

// Store persists rolling-search state (last-searched timestamps) so the
// scheduler does not re-search items too eagerly across restarts.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetCandidates returns up to limit items of the given mediaType that
// have not been searched within minResearchDays, ordered by priority
// (newer content first, then by when they were added).
func (s *Store) GetCandidates(ctx context.Context, mediaType string, limit int, minResearchDays int) ([]SearchCandidate, error) {
	cutoff := time.Now().AddDate(0, 0, -minResearchDays)

	var query string
	switch mediaType {
	case "movie":
		query = `
			SELECT m.id, m.title, m.year
			FROM movies m
			LEFT JOIN search_state ss ON ss.media_type = 'movie' AND ss.media_id = m.id
			WHERE m.status = 'missing'
			  AND m.monitoring_status = 'monitored'
			  AND (ss.last_searched_at IS NULL OR ss.last_searched_at < ?)
			ORDER BY m.year DESC, m.created_at DESC
			LIMIT ?`
	case "episode":
		query = `
			SELECT e.id, e.title, 0
			FROM episodes e
			JOIN series s ON e.series_id = s.id
			LEFT JOIN search_state ss ON ss.media_type = 'episode' AND ss.media_id = e.id
			WHERE e.has_file = 0
			  AND e.monitored = 1
			  AND s.monitoring_status NOT IN ('none', 'unmonitored', 'archived')
			  AND (e.air_date != '' AND e.air_date <= date('now'))
			  AND (ss.last_searched_at IS NULL OR ss.last_searched_at < ?)
			ORDER BY e.air_date DESC, e.created_at DESC
			LIMIT ?`
	default:
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, query, cutoff.Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []SearchCandidate
	pri := 0
	for rows.Next() {
		var c SearchCandidate
		c.MediaType = mediaType
		if err := rows.Scan(&c.MediaID, &c.Title, &c.Year); err != nil {
			return nil, err
		}
		c.Priority = pri
		pri++
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// RecordSearch upserts the last-searched timestamp for a media item.
func (s *Store) RecordSearch(ctx context.Context, mediaType, mediaID string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO search_state (media_type, media_id, last_searched_at, search_count)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(media_type, media_id)
		DO UPDATE SET last_searched_at = excluded.last_searched_at,
		              search_count = search_state.search_count + 1`,
		mediaType, mediaID, now)
	return err
}

// QueueSize returns the count of items that would be eligible for search.
func (s *Store) QueueSize(ctx context.Context, minResearchDays int) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -minResearchDays).Format(time.RFC3339)
	var count int

	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM movies m
		LEFT JOIN search_state ss ON ss.media_type = 'movie' AND ss.media_id = m.id
		WHERE m.status = 'missing'
		  AND m.monitoring_status = 'monitored'
		  AND (ss.last_searched_at IS NULL OR ss.last_searched_at < ?)`, cutoff)
	if err := row.Scan(&count); err != nil {
		return 0, err
	}

	var epCount int
	row = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM episodes e
		JOIN series s ON e.series_id = s.id
		LEFT JOIN search_state ss ON ss.media_type = 'episode' AND ss.media_id = e.id
		WHERE e.has_file = 0
		  AND e.monitored = 1
		  AND s.monitoring_status NOT IN ('none', 'unmonitored', 'archived')
		  AND (e.air_date != '' AND e.air_date <= date('now'))
		  AND (ss.last_searched_at IS NULL OR ss.last_searched_at < ?)`, cutoff)
	if err := row.Scan(&epCount); err != nil {
		return count, err
	}
	return count + epCount, nil
}
