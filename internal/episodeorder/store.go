package episodeorder

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Store handles SQLite persistence for episode ordering mappings.
type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{db: db, logger: logger}
}

// ListMappings returns all episode mappings for a series, optionally
// filtered by ordering type.
func (s *Store) ListMappings(ctx context.Context, seriesID string, orderingType string) ([]EpisodeMapping, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if orderingType != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, series_id, ordering_type, season_from, episode_from, absolute_from,
			       season_to, episode_to, absolute_to, source, created_at
			FROM episode_mappings
			WHERE series_id = ? AND ordering_type = ?
			ORDER BY season_from, episode_from, absolute_from`, seriesID, orderingType)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, series_id, ordering_type, season_from, episode_from, absolute_from,
			       season_to, episode_to, absolute_to, source, created_at
			FROM episode_mappings
			WHERE series_id = ?
			ORDER BY ordering_type, season_from, episode_from, absolute_from`, seriesID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EpisodeMapping
	for rows.Next() {
		var m EpisodeMapping
		var createdAt string
		if err := rows.Scan(&m.ID, &m.SeriesID, &m.OrderingType, &m.SeasonFrom, &m.EpisodeFrom,
			&m.AbsoluteFrom, &m.SeasonTo, &m.EpisodeTo, &m.AbsoluteTo, &m.Source, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateMapping inserts a new episode mapping.
func (s *Store) CreateMapping(ctx context.Context, m *EpisodeMapping) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	if m.Source == "" {
		m.Source = SourceManual
	}
	m.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO episode_mappings
		  (id, series_id, ordering_type, season_from, episode_from, absolute_from,
		   season_to, episode_to, absolute_to, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SeriesID, m.OrderingType, m.SeasonFrom, m.EpisodeFrom, m.AbsoluteFrom,
		m.SeasonTo, m.EpisodeTo, m.AbsoluteTo, m.Source, m.CreatedAt)
	return err
}

// DeleteMapping removes a single mapping by ID.
func (s *Store) DeleteMapping(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM episode_mappings WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
