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
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/migrate"
	"github.com/ebenderooock/loom/internal/rss"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/logging"
	"github.com/ebenderooock/loom/internal/kernel/telemetry"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
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

	sched, err := buildScheduler(ctx, cfg, db, auditLogger, logger)
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

	// Create the shared event bus before server and services that need it.
	bus := eventbus.NewInProc()

	moviesSvc, err := buildMoviesService(ctx, cfg, db, logger, bus)
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
	defer infra.auditSink.Close()

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
