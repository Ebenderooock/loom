package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	// PlayHistoryPruneJobName is the scheduled_jobs name for analytics retention.
	PlayHistoryPruneJobName = "system.play-history-prune"
	// PlayHistoryPruneSchedule runs daily at 05:00.
	PlayHistoryPruneSchedule = "0 5 * * *" // daily at 05:00
	// PlayHistoryPruneDefaultRetention is the fallback retention in days.
	PlayHistoryPruneDefaultRetention = 90
)

type playHistoryPruner interface {
	PruneHistory(ctx context.Context, olderThan time.Time) (int64, error)
}

// RegisterPlayHistoryPrune installs a daily job to delete analytics
// play-history rows older than the configured retention period.
func RegisterPlayHistoryPrune(
	ctx context.Context,
	s *Scheduler,
	pruner playHistoryPruner,
	retentionDays int,
	logger *slog.Logger,
) error {
	if s == nil {
		return fmt.Errorf("scheduler: RegisterPlayHistoryPrune: scheduler must not be nil")
	}
	if pruner == nil {
		return fmt.Errorf("scheduler: RegisterPlayHistoryPrune: pruner must not be nil")
	}
	if retentionDays <= 0 {
		retentionDays = PlayHistoryPruneDefaultRetention
	}
	retention := time.Duration(retentionDays) * 24 * time.Hour

	handler := func(ctx context.Context) error {
		cutoff := time.Now().UTC().Add(-retention)
		n, err := pruner.PruneHistory(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("play history prune: %w", err)
		}
		if n > 0 {
			logger.Info("play history pruned", "deleted", n, "retention_days", retentionDays)
		}
		return nil
	}
	return s.Register(ctx, PlayHistoryPruneJobName, PlayHistoryPruneSchedule, handler, []byte(`{"builtin":true}`))
}
