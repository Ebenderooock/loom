package featureflags

import (
	"context"
	"database/sql"
)

// Store persists feature flag overrides.
type Store struct {
	db *sql.DB
}

// NewStore creates a feature flag store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetAll returns all persisted overrides keyed by flag key.
func (s *Store) GetAll(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, enabled FROM feature_flags`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var key string
		var enabled bool
		if err := rows.Scan(&key, &enabled); err != nil {
			return nil, err
		}
		out[key] = enabled
	}
	return out, rows.Err()
}

// Set upserts the override for a flag.
func (s *Store) Set(ctx context.Context, key string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO feature_flags (key, enabled, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET enabled = excluded.enabled, updated_at = CURRENT_TIMESTAMP`,
		key, enabled,
	)
	return err
}
