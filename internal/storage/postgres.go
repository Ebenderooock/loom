package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

const pgDriverName = "pgx"

type postgresDB struct {
	raw    *sql.DB
	logger *slog.Logger
}

func openPostgres(ctx context.Context, cfg config.PostgresConfig, logger *slog.Logger) (DB, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, errors.New("postgres: storage.postgres.dsn is required when storage.engine=postgres")
	}

	db, err := sql.Open(pgDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	logger.Info("storage opened", "engine", "postgres")
	return &postgresDB{raw: db, logger: logger}, nil
}

func (p *postgresDB) DB() *sql.DB    { return p.raw }
func (p *postgresDB) Engine() Engine { return EnginePostgres }

func (p *postgresDB) Migrate(ctx context.Context) error {
	return runMigrations(ctx, p.raw, EnginePostgres, p.logger)
}

func (p *postgresDB) Ping(ctx context.Context) error { return p.raw.PingContext(ctx) }
func (p *postgresDB) Close() error                   { return p.raw.Close() }
