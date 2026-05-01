// Package scheduler runs Loom's recurring work — RSS sync, indexer
// health checks, library refreshes, housekeeping — on a persistent,
// cron-driven loop.
//
// # When to use
//
// Reach for this package any time you need a task to run on a schedule
// that survives process restarts: the row in scheduled_jobs is the
// source of truth for last_run_at / next_run_at, and registration is
// idempotent so calling Register at every startup is the expected
// pattern.
//
// # When not to use
//
// One-shot work triggered by user action (e.g. "search for this movie
// now") belongs in the relevant module's own goroutine pool, not here.
// The scheduler is for unattended, repeating jobs. For sub-second
// cadence work use a plain time.Ticker — robfig/cron's resolution is
// one second.
//
// # Cron syntax
//
// Standard 5-field cron (minute hour dom month dow). The optional
// seconds field is supported when the scheduler is constructed with
// WithSeconds. Predefined macros are also accepted: @hourly, @daily,
// @weekly, @monthly, @yearly, and @every <duration>.
//
// # Concurrency
//
// At most one instance of a given job runs at a time per process. If a
// tick fires while the previous run is still going the new tick is
// skipped and recorded as last_status="skipped". Failures are recorded
// as last_status="failed" with last_error populated; they never crash
// the loop.
//
// # Shutdown
//
// On context cancel new ticks stop firing immediately and in-flight
// jobs are given Config.ShutdownGrace (default 30s) to finish before
// being abandoned. Stop blocks until the deadline elapses or all jobs
// return.
package scheduler
