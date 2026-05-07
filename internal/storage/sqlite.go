package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

// sqliteDriverName is the database/sql driver name registered by the
// modernc.org/sqlite import.
const sqliteDriverName = "sqlite"

type sqliteDB struct {
	raw    *sql.DB
	path   string
	logger *slog.Logger
}

func openSQLite(ctx context.Context, cfg config.SQLiteConfig, logger *slog.Logger) (DB, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = "/data/loom.db"
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file::memory:") {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("sqlite: create parent dir: %w", err)
			}
		}
	}

	dsn := buildSQLiteDSN(path)
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// Single writer keeps WAL contention out of the picture for v1.
	db.SetMaxOpenConns(1)

	pingCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}

	logger.Info("storage opened", "engine", "sqlite", "path", path)
	return &sqliteDB{raw: db, path: path, logger: logger}, nil
}

// buildSQLiteDSN constructs a DSN that enables WAL, NORMAL sync, foreign
// keys, a busy timeout, and tells modernc to parse times itself. Pragmas
// are passed via repeated _pragma query keys.
func buildSQLiteDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Set("_time_format", "sqlite")
	return path + "?" + q.Encode()
}

func (s *sqliteDB) DB() *sql.DB    { return s.raw }
func (s *sqliteDB) Engine() Engine { return EngineSQLite }

func (s *sqliteDB) Migrate(ctx context.Context) error {
	return runMigrations(ctx, s.raw, EngineSQLite, s.logger)
}

func (s *sqliteDB) Ping(ctx context.Context) error { return s.raw.PingContext(ctx) }
func (s *sqliteDB) Close() error                   { return s.raw.Close() }
