package appearance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ebenderooock/loom/internal/storage"
)

const settingsKey = "appearance"

// Store persists the full configuration atomically in the existing key/value
// table. Reads go to the DB so updates are visible across server instances.
type Store struct {
	db       *sql.DB
	postgres bool
}

// NewStore creates a store backed by Loom's configured database.
func NewStore(db storage.DB) *Store {
	return &Store{db: db.DB(), postgres: db.Engine() == storage.EnginePostgres}
}

// Get returns saved appearance settings, or defaults when none are saved.
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, storage.RebindQuery(
		`SELECT value FROM schema_meta WHERE key = ?`, s.postgres), settingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Defaults(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	settings := Defaults()
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return Settings{}, fmt.Errorf("decode appearance settings: %w", err)
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, fmt.Errorf("invalid stored appearance settings: %w", err)
	}
	return settings, nil
}

// Set atomically replaces the saved appearance settings.
func (s *Store) Set(ctx context.Context, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, storage.RebindQuery(
		`INSERT INTO schema_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, s.postgres), settingsKey, string(raw))
	return err
}
