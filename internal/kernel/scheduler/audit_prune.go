package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/auditlog"
)

const (
	// AuditPruneJobName is the scheduled_jobs name for audit-log retention.
	AuditPruneJobName = "system.audit-prune"
	// AuditPruneSchedule runs daily at 03:00.
	AuditPruneSchedule = "0 3 * * *" // daily at 03:00
	// AuditPruneDefaultRetention is the fallback retention in days.
	AuditPruneDefaultRetention = 30
)

// RegisterAuditPrune installs the daily audit-log pruning job.
func RegisterAuditPrune(ctx context.Context, s *Scheduler, al *auditlog.Logger, retentionDays int, logger *slog.Logger) error {
	if s == nil {
		return fmt.Errorf("scheduler: RegisterAuditPrune: scheduler must not be nil")
	}
	if retentionDays <= 0 {
		retentionDays = AuditPruneDefaultRetention
	}
	retention := time.Duration(retentionDays) * 24 * time.Hour
	handler := func(ctx context.Context) error {
		n, err := al.Prune(ctx, retention)
		if err != nil {
			return fmt.Errorf("audit prune: %w", err)
		}
		if n > 0 {
			logger.Info("audit log pruned", "deleted", n, "retention_days", retentionDays)
		}
		return nil
	}
	return s.Register(ctx, AuditPruneJobName, AuditPruneSchedule, handler, []byte(`{"builtin":true}`))
}
