package workflows

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler runs periodic health checks on active workflows.
type Scheduler struct {
	engine *Engine
	logger *slog.Logger
}

// NewScheduler creates a workflow scheduler.
func NewScheduler(engine *Engine, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		engine: engine,
		logger: logger.With("component", "workflow-scheduler"),
	}
}

// RunLoop starts the scheduler loop. Blocks until ctx is cancelled.
func (s *Scheduler) RunLoop(ctx context.Context) {
	ticker := time.NewTicker(SchedulerTick)
	defer ticker.Stop()

	// Also run a prune cycle every hour
	pruneTicker := time.NewTicker(1 * time.Hour)
	defer pruneTicker.Stop()

	s.logger.Info("workflow scheduler started", "tick", SchedulerTick)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("workflow scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		case <-pruneTicker.C:
			s.prune(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	if err := s.engine.HandleStaleWorkflows(ctx); err != nil {
		s.logger.Error("scheduler: stale check failed", "error", err)
	}
}

func (s *Scheduler) prune(ctx context.Context) {
	pruned, err := s.engine.store.PruneCompleted(ctx, CompletedTTL)
	if err != nil {
		s.logger.Error("scheduler: prune failed", "error", err)
		return
	}
	if pruned > 0 {
		s.logger.Info("scheduler: pruned completed workflows", "count", pruned)
	}
}

// Reconcile checks active workflows against actual download client state.
// Called by the download monitor to sync workflow states with reality.
func (s *Scheduler) Reconcile(ctx context.Context, activeDownloads map[string]string) {
	active, err := s.engine.store.ListActive(ctx)
	if err != nil {
		s.logger.Error("reconcile: list active failed", "error", err)
		return
	}

	for _, wf := range active {
		if wf.DownloadClientID == "" || wf.DownloadID == "" {
			continue // not yet grabbed
		}

		key := wf.DownloadClientID + ":" + wf.DownloadID
		dlState, exists := activeDownloads[key]

		switch {
		case !exists && wf.State == StateDownloading:
			// Download disappeared — might have completed or been removed
			s.logger.Warn("reconcile: download not found for workflow",
				"id", wf.ID, "client", wf.DownloadClientID, "download", wf.DownloadID)
		case exists && dlState == "downloading" && wf.State == StateGrabbed:
			// Download confirmed active, upgrade from grabbed → downloading
			_ = s.engine.MarkDownloading(ctx, wf.ID)
		}
	}
}
