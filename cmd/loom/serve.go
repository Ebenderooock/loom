package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/backup"
	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/kernel/logging"
	"github.com/ebenderooock/loom/internal/kernel/scheduler"
	"github.com/ebenderooock/loom/internal/kernel/telemetry"
	"github.com/ebenderooock/loom/internal/migrate"
	"github.com/ebenderooock/loom/internal/rss"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
	"github.com/ebenderooock/loom/internal/systemlogs"
	"github.com/ebenderooock/loom/internal/workflows"
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
	slog.SetDefault(logger)
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

	auditLogger := auditlog.New(db.DB(), logger)

	// System logs capture — ring buffer + DB persistence.
	logBuf := logging.NewRingBuffer(cfg.Log.BufferSize)
	sysLogStore := systemlogs.NewStore(db.DB())
	sysLogBatchWriter := sysLogStore.NewBatchWriter(ctx)
	captureLevel, _ := logging.ParseCaptureLevel(cfg.Log.CaptureLevel)
	captureHandler := logging.NewCaptureHandler(logging.CaptureHandlerConfig{
		Inner:             logger.Handler(),
		Buffer:            logBuf,
		DBSink:            sysLogBatchWriter.Sink(),
		CaptureLevel:      captureLevel,
		ExtractWorkflowID: workflows.WorkflowIDFromContext,
	})
	capturedLogger := slog.New(captureHandler)
	slog.SetDefault(capturedLogger)
	logger = capturedLogger

	sched, err := buildScheduler(ctx, cfg, db, auditLogger, logger)
	if err != nil {
		return fmt.Errorf("init scheduler: %w", err)
	}

	// Register system log pruning job.
	if err := scheduler.RegisterSystemLogPrune(ctx, sched, sysLogStore, cfg.Log.RetentionDays, logger); err != nil {
		return fmt.Errorf("register system log prune: %w", err)
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

	// Create the shared event bus before server and services that need it.
	bus := eventbus.NewInProc()

	moviesSvc, metadataSvc, err := buildMoviesService(ctx, cfg, db, logger, bus)
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

	// Build Newznab/Torznab aggregator server
	aggSvc, err := wireAggregator(indexerSvc, authSvc, logger)
	if err != nil {
		return fmt.Errorf("init aggregator: %w", err)
	}

	srv, err := server.New(cfg, appCfg, logger, tel, db, authSvc, indexerSvc, moviesSvc, aggSvc, bus)
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}
	srv.SetDownloads(downloadSvc)
	srv.SetRSS(rssSvc)
	srv.SetMovies(moviesSvc)
	srv.SetCustomFormats(customformats.NewStore(db.DB()))
	srv.SetSystemLogs(&systemlogs.HandlerDeps{
		Store:   sysLogStore,
		Buffer:  logBuf,
		Capture: captureHandler,
	})

	// Wire media services (scanner, organizer, series, libraries, etc.)
	media, err := wireMedia(ctx, cfg, db, srv, moviesSvc, auditLogger, logger)
	if err != nil {
		return fmt.Errorf("wire media: %w", err)
	}
	media.importListSyncMgr.Start(ctx)
	defer media.importListSyncMgr.Stop()

	// Wire download services (grabs, autosearch, import pipeline, monitor)
	dlWiring, err := wireDownloads(ctx, cfg, db, srv, downloadSvc, moviesSvc, indexerSvc, media, auditLogger, logger)
	if err != nil {
		return fmt.Errorf("wire downloads: %w", err)
	}
	defer dlWiring.importPipeline.Stop()
	defer dlWiring.monitorCancel()
	defer dlWiring.orchestratorCancel()

	// Wire downloads-cleanup (scan orphans in download folders + auto-delete).
	cleanupSvc := buildCleanupService(db, downloadSvc, dlWiring.importPipeline, media.libStore, appCfg, logger)
	srv.SetCleanup(cleanupSvc)
	if err := registerCleanupJob(ctx, sched, cleanupSvc); err != nil {
		return fmt.Errorf("register cleanup job: %w", err)
	}

	// Wire media requests (users request; admins approve → auto-add + grab).
	requestsSvc := buildRequestsService(db, moviesSvc, media.seriesSvc, dlWiring.autoSearchEngine, media.libStore, media.qpStore, logger)
	srv.SetRequests(requestsSvc)

	// Wire interactive request bots (Telegram + Discord).
	botAuthStore, err := buildAuthStore(db)
	if err != nil {
		return fmt.Errorf("init bot auth store: %w", err)
	}
	botsRouter, botsSupervisor := buildBots(db, requestsSvc, metadataSvc, botAuthStore, authSvc.RequireRole("admin"), logger)
	srv.SetBots(botsRouter)
	if err := botsSupervisor.Start(ctx); err != nil {
		logger.Error("bots: initial start failed", "err", err)
	}
	defer botsSupervisor.Shutdown()

	// Wire infrastructure services (connect, compat, notifications, rolling search, health)
	infra, err := wireInfra(ctx, db, srv, indexerSvc, downloadSvc, moviesSvc, media, auditLogger, logger)
	if err != nil {
		return fmt.Errorf("wire infra: %w", err)
	}
	infra.notifDispatcher.Start(ctx)
	defer infra.notifDispatcher.Stop()
	infra.rollingSearcher.Start(ctx)
	defer infra.rollingSearcher.Stop()
	infra.healthMon.Start(ctx)
	defer infra.healthMon.Stop()
	if infra.analyticsPoller != nil {
		infra.analyticsPoller.Start(ctx)
		defer infra.analyticsPoller.Stop()
	}
	if infra.pluginRunner != nil {
		infra.pluginRunner.Start(ctx)
		defer infra.pluginRunner.Stop()
	}
	defer infra.auditSink.Close()
	defer sysLogBatchWriter.Close()

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
	from := fs.String("from", "", "source app to import from: radarr, sonarr, or prowlarr")
	sourceDB := fs.String("db", "", "path to the source app's SQLite database file")
	if err := fs.Parse(args); err != nil {
		return err
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

	// Always run schema migrations first.
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// If no --from flag, just do schema migration (existing behavior).
	if *from == "" {
		logger.Info("migrate complete", "engine", string(db.Engine()))
		return nil
	}

	if *sourceDB == "" {
		return errors.New("--db flag is required when using --from")
	}

	imp := migrate.NewImporter(db.DB(), logger)

	var res *migrate.ImportResult
	switch *from {
	case "radarr":
		res, err = imp.ImportRadarr(ctx, *sourceDB)
	case "sonarr":
		res, err = imp.ImportSonarr(ctx, *sourceDB)
	case "prowlarr":
		res, err = imp.ImportProwlarr(ctx, *sourceDB)
	default:
		return fmt.Errorf("unknown --from source %q (expected radarr, sonarr, or prowlarr)", *from)
	}
	if err != nil {
		return fmt.Errorf("import from %s: %w", *from, err)
	}

	fmt.Print(res.Summary())
	return nil
}

func cmdBackup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	action := fs.String("action", "", "create or restore")
	outputPath := fs.String("output", "", "output path for backup tarball (create only)")
	inputPath := fs.String("input", "", "path to backup tarball (restore only)")
	configDir := fs.String("config-dir", "", "config directory (default: auto-detect)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := *configDir
	if dir == "" {
		cfg, err := config.Load("")
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		dir = cfg.ConfigDir
	}

	switch *action {
	case "create":
		path, err := backup.CreateBackup(dir, *outputPath)
		if err != nil {
			return fmt.Errorf("create backup: %w", err)
		}
		fmt.Printf("backup created: %s\n", path)
		return nil

	case "restore":
		if *inputPath == "" {
			return errors.New("--input is required for restore")
		}
		if _, err := os.Stat(*inputPath); err != nil {
			return fmt.Errorf("backup file not found: %w", err)
		}
		if err := backup.RestoreBackup(dir, *inputPath); err != nil {
			return fmt.Errorf("restore backup: %w", err)
		}
		fmt.Println("backup restored successfully")
		return nil

	case "":
		return errors.New("--action is required (create or restore)")
	default:
		return fmt.Errorf("unknown action %q (expected create or restore)", *action)
	}
}
