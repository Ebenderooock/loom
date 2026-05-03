package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomctl/loom/internal/downloads"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/scheduler"
	"github.com/loomctl/loom/internal/storage"
)

// buildDownloadService constructs the downloads.Service backed by the
// storage engine in cfg, hydrates the registry from any persisted
// rows, and returns the wired service. Hydrate failures are logged
// but non-fatal — a single broken row never blocks startup.
func buildDownloadService(ctx context.Context, cfg *config.Config, db storage.DB, logger *slog.Logger) (*downloads.Service, error) {
	var repo downloads.Repository
	switch db.Engine() {
	case storage.EngineSQLite:
		repo = downloads.NewSQLiteRepository(db.DB())
	case storage.EnginePostgres:
		repo = downloads.NewPostgresRepository(db.DB())
	default:
		return nil, fmt.Errorf("downloads: unsupported storage engine %q", string(db.Engine()))
	}

	svc, err := downloads.NewService(downloads.ServiceOptions{
		Repository:       repo,
		Logger:           logger,
		OperationTimeout: time.Duration(cfg.Downloads.OperationTimeoutSec) * time.Second,
		MaxParallel:      cfg.Downloads.MaxParallel,
		HealthTimeout:    time.Duration(cfg.Downloads.HealthCheckTimeoutSec) * time.Second,
	})
	if err != nil {
		return nil, err
	}

	if err := svc.HydrateAll(ctx); err != nil {
		logger.Warn("download client hydrate failed", "err", err)
	}
	return svc, nil
}

// registerDownloadHealthJob hooks the periodic download-client health
// sweep into the persistent scheduler.
func registerDownloadHealthJob(ctx context.Context, sched *scheduler.Scheduler, cfg *config.Config, svc *downloads.Service) error {
	checker := downloads.NewHealthChecker(svc,
		cfg.Downloads.MaxParallel,
		time.Duration(cfg.Downloads.HealthCheckTimeoutSec)*time.Second,
	)
	payload, _ := json.Marshal(map[string]any{
		"schedule": cfg.Downloads.HealthCheckSchedule,
	})
	return sched.Register(ctx, downloads.HealthCheckJobName,
		cfg.Downloads.HealthCheckSchedule,
		func(ctx context.Context) error { return checker.Run(ctx) },
		payload,
	)
}
