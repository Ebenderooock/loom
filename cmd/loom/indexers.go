package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/loomctl/loom/internal/indexers"
	"github.com/loomctl/loom/internal/indexers/cardigann"
	"github.com/loomctl/loom/internal/indexers/newznab"
	"github.com/loomctl/loom/internal/indexers/proxies"
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
	var (
		repo        indexers.Repository
		caps        indexers.CapsCache
		proxiesRepo proxies.Repository
	)
	switch db.Engine() {
	case storage.EngineSQLite:
		repo = indexers.NewSQLiteRepository(db.DB())
		caps = indexers.NewSQLiteCapsCache(db.DB())
		proxiesRepo = proxies.NewSQLiteRepository(db.DB())
	case storage.EnginePostgres:
		repo = indexers.NewPostgresRepository(db.DB())
		caps = indexers.NewPostgresCapsCache(db.DB())
		proxiesRepo = proxies.NewPostgresRepository(db.DB())
	default:
		return nil, fmt.Errorf("indexers: unsupported storage engine %q", string(db.Engine()))
	}

	// Wire the caps cache before hydrate so factories see it.
	newznab.SetCapsCache(caps)

	// Build the proxies service first so its TransportFor is
	// installed before HydrateAll constructs any newznab clients.
	proxiesSvc, err := proxies.NewService(proxies.ServiceOptions{
		Repository:          proxiesRepo,
		Logger:              logger,
		FlareSolverrTimeout: time.Duration(cfg.Indexers.Proxies.FlareSolverrDefaultTimeoutSec) * time.Second,
		TestProbeURL:        cfg.Indexers.Proxies.TestProbeURL,
	})
	if err != nil {
		return nil, fmt.Errorf("proxies: %w", err)
	}
	indexers.SetTransportProvider(proxiesSvc)

	// Boot the Cardigann definition loader. A missing or empty
	// directory is non-fatal: the kind simply has no definitions to
	// resolve, and any indexer rows pointing at a missing definition
	// surface a clear error at hydrate time.
	cardigannDir := cfg.Indexers.Cardigann.DefinitionsDir
	if cardigannDir == "" {
		cardigannDir = filepath.Join(cfg.DataDir, "definitions", "cardigann")
	}
	cardLoader := cardigann.NewLoader(cardigannDir)
	if _, loadErrs := cardLoader.Reload(); len(loadErrs) > 0 {
		for _, lerr := range loadErrs {
			logger.Warn("cardigann definition skipped", "err", lerr)
		}
	}
	cardigann.SetLoader(cardLoader)

	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository:         repo,
		Logger:             logger,
		SearchTimeout:      time.Duration(cfg.Indexers.SearchTimeoutSec) * time.Second,
		MaxParallel:        cfg.Indexers.MaxParallel,
		HealthCheckTimeout: time.Duration(cfg.Indexers.HealthCheckTimeoutSec) * time.Second,
		RouteExtensions:    []indexers.RouteMounter{proxiesSvc.Mount},
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
