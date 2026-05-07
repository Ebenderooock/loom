// Package storage owns the database layer: engine selection, connection
// lifecycle, schema migrations (via goose), and the seam consumed by the
// rest of Loom. Two engines are supported: SQLite (modernc.org/sqlite,
// pure-Go) for the single-binary deployment and Postgres (jackc/pgx via
// database/sql) for shared/HA installs. Migrations live under
// migrations/<engine>/ and are embedded; sqlc-generated query packages
// live under db/sqlite and db/postgres.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

// Engine identifies a concrete storage backend.
type Engine string

const (
	EngineSQLite   Engine = "sqlite"
	EnginePostgres Engine = "postgres"
)

// DB is the narrow surface the rest of Loom depends on. The raw *sql.DB
// is exposed so sqlc-generated query packages can consume it directly;
// callers that only need lifecycle should use Ping/Close/Migrate.
type DB interface {
	DB() *sql.DB
	Engine() Engine
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) error
	Close() error
}

// ErrNotImplemented marks paths that are intentionally stubbed.
var ErrNotImplemented = errors.New("storage: not implemented yet")

// Open returns a DB for cfg, dispatching on cfg.Engine. The logger is
// passed to engine implementations for migration progress.
func Open(ctx context.Context, cfg config.StorageConfig, logger *slog.Logger) (DB, error) {
	if logger == nil {
		logger = slog.Default()
	}
	switch Engine(strings.ToLower(strings.TrimSpace(cfg.Engine))) {
	case EngineSQLite, "":
		return openSQLite(ctx, cfg.SQLite, logger)
	case EnginePostgres:
		return openPostgres(ctx, cfg.Postgres, logger)
	default:
		return nil, fmt.Errorf("storage: unknown engine %q", cfg.Engine)
	}
}
