package alttitles

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Store handles SQLite persistence for alternate titles.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new alternate title. The ID and CreatedAt fields are
// populated automatically if empty.
func (s *Store) Create(ctx context.Context, alt *AltTitle) error {
	if alt.ID == "" {
		alt.ID = uuid.New().String()
	}
	if alt.Language == "" {
		alt.Language = "en"
	}
	if alt.Source == "" {
		alt.Source = "manual"
	}
	alt.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alternate_titles (id, media_id, media_type, title, language, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, title) DO NOTHING`,
		alt.ID, alt.MediaID, alt.MediaType, alt.Title, alt.Language, alt.Source,
		alt.CreatedAt.Format("2006-01-02T15:04:05Z"),
	)
	return err
}

// Delete removes an alternate title by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alternate_titles WHERE id = ?`, id)
	return err
}

// GetByMediaID returns all alternate titles for a given media item.
func (s *Store) GetByMediaID(ctx context.Context, mediaID, mediaType string) ([]*AltTitle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, media_id, media_type, title, language, source, created_at
		FROM alternate_titles
		WHERE media_id = ? AND media_type = ?
		ORDER BY created_at ASC`, mediaID, mediaType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AltTitle
	for rows.Next() {
		a := &AltTitle{}
		var ts string
		if err := rows.Scan(&a.ID, &a.MediaID, &a.MediaType, &a.Title, &a.Language, &a.Source, &ts); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ts)
		out = append(out, a)
	}
	return out, rows.Err()
}

// SearchByTitle returns all alternate titles matching the given title
// (case-insensitive) filtered by media type ("movie" or "series").
func (s *Store) SearchByTitle(ctx context.Context, title, mediaType string) ([]*AltTitle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, media_id, media_type, title, language, source, created_at
		FROM alternate_titles
		WHERE media_type = ? AND LOWER(title) = LOWER(?)
		ORDER BY created_at ASC`, mediaType, title)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AltTitle
	for rows.Next() {
		a := &AltTitle{}
		var ts string
		if err := rows.Scan(&a.ID, &a.MediaID, &a.MediaType, &a.Title, &a.Language, &a.Source, &ts); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ts)
		out = append(out, a)
	}
	return out, rows.Err()
}
