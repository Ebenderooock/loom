package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomctl/loom/internal/indexers"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/scheduler"
	"github.com/loomctl/loom/internal/storage"
)

// buildIndexerService constructs the indexer Service backed by the
// storage engine in cfg, hydrates the registry from any rows already
// persisted, and returns the wired service. Errors at hydrate time
// are not fatal: the Service logs them and the affected indexers are
// skipped, so a single broken row never blocks startup.
func buildIndexerService(ctx context.Context, cfg *config.Config, db storage.DB, logger *slog.Logger) (*indexers.Service, error) {
	var repo indexers.Repository
	switch db.Engine() {
	case storage.EngineSQLite:
		repo = indexers.NewSQLiteRepository(db.DB())
	case storage.EnginePostgres:
		repo = indexers.NewPostgresRepository(db.DB())
	default:
		return nil, fmt.Errorf("indexers: unsupported storage engine %q", string(db.Engine()))
	}

	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository:         repo,
		Logger:             logger,
		SearchTimeout:      time.Duration(cfg.Indexers.SearchTimeoutSec) * time.Second,
		MaxParallel:        cfg.Indexers.MaxParallel,
		HealthCheckTimeout: time.Duration(cfg.Indexers.HealthCheckTimeoutSec) * time.Second,
	})
	if err != nil {
		return nil, err
	}

	if err := svc.HydrateAll(ctx); err != nil {
		logger.Warn("indexer hydrate failed", "err", err)
	}
	return svc, nil
}

// registerIndexerHealthJob hooks the periodic indexer health sweep
// into the persistent scheduler. The schedule comes from
// cfg.Indexers.HealthCheckSchedule and is interpreted in the
// scheduler's configured timezone.
func registerIndexerHealthJob(ctx context.Context, sched *scheduler.Scheduler, cfg *config.Config, svc *indexers.Service) error {
	checker := indexers.NewHealthChecker(svc,
		cfg.Indexers.MaxParallel,
		time.Duration(cfg.Indexers.HealthCheckTimeoutSec)*time.Second,
	)
	payload, _ := json.Marshal(map[string]any{
		"schedule": cfg.Indexers.HealthCheckSchedule,
	})
	return sched.Register(ctx, indexers.HealthCheckJobName,
		cfg.Indexers.HealthCheckSchedule,
		func(ctx context.Context) error { return checker.Run(ctx) },
		payload,
	)
}
