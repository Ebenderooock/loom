package scheduler

import (
	"context"
	"database/sql"
	"fmt"
)

// HousekeepingJobName is the registry key for the built-in DB
// maintenance job. Exported so tests and admin tooling can refer to it
// without stringly-typed lookups.
const HousekeepingJobName = "system.housekeeping"

// HousekeepingSchedule is the cron expression Loom registers the
// housekeeping job under by default — every six hours, on the hour.
const HousekeepingSchedule = "0 */6 * * *"

// Engine names recognised by RegisterHousekeeping. Kept as a tiny
// internal vocabulary so this package doesn't depend on the storage
// package and avoid an import cycle.
const (
	EngineSQLite   = "sqlite"
	EnginePostgres = "postgres"
)

// RegisterHousekeeping installs the system.housekeeping job on s.
// The job runs SQLite's PRAGMA optimize or Postgres's VACUUM ANALYZE
// depending on engine, proving the wiring end-to-end and keeping the
// DB tidy without operator intervention.
func RegisterHousekeeping(ctx context.Context, s *Scheduler, db *sql.DB, engine string) error {
	if s == nil {
		return fmt.Errorf("scheduler: RegisterHousekeeping: scheduler must not be nil")
	}
	if db == nil {
		return fmt.Errorf("scheduler: RegisterHousekeeping: db must not be nil")
	}
	stmt, err := housekeepingStatement(engine)
	if err != nil {
		return err
	}
	handler := func(ctx context.Context) error {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("housekeeping %s: %w", engine, err)
		}
		return nil
	}
	return s.Register(ctx, HousekeepingJobName, HousekeepingSchedule, handler, []byte(`{"builtin":true}`))
}

func housekeepingStatement(engine string) (string, error) {
	switch engine {
	case EngineSQLite:
		return "PRAGMA optimize", nil
	case EnginePostgres:
		return "VACUUM (ANALYZE)", nil
	default:
		return "", fmt.Errorf("scheduler: housekeeping: unknown engine %q", engine)
	}
}
