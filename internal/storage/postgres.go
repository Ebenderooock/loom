package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

// PostgresIdentity identifies the configured endpoint without credentials.
// It describes the connection configuration, not the server's physical host.
type PostgresIdentity struct {
	Host     string
	Port     uint16
	Database string
	User     string
}

type postgresDB struct {
	raw        *sql.DB
	logger     *slog.Logger
	identity   PostgresIdentity
	identityOK bool
}

// postgresConnectionIdentity rejects failover endpoints: consent for one
// endpoint cannot authorize a different server selected by the driver.
func postgresConnectionIdentity(c *pgx.ConnConfig) (PostgresIdentity, bool) {
	id := PostgresIdentity{Host: c.Host, Port: c.Port, Database: c.Database, User: c.User}
	if id.Host == "" || id.Port == 0 || id.Database == "" || id.User == "" {
		return PostgresIdentity{}, false
	}
	for _, fallback := range c.Fallbacks {
		if fallback.Host != id.Host || fallback.Port != id.Port {
			return PostgresIdentity{}, false
		}
	}
	return id, true
}

func openPostgres(ctx context.Context, cfg config.PostgresConfig, logger *slog.Logger) (DB, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, errors.New("postgres: storage.postgres.dsn is required when storage.engine=postgres")
	}

	// Freeze pgx's environment/service-file resolution once, so every pooled
	// connection and the acknowledgement use the same effective identity.
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("postgres: invalid connection configuration")
	}
	identity, identityOK := postgresConnectionIdentity(connConfig)
	db := stdlib.OpenDB(*connConfig)
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
	return &postgresDB{raw: db, logger: logger, identity: identity, identityOK: identityOK}, nil
}

func (p *postgresDB) DB() *sql.DB    { return p.raw }
func (p *postgresDB) Engine() Engine { return EnginePostgres }

func (p *postgresDB) Migrate(ctx context.Context) error {
	return runMigrations(ctx, p.raw, EnginePostgres, p.logger)
}

func (p *postgresDB) Ping(ctx context.Context) error { return p.raw.PingContext(ctx) }
func (p *postgresDB) Close() error                   { return p.raw.Close() }
func (p *postgresDB) PostgresIdentity() (PostgresIdentity, bool) {
	return p.identity, p.identityOK
}
