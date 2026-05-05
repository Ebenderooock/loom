package packs

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"
)

// Store handles SQLite persistence for season pack history.
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

// RecordPackGrab inserts a pack-grab event into history.
func (s *Store) RecordPackGrab(ctx context.Context, h *PackHistory) error {
	epsJSON, _ := json.Marshal(h.EpisodesIncluded)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO season_pack_history (id, series_id, season, pack_title, episodes_included, quality, grabbed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.SeriesID, h.Season, h.PackTitle, string(epsJSON), h.Quality, h.GrabbedAt)
	return err
}

// ListHistory returns pack-grab history for a series, newest first.
// If seriesID is empty, all history is returned.
func (s *Store) ListHistory(ctx context.Context, seriesID string) ([]PackHistory, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if seriesID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, series_id, season, pack_title, episodes_included, quality, grabbed_at
			FROM season_pack_history WHERE series_id = ? ORDER BY grabbed_at DESC`, seriesID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, series_id, season, pack_title, episodes_included, quality, grabbed_at
			FROM season_pack_history ORDER BY grabbed_at DESC LIMIT 100`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PackHistory
	for rows.Next() {
		var h PackHistory
		var epsJSON string
		var grabbedAt string
		if err := rows.Scan(&h.ID, &h.SeriesID, &h.Season, &h.PackTitle, &epsJSON, &h.Quality, &grabbedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(epsJSON), &h.EpisodesIncluded)
		h.GrabbedAt, _ = time.Parse("2006-01-02 15:04:05", grabbedAt)
		out = append(out, h)
	}
	return out, rows.Err()
}
