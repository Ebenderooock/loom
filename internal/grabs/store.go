// Package grabs tracks which downloads are linked to which media items
// (episodes / movies). This lets the UI show "grabbed" / "downloading"
// status on the detail pages.
package grabs

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Grab represents an active download linked to media items.
type Grab struct {
	ID         string    `json:"id"`
	ClientID   string    `json:"client_id"`
	DownloadID string    `json:"download_id"`
	Title      string    `json:"title"`
	GrabbedAt  time.Time `json:"grabbed_at"`
}

// Store persists active-grab linkages in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new grab store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// RecordEpisodeGrab inserts a grab and links it to one or more episodes.
func (s *Store) RecordEpisodeGrab(ctx context.Context, clientID, downloadID, title string, episodeIDs []string) error {
	if len(episodeIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	grabID := uuid.New().String()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO active_grabs (id, client_id, download_id, title)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (client_id, download_id) DO UPDATE SET title = excluded.title`,
		grabID, clientID, downloadID, title,
	)
	if err != nil {
		return err
	}

	// Get the actual ID (may differ on conflict)
	var existingID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM active_grabs WHERE client_id = ? AND download_id = ?`,
		clientID, downloadID,
	).Scan(&existingID)
	if err != nil {
		return err
	}

	for _, epID := range episodeIDs {
		_, err = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO active_grab_episodes (grab_id, episode_id)
			 VALUES (?, ?)`,
			existingID, epID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RecordMovieGrab inserts a grab and links it to a movie.
func (s *Store) RecordMovieGrab(ctx context.Context, clientID, downloadID, title, movieID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	grabID := uuid.New().String()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO active_grabs (id, client_id, download_id, title)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (client_id, download_id) DO UPDATE SET title = excluded.title`,
		grabID, clientID, downloadID, title,
	)
	if err != nil {
		return err
	}

	var existingID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM active_grabs WHERE client_id = ? AND download_id = ?`,
		clientID, downloadID,
	).Scan(&existingID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO active_grab_movies (grab_id, movie_id)
		 VALUES (?, ?)`,
		existingID, movieID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GrabbedEpisodeIDs returns the set of episode IDs that have active grabs.
func (s *Store) GrabbedEpisodeIDs(ctx context.Context, episodeIDs []string) (map[string]bool, error) {
	if len(episodeIDs) == 0 {
		return make(map[string]bool), nil
	}

	query := `SELECT DISTINCT age.episode_id
		FROM active_grab_episodes age
		JOIN active_grabs ag ON ag.id = age.grab_id
		WHERE age.episode_id IN (`
	args := make([]interface{}, len(episodeIDs))
	for i, id := range episodeIDs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = id
	}
	query += ")"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var epID string
		if err := rows.Scan(&epID); err != nil {
			return nil, err
		}
		result[epID] = true
	}
	return result, rows.Err()
}

// GrabbedMovieIDs returns the set of movie IDs that have active grabs.
func (s *Store) GrabbedMovieIDs(ctx context.Context, movieIDs []string) (map[string]bool, error) {
	if len(movieIDs) == 0 {
		return make(map[string]bool), nil
	}

	query := `SELECT DISTINCT agm.movie_id
		FROM active_grab_movies agm
		JOIN active_grabs ag ON ag.id = agm.grab_id
		WHERE agm.movie_id IN (`
	args := make([]interface{}, len(movieIDs))
	for i, id := range movieIDs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = id
	}
	query += ")"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var mID string
		if err := rows.Scan(&mID); err != nil {
			return nil, err
		}
		result[mID] = true
	}
	return result, rows.Err()
}

// RemoveByDownload removes a grab when a download completes or is removed.
func (s *Store) RemoveByDownload(ctx context.Context, clientID, downloadID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM active_grabs WHERE client_id = ? AND download_id = ?`,
		clientID, downloadID,
	)
	return err
}

// RemoveByEpisode removes grab linkages for a specific episode.
func (s *Store) RemoveByEpisode(ctx context.Context, episodeID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM active_grab_episodes WHERE episode_id = ?`, episodeID,
	)
	if err != nil {
		return err
	}
	// Clean up orphaned grabs
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM active_grabs WHERE id NOT IN (
			SELECT grab_id FROM active_grab_episodes
			UNION
			SELECT grab_id FROM active_grab_movies
		)`,
	)
	return err
}

// RemoveByMovie removes grab linkages for a specific movie.
func (s *Store) RemoveByMovie(ctx context.Context, movieID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM active_grab_movies WHERE movie_id = ?`, movieID,
	)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM active_grabs WHERE id NOT IN (
			SELECT grab_id FROM active_grab_episodes
			UNION
			SELECT grab_id FROM active_grab_movies
		)`,
	)
	return err
}

// GrabMedia describes the media linked to a grab record.
type GrabMedia struct {
	EpisodeIDs []string
	MovieIDs   []string
}

// PruneStale removes grabs older than maxAge. This prevents stale grabs
// from accumulating if downloads are removed externally.
func (s *Store) PruneStale(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM active_grabs WHERE grabbed_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// LookupByDownload returns the media linkage for a grab identified by
// clientID + downloadID. Returns nil if no grab exists.
func (s *Store) LookupByDownload(ctx context.Context, clientID, downloadID string) (*GrabMedia, error) {
	var grabID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM active_grabs WHERE client_id = ? AND download_id = ?`,
		clientID, downloadID,
	).Scan(&grabID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	gm := &GrabMedia{}

	epRows, err := s.db.QueryContext(ctx,
		`SELECT episode_id FROM active_grab_episodes WHERE grab_id = ?`, grabID)
	if err != nil {
		return nil, err
	}
	defer epRows.Close()
	for epRows.Next() {
		var id string
		if err := epRows.Scan(&id); err != nil {
			return nil, err
		}
		gm.EpisodeIDs = append(gm.EpisodeIDs, id)
	}
	if err := epRows.Err(); err != nil {
		return nil, err
	}

	mvRows, err := s.db.QueryContext(ctx,
		`SELECT movie_id FROM active_grab_movies WHERE grab_id = ?`, grabID)
	if err != nil {
		return nil, err
	}
	defer mvRows.Close()
	for mvRows.Next() {
		var id string
		if err := mvRows.Scan(&id); err != nil {
			return nil, err
		}
		gm.MovieIDs = append(gm.MovieIDs, id)
	}

	return gm, mvRows.Err()
}
