package appearance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ebenderooock/loom/internal/storage"
)

const (
	settingsKey = "appearance"
	// At most two images are stored: the one referenced by saved settings and
	// the latest upload awaiting Save. Uploading replaces the staged slot.
	activeBackgroundKey = "appearance.background.active"
	stagedBackgroundKey = "appearance.background.staged"
)

// ErrBackgroundUnavailable means settings reference an uploaded image that is
// not stored (for example, it was replaced by a newer upload).
var ErrBackgroundUnavailable = errors.New("backgroundUrl references an uploaded image that is not available; upload it again")

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

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) read(ctx context.Context, q querier, key string) (string, bool, error) {
	var raw string
	err := q.QueryRowContext(ctx, storage.RebindQuery(
		`SELECT value FROM schema_meta WHERE key = ?`, s.postgres), key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return raw, err == nil, err
}

func (s *Store) write(ctx context.Context, q querier, key, value string) error {
	_, err := q.ExecContext(ctx, storage.RebindQuery(
		`INSERT INTO schema_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, s.postgres), key, value)
	return err
}

// Get returns saved appearance settings, or defaults when none are saved.
// Settings saved before palettes existed decode with a nil Palette.
func (s *Store) Get(ctx context.Context) (Settings, error) {
	raw, ok, err := s.read(ctx, s.db, settingsKey)
	if err != nil {
		return Settings{}, err
	}
	if !ok {
		return Defaults(), nil
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

// Set atomically replaces the saved appearance settings. When the background
// references an uploaded image, a matching staged upload is promoted to the
// active slot in the same transaction; an unknown image returns
// ErrBackgroundUnavailable.
func (s *Store) Set(ctx context.Context, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if hash, internal := InternalBackground(settings.BackgroundURL); internal {
		if err := s.activate(ctx, tx, hash); err != nil {
			return err
		}
	} else if err := s.remove(ctx, tx, activeBackgroundKey); err != nil {
		// The saved background no longer uses the upload; retire it.
		return err
	}
	if err := s.write(ctx, tx, settingsKey, string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) activate(ctx context.Context, tx *sql.Tx, hash string) error {
	active, err := s.image(ctx, tx, activeBackgroundKey)
	if err != nil {
		return err
	}
	if active != nil && (hash == "" || active.SHA256 == hash) {
		return nil
	}
	stagedRaw, ok, err := s.read(ctx, tx, stagedBackgroundKey)
	if err != nil {
		return err
	}
	if !ok {
		return ErrBackgroundUnavailable
	}
	staged, err := decodeImage(stagedRaw)
	if err != nil {
		return err
	}
	if hash != "" && staged.SHA256 != hash {
		return ErrBackgroundUnavailable
	}
	if err := s.write(ctx, tx, activeBackgroundKey, stagedRaw); err != nil {
		return err
	}
	return s.remove(ctx, tx, stagedBackgroundKey)
}

func (s *Store) remove(ctx context.Context, q querier, key string) error {
	_, err := q.ExecContext(ctx, storage.RebindQuery(`DELETE FROM schema_meta WHERE key = ?`, s.postgres), key)
	return err
}

func decodeImage(raw string) (*BackgroundImage, error) {
	var img BackgroundImage
	if err := json.Unmarshal([]byte(raw), &img); err != nil {
		return nil, fmt.Errorf("decode background image: %w", err)
	}
	if img.SHA256 == "" || len(img.Data) == 0 || img.Size != len(img.Data) {
		return nil, errors.New("decode background image: incomplete record")
	}
	return &img, nil
}

func (s *Store) image(ctx context.Context, q querier, key string) (*BackgroundImage, error) {
	raw, ok, err := s.read(ctx, q, key)
	if err != nil || !ok {
		return nil, err
	}
	return decodeImage(raw)
}

// StageBackground stores a validated upload, replacing any previous staged
// upload. Saved settings are unchanged until they reference its URL.
func (s *Store) StageBackground(ctx context.Context, img *BackgroundImage) error {
	raw, err := json.Marshal(img)
	if err != nil {
		return err
	}
	return s.write(ctx, s.db, stagedBackgroundKey, string(raw))
}

// Background returns the active or staged image with the given hash, or the
// active (saved) image when hash is empty.
func (s *Store) Background(ctx context.Context, hash string) (*BackgroundImage, error) {
	keys := []string{activeBackgroundKey, stagedBackgroundKey}
	if hash == "" {
		keys = keys[:1]
	}
	for _, key := range keys {
		img, err := s.image(ctx, s.db, key)
		if err != nil {
			return nil, err
		}
		if img != nil && (hash == "" || img.SHA256 == hash) {
			return img, nil
		}
	}
	return nil, ErrBackgroundNotFound
}
