package discover

import (
	"context"
	"database/sql"
	"fmt"
)

// Store persists per-user discovery dismissals.
type Store struct{ db *sql.DB }

// NewStore returns a dismissal store backed by db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func validMediaType(mediaType string) bool { return mediaType == "movie" || mediaType == "series" }

// Dismiss hides one movie or series suggestion for a user; repeats are no-ops.
func (s *Store) Dismiss(ctx context.Context, userID int64, mediaType string, tmdbID int) error {
	if userID <= 0 || !validMediaType(mediaType) || tmdbID <= 0 {
		return fmt.Errorf("invalid dismissal")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO discovery_dismissals (user_id, media_type, tmdb_id)
		VALUES ($1, $2, $3) ON CONFLICT (user_id, media_type, tmdb_id) DO NOTHING`, userID, mediaType, tmdbID)
	return err
}

// Dismissed returns the TMDB IDs a user has dismissed for a media type.
func (s *Store) Dismissed(ctx context.Context, userID int64, mediaType string) (map[int]bool, error) {
	if userID <= 0 || !validMediaType(mediaType) {
		return nil, fmt.Errorf("invalid dismissal owner")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT tmdb_id FROM discovery_dismissals WHERE user_id = $1 AND media_type = $2`, userID, mediaType)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
