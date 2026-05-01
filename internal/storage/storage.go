// Package storage is the database abstraction. Real wiring (modernc/sqlite,
// pgx for Postgres, goose migrations, sqlc-generated queries) lands as part
// of Phase 1's storage milestone. This file exists to hold the seam.
package storage

import (
	"context"
	"errors"

	"github.com/loomctl/loom/internal/kernel/config"
)

// DB is the narrow surface the rest of Loom depends on. It is intentionally
// not *sql.DB so we can swap engines or add a query-cache wrapper later.
type DB interface {
	Ping(ctx context.Context) error
	Close() error
	Engine() string // "sqlite" | "postgres"
}

// Open returns a DB for cfg. Currently returns a stub; real engines arrive
// with the Phase-1 storage milestone.
func Open(_ context.Context, cfg config.DatabaseConfig) (DB, error) {
	if cfg.URL == "" {
		return &stub{engine: "sqlite"}, nil
	}
	return &stub{engine: "postgres"}, nil
}

type stub struct{ engine string }

func (s *stub) Ping(_ context.Context) error { return nil }
func (s *stub) Close() error                  { return nil }
func (s *stub) Engine() string                { return s.engine }

// ErrNotImplemented is returned for storage operations that haven't been
// wired up yet. Callers check via errors.Is.
var ErrNotImplemented = errors.New("storage: not implemented yet")
