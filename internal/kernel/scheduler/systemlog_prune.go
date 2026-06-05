package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/systemlogs"
)

const (
	SystemLogPruneJobName  = "system.log-prune"
	SystemLogPruneSchedule = "0 4 * * *" // daily at 04:00
)

// RegisterSystemLogPrune installs a daily job to delete system logs
// older than the configured retention period.
func RegisterSystemLogPrune(ctx context.Context, s *Scheduler, store *systemlogs.Store, retentionDays int, logger *slog.Logger) error {
	if s == nil {
		return fmt.Errorf("scheduler: RegisterSystemLogPrune: scheduler must not be nil")
	}
	if retentionDays <= 0 {
		retentionDays = 7
	}
	retention := time.Duration(retentionDays) * 24 * time.Hour

	handler := func(ctx context.Context) error {
		cutoff := time.Now().UTC().Add(-retention)
		n, err := store.Prune(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("system log prune: %w", err)
		}
		if n > 0 {
			logger.Info("system logs pruned", "deleted", n, "retention_days", retentionDays)
		}
		return nil
	}
	return s.Register(ctx, SystemLogPruneJobName, SystemLogPruneSchedule, handler, []byte(`{"builtin":true}`))
}
