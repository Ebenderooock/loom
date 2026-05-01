# Storage

Loom uses a single schema across two database engines. SQLite is the
default — zero-config, single-file, pure-Go driver — and Postgres is a
drop-in replacement for HA / shared / split-mode deployments.

See ADR-0002 for the rationale.

## Engine selection

| Engine | When to pick it | DSN |
|---|---|---|
| **SQLite** (default) | Single-instance home use. | `storage.sqlite.path: /config/loom.db` |
| **Postgres 14+** | HA, shared deployments, split-mode, large libraries. | `storage.postgres.dsn: postgres://user:pass@host:5432/loom?sslmode=disable` |

The selector is `storage.engine` (`sqlite` | `postgres`). The legacy
`LOOM_DATABASE_URL` environment variable is also honoured: a value
starting with `postgres://` switches to Postgres.

### SQLite specifics

- Driver: `modernc.org/sqlite` (pure Go — no CGO required).
- Pragmas applied on open: WAL journal, foreign keys on, busy-timeout 5 s.
- The on-disk path is created with `0o755` if the parent directory does
  not exist.

### Postgres specifics

- Driver: `jackc/pgx` via `database/sql`.
- Connection pool defaults follow `database/sql` (tunable knobs land
  alongside HA work in Phase 11).
- TLS is whatever the DSN says — `sslmode=require` for production.

## Schema migrations

Migrations live under `internal/storage/migrations/<engine>/` and are
embedded into the binary via `//go:embed`. The runner is
[goose](https://github.com/pressly/goose) v3.

- Migrations are SQL files named `NNN_summary.sql` containing
  `-- +goose Up` / `-- +goose Down` blocks.
- Each engine has its own subdirectory because of dialect differences
  (e.g. `BLOB` vs `BYTEA`, `INTEGER PRIMARY KEY AUTOINCREMENT` vs
  `BIGSERIAL`). The set is otherwise kept in lock-step.
- The current schema version is tracked in goose's `goose_db_version`
  table plus a Loom-specific `schema_meta` row for human-readable
  metadata.
- `loom serve` and `loom migrate` both apply pending migrations on
  startup.

### Adding a migration

1. Add `internal/storage/migrations/sqlite/NNN_<summary>.sql` with both
   `Up` and `Down` blocks.
2. Mirror the change in `internal/storage/migrations/postgres/NNN_<summary>.sql`,
   adapting types where necessary.
3. Add the corresponding query SQL under
   `internal/storage/queries/<engine>/<table>.sql` and run `sqlc generate`.
4. Run `make test` — the migration is exercised against both engines.
5. Update [docs/configuration.md](configuration.md) if the change exposes
   a new config key, and add a CHANGELOG entry.

## Typed queries with sqlc

`sqlc.yaml` at the repo root declares two configurations — one per
engine. Generated Go lives in `internal/storage/db/sqlite/` and
`internal/storage/db/postgres/`. The shared `Querier` interface is
fulfilled by both packages so callers depend on the interface, not the
engine.

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
```

The generated files are checked in so contributors without `sqlc`
installed can still build.

## Backup & restore

A first-class `loom backup` / `loom restore` CLI lands in Phase 11.
Until then:

- **SQLite.** Stop Loom, copy `<data_dir>/loom.db` (and `*.db-wal` /
  `*.db-shm` if present) to the backup destination. Or use SQLite's
  `.backup` command via `sqlite3` against a running Loom (WAL mode makes
  this safe).
- **Postgres.** Standard `pg_dump` / `pg_restore`. The schema is
  Postgres-vanilla; no extensions required.

## Storage testing

The storage layer is exercised by the suite under `internal/storage/`.
SQLite tests run inline; Postgres tests use a containerised instance
when `LOOM_TEST_PG_DSN` is set in the CI environment.
