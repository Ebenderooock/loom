package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/scheduler"
	"github.com/loomctl/loom/internal/storage"
)

// buildScheduler constructs the persistent scheduler, registers
// built-in jobs (currently just system.housekeeping), and returns the
// fully wired *scheduler.Scheduler ready for Start.
func buildScheduler(ctx context.Context, cfg *config.Config, db storage.DB, logger *slog.Logger) (*scheduler.Scheduler, error) {
	loc, err := loadSchedulerTimezone(cfg.Scheduler.Timezone)
	if err != nil {
		return nil, err
	}

	sc := scheduler.Config{
		Enabled:       cfg.Scheduler.Enabled,
		Location:      loc,
		ShutdownGrace: time.Duration(cfg.Scheduler.ShutdownGrace) * time.Second,
	}
	store := storage.NewSchedulerStore(db)
	s, err := scheduler.New(sc, store, logger, scheduler.SystemClock{})
	if err != nil {
		return nil, fmt.Errorf("scheduler.New: %w", err)
	}

	if err := scheduler.RegisterHousekeeping(ctx, s, db.DB(), string(db.Engine())); err != nil {
		return nil, fmt.Errorf("register housekeeping: %w", err)
	}
	logger.Info("scheduler ready",
		"timezone", loc.String(),
		"shutdown_grace", sc.ShutdownGrace,
		"jobs", s.JobNames(),
	)
	return s, nil
}

// loadSchedulerTimezone resolves "Local" / "" to time.Local and
// otherwise defers to time.LoadLocation. Wrapped so the error message
// mentions the config key the user actually set.
func loadSchedulerTimezone(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("scheduler.timezone %q: %w", name, err)
	}
	return loc, nil
}
