# Scheduler

Loom runs every recurring task — RSS sync, indexer health checks,
library refreshes, database housekeeping — through a single persistent
scheduler. Schedules are cron expressions, run history is stored in
the database, and registration is idempotent so each module re-declares
its jobs at startup without fear of duplication.

This page is the operator and developer reference. The package godoc
(`go doc github.com/ebenderooock/loom/internal/kernel/scheduler`) is the
source of truth for API details.

## TL;DR

- Jobs are registered in Go code at startup; the scheduler upserts a
  row in `scheduled_jobs` and starts firing them on schedule.
- Runs are recorded in the same row: `last_run_at`, `last_status`
  (`success`, `failed`, or `skipped`), and `last_error`.
- At most one instance of a given job runs at a time.
- On `SIGTERM`, in-flight jobs get up to `scheduler.shutdown_grace`
  seconds to finish before the process exits.

## Cron syntax

Loom uses standard 5-field cron with `robfig/cron/v3` semantics:

```text
┌── minute (0-59)
│ ┌── hour (0-23)
│ │ ┌── day of month (1-31)
│ │ │ ┌── month (1-12 or JAN-DEC)
│ │ │ │ ┌── day of week (0-6 or SUN-SAT)
│ │ │ │ │
* * * * *
```

### Cheat sheet

| Expression       | Meaning                              |
| ---------------- | ------------------------------------ |
| `* * * * *`      | every minute                         |
| `*/5 * * * *`    | every 5 minutes                      |
| `0 * * * *`      | every hour, on the hour              |
| `0 */6 * * *`    | every 6 hours (housekeeping default) |
| `0 3 * * *`      | every day at 03:00                   |
| `0 3 * * MON`    | every Monday at 03:00                |
| `@hourly`        | macro: `0 * * * *`                   |
| `@daily`         | macro: `0 0 * * *`                   |
| `@weekly`        | macro: `0 0 * * 0`                   |
| `@every 30s`     | every 30 seconds (since registered)  |
| `@every 1h30m`   | every 90 minutes                     |

The optional 6-field form (with seconds first) is available when the
scheduler is configured with `with_seconds: true`. Most production
deployments stay on the 5-field form.

## Registering a job from code

```go
import (
    "context"
    "log/slog"
    "time"

    "github.com/ebenderooock/loom/internal/kernel/scheduler"
)

func registerRSSSync(ctx context.Context, sched *scheduler.Scheduler, logger *slog.Logger) error {
    return sched.Register(
        ctx,
        "indexers.rss_sync",          // unique name; becomes the row key
        "@every 15m",                 // cron expression
        func(ctx context.Context) error {
            // do the work; honour ctx for graceful shutdown
            return nil
        },
        []byte(`{"source":"rss"}`),   // optional JSON payload, stored as-is
    )
}
```

Calling `Register` again with the same name updates the schedule and
handler, leaving `last_run_at`, `last_status`, and `last_error`
untouched. This is the expected pattern: each module declares its jobs
on every boot.

### Worked example: a module-level job

`internal/indexers/init.go` (illustrative — replace with your module):

```go
func Wire(ctx context.Context, sched *scheduler.Scheduler, deps Deps) error {
    if err := sched.Register(ctx, "indexers.health", "@every 5m", deps.Health, nil); err != nil {
        return fmt.Errorf("register indexers.health: %w", err)
    }
    if err := sched.Register(ctx, "indexers.rss", "@every 15m", deps.RSS, nil); err != nil {
        return fmt.Errorf("register indexers.rss: %w", err)
    }
    return nil
}
```

After the next restart, both jobs are visible in `scheduled_jobs` and
fire on their schedules.

## Inspecting jobs

The job table is queryable directly:

```sql
SELECT name, schedule, last_run_at, last_status, last_error, next_run_at
FROM scheduled_jobs ORDER BY name;
```

A `GET /api/v1/system/jobs` HTTP endpoint and a `loom jobs` CLI
subcommand are planned; until they land, SQL is the supported
inspection path.

## Failure semantics

- **Handler returns an error.** `last_status` becomes `failed`,
  `last_error` carries the `Error()` string, and the next tick fires
  on schedule. Errors do not crash the loop.
- **Handler panics.** Recovered, recorded as `failed` with
  `panic: <value>` in `last_error`.
- **Handler still running when the next tick fires.** The new tick is
  recorded as `skipped` (no row in `last_status`, but `next_run_at` is
  bumped forward). At-most-one-execution per job is enforced via a
  per-job mutex inside the process. For multi-process deployments
  (Phase 11 split mode) a DB-row advisory lock is the planned
  upgrade path.

## Timezone

Cron expressions are interpreted in `scheduler.timezone`. The default,
`Local`, follows the host's `TZ`. Set an explicit IANA name (e.g.
`Europe/Stockholm`) when you want schedules to ignore DST drift on the
host. Changing the timezone takes effect on the next restart.

## Graceful shutdown

When the process receives `SIGTERM`:

1. The dispatch loop stops firing new ticks immediately.
2. In-flight handlers continue to run, with the parent context still
   alive, for up to `scheduler.shutdown_grace` seconds (default 30s).
3. After the grace expires the per-job context is canceled; handlers
   that respect cancellation unwind cleanly. Handlers that ignore
   cancellation are abandoned (the goroutine leaks for the brief
   remainder of process lifetime, which is fine).

If you control the handler, always select on `ctx.Done()` inside any
loop or long sleep — that's how the grace becomes useful instead of a
guaranteed wait.

## Built-in jobs

| Name                  | Schedule       | What it does                                         |
| --------------------- | -------------- | ---------------------------------------------------- |
| `system.housekeeping` | `0 */6 * * *`  | Runs `PRAGMA optimize` (SQLite) or `VACUUM ANALYZE` (Postgres). |

The housekeeping job exists primarily to prove the scheduler wiring
end-to-end; you can disable it by setting `enabled = 0` on its row in
`scheduled_jobs`.

## Troubleshooting

| Symptom                                        | Likely cause / fix |
| ---------------------------------------------- | ------------------ |
| Job never fires                                | Check `scheduler.enabled` is true and that the row in `scheduled_jobs` has `enabled = 1`. |
| `last_status` stuck on `failed`                | Read `last_error`; the handler is returning an error every tick. |
| `last_status` stuck on `skipped`               | Previous run is still in flight — handler is slower than the schedule. Increase the interval or shorten the work. |
| `next_run_at` is in the past after a restart   | Expected; the next dispatch tick (within 10s) will fire it. |
| Schedule changes in code don't take effect     | Confirm the binary actually restarted; `Register` updates the schedule, but only on the next process start. |

## Configuration

See the `scheduler.*` keys in [configuration.md](configuration.md).

## Related

- ADR-0006 — Scheduler: cron + persistence ([adr/0006-scheduler-cron-and-persistence.md](adr/0006-scheduler-cron-and-persistence.md))
- Package godoc: `internal/kernel/scheduler`
