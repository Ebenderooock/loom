# ADR-0002: Storage — SQLite by default, Postgres optional, one schema

- Status: Accepted
- Date: 2025-05-01
- Deciders: Loom maintainers

## Context

The 90% case for *arr users is single-instance home use; SQLite is ideal
there. The remaining 10% — large libraries, HA, shared deployments,
split-mode where multiple Loom processes share state — needs Postgres.

## Decision

- **SQLite (pure-Go, `modernc.org/sqlite`) by default**, no CGO required.
- **Postgres 14+ optional**, selected via `LOOM_DATABASE_URL=postgres://…`.
- A **single schema** maintained in `internal/storage/migrations/` using
  `goose`, with engine-specific quirks isolated per migration.
- Queries authored in SQL and type-checked via `sqlc` with both engine
  configurations validated in CI.

## Consequences

### Positive
- Zero-config first-run on every platform.
- No CGO means trivial cross-compilation and small distroless images.
- Postgres path unlocks HA, backups via `pg_dump`, and shared state for
  split-mode deployments.

### Negative / trade-offs
- SQLite + Postgres dialect differences cost some migration effort
  (e.g. `INTEGER PRIMARY KEY AUTOINCREMENT` vs `BIGSERIAL`). We absorb
  this by writing migrations in two flavors when needed.
- `sqlc` requires keeping queries in sync; this is a feature, not a bug.

### Neutral
- The DB abstraction is `*sql.DB` + sqlc-generated `Querier`. We do not
  introduce an ORM.

## Alternatives considered

- **SQLite only** — excludes the HA/split-mode story.
- **Postgres only** — adds a hard dependency for new users; non-starter.
- **MySQL** — no advantage over Postgres for our workload.
- **Embedded KV (Bolt/Pebble)** — wrong shape; we have rich relational
  queries (history, queue, search).
