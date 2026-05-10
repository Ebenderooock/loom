package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/cardigann"
	"github.com/ebenderooock/loom/internal/indexers/newznab"
	"github.com/ebenderooock/loom/internal/indexers/proxies"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/scheduler"
	"github.com/ebenderooock/loom/internal/storage"
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
	indexers.SetCloudFlareFallback(proxiesSvc)

	// Boot the Cardigann definition loader. A missing or empty
	// directory is non-fatal: the kind simply has no definitions to
	// resolve, and any indexer rows pointing at a missing definition
	// surface a clear error at hydrate time.
	cardigannDir := cfg.Indexers.Cardigann.DefinitionsDir
	if cardigannDir == "" {
		cardigannDir = filepath.Join(cfg.DataDir, "definitions", "cardigann")
	}
	cardLoader := cardigann.NewLoader(cardigannDir)
	diskDefs, loadErrs := cardLoader.Reload()
	if len(loadErrs) > 0 {
		for _, lerr := range loadErrs {
			logger.Warn("cardigann definition skipped", "err", lerr)
		}
	}
	embDefs, embErrs := cardLoader.LoadEmbedded(cardigann.BundledFS())
	if len(embErrs) > 0 {
		for _, lerr := range embErrs {
			logger.Warn("bundled cardigann definition skipped", "err", lerr)
		}
	}
	logger.Info("cardigann definitions loaded", "disk", len(diskDefs), "bundled", len(embDefs))
	cardigann.SetLoader(cardLoader)

	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository:         repo,
		Logger:             logger,
		SearchTimeout:      time.Duration(cfg.Indexers.SearchTimeoutSec) * time.Second,
		MaxParallel:        cfg.Indexers.MaxParallel,
		HealthCheckTimeout: time.Duration(cfg.Indexers.HealthCheckTimeoutSec) * time.Second,
		RouteExtensions:    []indexers.RouteMounter{proxiesSvc.Mount},
		DefinitionLister:   &cardigann.LoaderDefinitionLister{Loader: cardLoader},
		QueryLog:           indexers.NewQueryLog(db.DB()),
		AuditLog:           auditlog.New(db.DB(), logger),
	})
	if err != nil {
		return nil, err
	}

	if err := svc.HydrateAll(ctx); err != nil {
		logger.Warn("indexer hydrate failed", "err", err)
	}
	// Wire the rate-limit provider after the service exists so the
	// throttle transport can resolve per-indexer overrides.
	indexers.SetRateLimitProvider(svc)
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
