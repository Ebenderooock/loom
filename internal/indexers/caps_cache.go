package indexers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sqlc-dev/pqtype"

	dbpg "github.com/ebenderooock/loom/internal/storage/db/postgres"
	dbsqlite "github.com/ebenderooock/loom/internal/storage/db/sqlite"
)

// CapsCache persists the last-known Caps document for an indexer so a
// restart doesn't blank-state every kind that does network discovery
// (Newznab/Torznab `t=caps`). It is intentionally split from
// Repository: kinds that don't need caching (e.g. builtin/null) ignore
// it; kinds that do (newznab, torznab) wire it via their factory.
//
// Implementations must be safe for concurrent use; the sqlc-generated
// query packages we delegate to already are.
type CapsCache interface {
	// Load returns the cached Caps for id, ok=false when the row has
	// never been populated. A decode failure is reported as err.
	Load(ctx context.Context, id string) (caps Caps, ok bool, err error)

	// Save replaces the cached Caps for id. It is a no-op when no
	// indexer_health row exists yet — Service.Create always seeds an
	// "unknown" row at create time, so callers reach a populated row
	// after the first request.
	Save(ctx context.Context, id string, caps Caps) error
}

// NewSQLiteCapsCache wires a CapsCache over the sqlc-generated SQLite
// queries.
func NewSQLiteCapsCache(db *sql.DB) CapsCache {
	return &sqliteCapsCache{q: dbsqlite.New(db)}
}

// NewPostgresCapsCache wires a CapsCache over the sqlc-generated
// Postgres queries.
func NewPostgresCapsCache(db *sql.DB) CapsCache {
	return &pgCapsCache{q: dbpg.New(db)}
}

type sqliteCapsCache struct{ q *dbsqlite.Queries }

func (s *sqliteCapsCache) Load(ctx context.Context, id string) (Caps, bool, error) {
	row, err := s.q.GetIndexerCapsCache(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Caps{}, false, nil
		}
		return Caps{}, false, fmt.Errorf("caps cache load %q: %w", id, err)
	}
	if !row.Valid || row.String == "" {
		return Caps{}, false, nil
	}
	var caps Caps
	if err := json.Unmarshal([]byte(row.String), &caps); err != nil {
		return Caps{}, false, fmt.Errorf("caps cache decode %q: %w", id, err)
	}
	return caps, true, nil
}

func (s *sqliteCapsCache) Save(ctx context.Context, id string, caps Caps) error {
	raw, err := json.Marshal(caps)
	if err != nil {
		return fmt.Errorf("caps cache encode %q: %w", id, err)
	}
	if err := s.q.UpdateIndexerCapsCache(ctx, dbsqlite.UpdateIndexerCapsCacheParams{
		IndexerID:    id,
		LastCapsJson: sql.NullString{String: string(raw), Valid: true},
	}); err != nil {
		return fmt.Errorf("caps cache save %q: %w", id, err)
	}
	return nil
}

type pgCapsCache struct{ q *dbpg.Queries }

func (p *pgCapsCache) Load(ctx context.Context, id string) (Caps, bool, error) {
	row, err := p.q.GetIndexerCapsCache(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Caps{}, false, nil
		}
		return Caps{}, false, fmt.Errorf("caps cache load %q: %w", id, err)
	}
	if !row.Valid || len(row.RawMessage) == 0 {
		return Caps{}, false, nil
	}
	var caps Caps
	if err := json.Unmarshal(row.RawMessage, &caps); err != nil {
		return Caps{}, false, fmt.Errorf("caps cache decode %q: %w", id, err)
	}
	return caps, true, nil
}

func (p *pgCapsCache) Save(ctx context.Context, id string, caps Caps) error {
	raw, err := json.Marshal(caps)
	if err != nil {
		return fmt.Errorf("caps cache encode %q: %w", id, err)
	}
	if err := p.q.UpdateIndexerCapsCache(ctx, dbpg.UpdateIndexerCapsCacheParams{
		IndexerID:    id,
		LastCapsJson: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
	}); err != nil {
		return fmt.Errorf("caps cache save %q: %w", id, err)
	}
	return nil
}
