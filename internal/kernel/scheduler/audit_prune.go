package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/auditlog"
)

const (
	AuditPruneJobName   = "system.audit-prune"
	AuditPruneSchedule  = "0 3 * * *"         // daily at 03:00
	AuditPruneRetention = 30 * 24 * time.Hour // 30 days
)

// RegisterAuditPrune installs the daily audit-log pruning job.
func RegisterAuditPrune(ctx context.Context, s *Scheduler, al *auditlog.Logger, logger *slog.Logger) error {
	if s == nil {
		return fmt.Errorf("scheduler: RegisterAuditPrune: scheduler must not be nil")
	}
	handler := func(ctx context.Context) error {
		n, err := al.Prune(ctx, AuditPruneRetention)
		if err != nil {
			return fmt.Errorf("audit prune: %w", err)
		}
		if n > 0 {
			logger.Info("audit log pruned", "deleted", n, "retention", AuditPruneRetention)
		}
		return nil
	}
	return s.Register(ctx, AuditPruneJobName, AuditPruneSchedule, handler, []byte(`{"builtin":true}`))
}
