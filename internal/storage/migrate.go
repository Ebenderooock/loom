package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrationsFS embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrationsFS embed.FS

// migrationsFor returns the embedded sub-FS rooted at migrations/<engine>
// so goose sees the .sql files as if they lived at the root.
func migrationsFor(engine Engine) (fs.FS, string, error) {
	switch engine {
	case EngineSQLite:
		sub, err := fs.Sub(sqliteMigrationsFS, "migrations/sqlite")
		if err != nil {
			return nil, "", err
		}
		return sub, "sqlite3", nil
	case EnginePostgres:
		sub, err := fs.Sub(postgresMigrationsFS, "migrations/postgres")
		if err != nil {
			return nil, "", err
		}
		return sub, "postgres", nil
	default:
		return nil, "", fmt.Errorf("storage: unknown engine %q", engine)
	}
}

// runMigrations applies every pending migration for engine against db. It
// is idempotent: re-running with no pending migrations is a no-op.
func runMigrations(ctx context.Context, db *sql.DB, engine Engine, logger *slog.Logger) error {
	sub, dialect, err := migrationsFor(engine)
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(
		goose.Dialect(dialect),
		db,
		sub,
		goose.WithLogger(gooseSlogLogger{logger: logger}),
	)
	if err != nil {
		return fmt.Errorf("storage: goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("storage: migrate up: %w", err)
	}
	logger.Info("migrations applied", "engine", string(engine), "count", len(results))
	return nil
}

// gooseSlogLogger adapts *slog.Logger to goose.Logger.
type gooseSlogLogger struct {
	logger *slog.Logger
}

func (g gooseSlogLogger) Printf(format string, v ...interface{}) {
	g.logger.Info(fmt.Sprintf(format, v...))
}

func (g gooseSlogLogger) Fatalf(format string, v ...interface{}) {
	g.logger.Error(fmt.Sprintf(format, v...))
}
