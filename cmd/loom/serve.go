package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/loomctl/loom/internal/appconfig"
	"github.com/loomctl/loom/internal/downloads"
	"github.com/loomctl/loom/internal/imports"
	"github.com/loomctl/loom/internal/indexers/newznabserver"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/logging"
	"github.com/loomctl/loom/internal/kernel/telemetry"
	"github.com/loomctl/loom/internal/rss"
	"github.com/loomctl/loom/internal/safety"
	"github.com/loomctl/loom/internal/server"
	"github.com/loomctl/loom/internal/storage"
)

func cmdServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml (overrides $LOOM_CONFIG_DIR/loom.yaml)")
	addr := fs.String("addr", "", "HTTP listen address (e.g. :8989); overrides config")
	logLevel := fs.String("log-level", "", "log level: debug|info|warn|error")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if *addr != "" {
		cfg.HTTP.Addr = *addr
	}
	if *logLevel != "" {
		cfg.Log.Level = *logLevel
	}

	logger, err := logging.New(cfg.Log)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	logger.Info("starting loom",
		"addr", cfg.HTTP.Addr,
		"config_dir", cfg.ConfigDir,
		"data_dir", cfg.DataDir,
	)

	// Load app configuration (loom.json)
	appCfgPath := filepath.Join(cfg.ConfigDir, "loom.json")
	appCfg, err := appconfig.Load(appCfgPath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			// First run — create default config
			appCfg = appconfig.NewDefault()
			if err := appCfg.Save(appCfgPath); err != nil {
				return fmt.Errorf("save default app config: %w", err)
			}
			logger.Info("default app config created", "path", appCfgPath)
		} else {
			// Parse/validation error — fail startup, don't overwrite user's config
			return fmt.Errorf("load app config %s: %w", appCfgPath, err)
		}
	}

	tel, err := telemetry.Init(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}

	db, err := storage.Open(ctx, cfg.Storage, logger)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate storage: %w", err)
	}

	// Startup reconciliation: config is the source of truth for admin credentials.
	// The auth service's ReconcileAdmin will be called after authSvc is created.

	if cfg.HotReload {
		config.OnConfigChange(func(_ *config.Config) {
			logger.Info("config reloaded")
		})
		if config.StartWatch() {
			logger.Info("config hot-reload watching", "path", *configPath)
		}
	}

	authSvc, err := buildAuthService(ctx, cfg, db, appCfg, appCfgPath, logger)
	if err != nil {
		return fmt.Errorf("init auth: %w", err)
	}

	// Reconcile admin user from config on startup.
	// If setup_complete=true and admin credentials are in config, ensure DB matches.
	// This enables: password reset via config edit, K8s pre-configured deployments.
	if appCfg.SetupComplete && appCfg.Admin.Username != "" && appCfg.Admin.PasswordHash != "" {
		if _, err := authSvc.ReconcileAdmin(ctx); err != nil {
			return fmt.Errorf("reconcile admin from config: %w", err)
		}
	}

	sched, err := buildScheduler(ctx, cfg, db, logger)
	if err != nil {
		return fmt.Errorf("init scheduler: %w", err)
	}

	indexerSvc, err := buildIndexerService(ctx, cfg, db, logger)
	if err != nil {
		return fmt.Errorf("init indexers: %w", err)
	}
	if err := registerIndexerHealthJob(ctx, sched, cfg, indexerSvc); err != nil {
		return fmt.Errorf("register indexer health job: %w", err)
	}

	downloadSvc, err := buildDownloadService(ctx, cfg, db, logger)
	if err != nil {
		return fmt.Errorf("init downloads: %w", err)
	}
	if err := registerDownloadHealthJob(ctx, sched, cfg, downloadSvc); err != nil {
		return fmt.Errorf("register download health job: %w", err)
	}

	moviesSvc, err := buildMoviesService(ctx, cfg, db, logger)
	if err != nil {
		return fmt.Errorf("init movies: %w", err)
	}

	rssSvc := rss.NewSourcesService(logger, db)

	// Initialize RSS sync manager and load user sources
	rssMgr, err := buildRSSManager(ctx, sched, db, logger)
	if err != nil {
		return fmt.Errorf("init RSS sync manager: %w", err)
	}
	_ = rssMgr // rssMgr is used via scheduler jobs; keep reference to prevent GC

	sched.Start(ctx)
	defer sched.Stop()

	aggSvc, err := newznabserver.NewServer(newznabserver.Options{
		Search:    indexerSvc,
		Auth:      authSvc,
		Logger:    logger,
		Title:     "Loom",
		Strapline: "Loom Newznab/Torznab aggregator",
	})
	if err != nil {
		return fmt.Errorf("init aggregator: %w", err)
	}

	srv, err := server.New(cfg, appCfg, logger, tel, db, authSvc, indexerSvc, moviesSvc, aggSvc)
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}
	srv.SetDownloads(downloadSvc)
	srv.SetBlocklistStore(downloads.NewBlocklistStore(db.DB()))
	srv.SetRSS(rssSvc)
	srv.SetMovies(moviesSvc)

	// Build and wire the library scanner
	scannerSvc := buildScanner(moviesSvc, cfg, logger)
	srv.SetScanner(scannerSvc)

	// Build and wire the file organizer
	organizerSvc := buildOrganizer(moviesSvc, db, logger)
	if mode := cfg.MediaManagement.ImportMode; mode != "" {
		organizerSvc.SetImportMode(mode)
	}
	srv.SetOrganizer(organizerSvc)

	// Build and wire the TV series service
	seriesSvc := buildSeriesService(db)
	srv.SetSeries(seriesSvc)

	// Build and wire the series scanner
	seriesScannerSvc := buildSeriesScanner(seriesSvc, logger)
	srv.SetSeriesScanner(seriesScannerSvc)

	// Build and wire the notifications service
	notifSvc := buildNotificationsService(db)
	srv.SetNotifications(notifSvc)

	// Build and wire the import pipeline
	importMode := imports.ImportMode(cfg.MediaManagement.ImportMode)
	if importMode == "" {
		importMode = imports.ImportModeMove
	}
	importPipeline, err := imports.NewPipeline(imports.PipelineOptions{
		DB:          db.DB(),
		Bus:         srv.Bus(),
		DownloadSvc: downloadSvc,
		MoviesSvc:   moviesSvc,
		SeriesSvc:   seriesSvc,
		NotifSvc:    notifSvc,
		PostVal:     safety.NewPostValidator(safety.DefaultConfig()),
		ReviewStore: safety.NewReviewStore(db.DB()),
		Logger:      logger,
		ImportMode:  importMode,
	})
	if err != nil {
		return fmt.Errorf("init import pipeline: %w", err)
	}
	importPipeline.Start()
	defer importPipeline.Stop()
	srv.SetImportPipeline(importPipeline)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		return err
	}
	if err := tel.Shutdown(shutdownCtx); err != nil {
		logger.Error("telemetry shutdown failed", "err", err)
	}
	logger.Info("stopped cleanly")
	return nil
}

func cmdHealthcheck(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	url := fs.String("url", defaultHealthURL(), "base URL to probe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(*url + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	return nil
}

func defaultHealthURL() string {
	if v := os.Getenv("LOOM_HEALTH_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:8989"
}

func cmdMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to loom.yaml (overrides $LOOM_CONFIG_DIR/loom.yaml)")
	importMode := fs.Bool("import", false, "import data from radarr/sonarr/prowlarr (Phase 8 — not implemented)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *importMode {
		return errors.New("migrate --import: not implemented yet (Phase 8)")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger, err := logging.New(cfg.Log)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	db, err := storage.Open(ctx, cfg.Storage, logger)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	logger.Info("migrate complete", "engine", string(db.Engine()))
	return nil
}
