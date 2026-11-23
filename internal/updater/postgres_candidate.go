//go:build loom_postgres_candidate

package updater

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
)

// postgresIdentity contains no credentials. It must be derived in the service
// process from its effective DSN and environment, never from the HTTP request.
type postgresIdentity struct {
	Host     string `json:"host"`
	Port     uint16 `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
}

func effectivePostgresIdentity(dsn string) (postgresIdentity, error) {
	if strings.TrimSpace(dsn) == "" {
		return postgresIdentity{}, errors.New("PostgreSQL DSN is missing")
	}
	c, err := pgx.ParseConfig(dsn)
	if err != nil {
		// Parser errors can contain the DSN and therefore must not be surfaced.
		return postgresIdentity{}, errors.New("invalid effective PostgreSQL configuration")
	}
	id := postgresIdentity{Host: c.Host, Port: c.Port, Database: c.Database, User: c.User}
	if id.Host == "" || id.Port == 0 || id.Database == "" || id.User == "" {
		return postgresIdentity{}, errors.New("incomplete effective PostgreSQL identity")
	}
	for _, fallback := range c.Fallbacks {
		// TLS/non-TLS fallbacks for the same endpoint are fine. Failover to
		// another host is not covered by an acknowledgement for this one.
		if fallback.Host != id.Host || fallback.Port != id.Port {
			return postgresIdentity{}, errors.New("multi-host PostgreSQL configurations are not supported")
		}
	}
	return id, nil
}

func postgresAcknowledged(effective postgresIdentity, ack *postgresIdentity) bool {
	return ack != nil && effective.Host != "" && effective.Port != 0 &&
		effective.Database != "" && effective.User != "" && *ack == effective
}

type postgresRow interface{ Scan(...any) error }

// An implementation must use a connection to the postgres maintenance database.
type postgresQueryer interface {
	QueryRowContext(context.Context, string, ...any) postgresRow
}

// postgresTools must select only trusted fixed-path binaries, all with the
// server's major version. It must bind execution to the checked database;
// credentials must be supplied out of argv.
type postgresTools interface {
	Validate(context.Context, int, string) error
	Run(context.Context, string, []string, io.Reader, io.Writer) error
}

// This is stronger than ALLOW_CONNECTIONS=false: the fence must exclude every
// external client while allowing the updater's own pg_dump/pg_restore sessions.
type postgresFence interface {
	VerifyUpdaterOnly(context.Context) error
}

// postgresCandidate is intentionally not wired to the native updater. TODO:
// implement a durable, crash-recoverable external updater-only connection
// fence and credential-dropped tool runner before considering production use.
// ALLOW_CONNECTIONS=false alone blocks the tools too; toggling it is unsafe.
// TODO: independently bind postgresDedicatedAck at the privileged-helper
// boundary and read only verified unit configuration. The settings record is
// database-local and can revive after switching away and back to a different
// database; resolve that continuity gap before enabling installation.
// TODO: persist fence intent before changing connection access, restore it in
// boot recovery, and implement idempotent drop/recreate/restore with the
// original owner, encoding and locale. No rollback operation exists yet.
type postgresCandidate struct {
	db    postgresQueryer
	tools postgresTools
	fence postgresFence
}

func (p postgresCandidate) preflight(ctx context.Context, database string) error {
	if p.db == nil || p.tools == nil || p.fence == nil || database == "" {
		return errors.New("PostgreSQL updater safety prerequisites are unavailable")
	}
	var version int
	var readStats, createDB bool
	if err := p.db.QueryRowContext(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		return errors.New("cannot determine PostgreSQL server version")
	}
	if version < 130000 {
		return errors.New("PostgreSQL 13 or later is required")
	}
	if err := p.db.QueryRowContext(ctx, `SELECT r.rolsuper OR pg_has_role(current_user, 'pg_read_all_stats', 'USAGE'), r.rolsuper OR r.rolcreatedb FROM pg_catalog.pg_roles AS r WHERE r.rolname = current_user`).Scan(&readStats, &createDB); err != nil || !readStats || !createDB {
		return errors.New("PostgreSQL updater requires session visibility and database creation privileges")
	}
	if err := p.tools.Validate(ctx, version/10000, database); err != nil {
		return errors.New("compatible trusted PostgreSQL tools are unavailable")
	}
	if err := p.fence.VerifyUpdaterOnly(ctx); err != nil {
		return errors.New("PostgreSQL updater-only connection fence is unavailable")
	}
	var count int64
	if err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM pg_catalog.pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, database).Scan(&count); err != nil || count != 0 {
		return errors.New("cannot establish that PostgreSQL has no other database client sessions")
	}
	return nil
}

// snapshot runs a full custom-format dump and checks its table of contents
// before a caller could publish a transaction journal. Listing does not prove
// that every archive entry can be restored. archive must be a new root-private
// file.
func (p postgresCandidate) snapshot(ctx context.Context, database string, archive interface {
	io.ReadWriteSeeker
	Sync() error
}) error {
	if archive == nil {
		return errors.New("PostgreSQL snapshot destination is unavailable")
	}
	if err := p.preflight(ctx, database); err != nil {
		return err
	}
	if err := p.tools.Run(ctx, "pg_dump", []string{"--format=custom"}, nil, archive); err != nil {
		return errors.New("PostgreSQL snapshot failed")
	}
	if err := archive.Sync(); err != nil {
		return errors.New("PostgreSQL snapshot could not be made durable")
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := p.tools.Run(ctx, "pg_restore", []string{"--list"}, archive, io.Discard); err != nil {
		return errors.New("PostgreSQL snapshot validation failed")
	}
	return nil
}
