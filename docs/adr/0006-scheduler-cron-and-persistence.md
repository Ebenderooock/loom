# ADR-0006: Scheduler — robfig/cron/v3 with database persistence

- Status: Accepted
- Date: 2025-05-01
- Deciders: Loom maintainers

## Context

Loom needs to run a steady stream of recurring work: RSS feed sync,
indexer health checks, library refreshes, queue reconciliation, log
rotation, database housekeeping. The Phase-1a skeleton shipped a tiny
ticker-based abstraction with no persistence — fine for the bootstrap,
not fine for production.

A scheduler we ship to operators must:

1. Use cron-style expressions. Every operator coming from Radarr,
   Sonarr or Prowlarr already thinks in cron.
2. Survive restarts: `last_run_at` and `next_run_at` must persist so a
   restart doesn't double-fire or skip a window.
3. Refuse to run two copies of the same job concurrently.
4. Shut down cleanly under `SIGTERM` with a bounded grace.
5. Stay tiny — single binary, one new dependency at most.

## Decision

- **Library: `github.com/robfig/cron/v3`** pinned at `v3.0.1`.
- **Persistence: the existing `scheduled_jobs` table** in the storage
  layer, extended (migration `0005_scheduled_jobs_status.sql`) to add
  `enabled`, `last_status`, and `last_error` columns. sqlc-generated
  queries live alongside every other DAL in the project.
- **Concurrency: per-job `sync.Mutex` (with `TryLock`)** in the Go
  process. Skipped runs are recorded as `last_status="skipped"`. A
  database advisory-lock variant is the planned upgrade for split-mode
  (multi-process) deployments in Phase 11.
- **Shutdown: two-phase.** New ticks stop immediately on context
  cancel; in-flight handlers see the parent context for up to
  `scheduler.shutdown_grace` (default 30s); on timeout the per-job
  context is canceled and the loop returns.
- **Built-in job**: `system.housekeeping` runs every six hours and
  exercises `PRAGMA optimize` (SQLite) or `VACUUM ANALYZE`
  (Postgres). It proves the wiring without requiring any module code.

## Consequences

### Positive

- Cron expressions out of the box, including `@hourly`,
  `@every 5m`, etc. Operators feel at home.
- Run history survives restarts; metrics and an eventual
  `GET /api/v1/system/jobs` endpoint can simply read the table.
- Idempotent registration. Modules call `Register` at every startup;
  the scheduler upserts the row and silently refreshes the schedule
  without losing run history.
- One direct dependency added (`robfig/cron/v3`); no transitive
  surprises.

### Negative / trade-offs

- `robfig/cron/v3` was last released in 2019. It is small, stable, and
  widely vendored, but unmaintained in the literal sense — we accept
  the risk because the surface area we use (Parser, Schedule.Next) is
  trivial and replaceable.
- Per-process locking is insufficient for multi-process deployments.
  We track this as a known limitation; Phase 11 split-mode will swap
  in a DB advisory lock.
- 5-field cron is not as expressive as quartz / k8s CronJob (no
  per-second, no time-ranges in DOW). We expose the optional 6-field
  form for the rare case it's needed.

### Neutral

- Schedule strings are stored verbatim in the DB; tooling that wants
  to render "next 5 fires" can re-parse on demand.
- Time zone is configurable (`scheduler.timezone`, default `Local`).

## Alternatives considered

- **`github.com/go-co-op/gocron` (v2).** Bigger surface, a fluent
  builder API, native distributed-lock support. Rejected because the
  fluent API is overkill for our register-by-name pattern, and we want
  the persistence model to be ours (not the library's), so we can
  shape `scheduled_jobs` to fit the DAL we already have.
- **Plain `time.Ticker` per job.** What we shipped in 1a. Doesn't do
  cron, can't represent "every day at 03:00" without bespoke code, and
  has zero persistence. Insufficient.
- **External (systemd timers, Kubernetes CronJob).** Operationally
  attractive but couples Loom to a specific deploy substrate; the
  goal is "single static binary that just works" and that excludes
  external schedulers from the default path. Operators can still
  disable the in-process scheduler and run jobs externally if they
  want; a CLI subcommand for this is planned.
- **Quartz-like custom scheduler.** Fun, large, and outside the
  budget of Phase 1d.

## References

- ADR-0002 — Storage (SQLite default, Postgres optional).
- [docs/scheduler.md](../scheduler.md) — operator and developer guide.
- Migration: `internal/storage/migrations/{sqlite,postgres}/0005_scheduled_jobs_status.sql`.
