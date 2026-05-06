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

	"github.com/loomctl/loom/internal/alttitles"
	"github.com/loomctl/loom/internal/backup"
	"github.com/loomctl/loom/internal/anime"
	"github.com/loomctl/loom/internal/appconfig"
	"github.com/loomctl/loom/internal/calendar"
	"github.com/loomctl/loom/internal/connect"
	"github.com/loomctl/loom/internal/autosearch"
	"github.com/loomctl/loom/internal/customformats"
	"github.com/loomctl/loom/internal/downloads"
	"github.com/loomctl/loom/internal/healthmonitor"
	"github.com/loomctl/loom/internal/importlists"
	"github.com/loomctl/loom/internal/imports"
	"github.com/loomctl/loom/internal/indexers"
	"github.com/loomctl/loom/internal/indexers/newznabserver"
	"github.com/loomctl/loom/internal/languages"
	"github.com/loomctl/loom/internal/libraries"
	"github.com/loomctl/loom/internal/mediainfo"
	"github.com/loomctl/loom/internal/migrate"
	"github.com/loomctl/loom/internal/compat/prowlarrv1"
	"github.com/loomctl/loom/internal/compat/radarrv3"
	"github.com/loomctl/loom/internal/compat/sonarrv3"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/logging"
	"github.com/loomctl/loom/internal/kernel/telemetry"
	"github.com/loomctl/loom/internal/notifications"
	"github.com/loomctl/loom/internal/qualityprofiles"
	"github.com/loomctl/loom/internal/rss"
	"github.com/loomctl/loom/internal/safety"
	"github.com/loomctl/loom/internal/scheduler"
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
	remotePathStore := downloads.NewRemotePathStore(db.DB())
	downloadSvc.AddRouteExtension(downloads.MountRemotePathRoutes(remotePathStore))
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
	srv.SetCustomFormats(customformats.NewStore(db.DB()))

	// Build and wire the libraries store (needed by scanner, organizer, imports)
	libStore := libraries.NewStore(db.DB())

	// Build and wire the library scanner
	scannerSvc := buildScanner(moviesSvc, cfg, logger)
	srv.SetScanner(scannerSvc)

	// Build and wire the file organizer
	organizerSvc := buildOrganizer(moviesSvc, libStore, db, logger)
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

	// Build and wire the connect (media server integrations) service
	connectSvc := connect.NewService(db.DB())
	srv.SetConnect(connectSvc)

	// Build and wire the calendar handler
	srv.SetCalendar(calendar.Router(db.DB()))

	// Start the notification dispatcher — subscribes to domain events on the
	// bus and fans out to all matching notification connections.
	notifDispatcher := notifications.NewDispatcher(srv.Bus(), notifSvc, logger)
	notifDispatcher.Start(ctx)
	defer notifDispatcher.Stop()

	// Build and wire the language-profile store
	langStore := languages.NewStore(db.DB())
	if err := langStore.EnsureDefault(ctx); err != nil {
		return fmt.Errorf("init language profiles: %w", err)
	}
	srv.SetLanguages(langStore)

	// Build and wire the anime store
	srv.SetAnime(anime.NewStore(db.DB(), logger))

	// Build and wire the import lists
	importListStore := importlists.NewStore(db.DB())
	importListSyncMgr := importlists.NewSyncManager(importListStore, logger)
	srv.SetImportLists(importListStore, importListSyncMgr)
	importListSyncMgr.Start(ctx)
	defer importListSyncMgr.Stop()

	// Wire the libraries scanner
	libScanner := libraries.NewScanner(libStore, logger)
	srv.SetLibraries(libStore, libScanner)

	// Build and wire the media-info store
	srv.SetMediaInfo(mediainfo.NewStore(db.DB(), logger))

	// Build and wire the alt-title store
	srv.SetAltTitles(alttitles.NewStore(db.DB()))

	// Build and wire the quality profiles store
	qpStore := qualityprofiles.NewStore(db.DB())
	srv.SetQualityProfiles(qpStore)
	qualityprofiles.SeedDefaults(ctx, qpStore, moviesSvc)

	// Build and wire the autosearch decision engine
	cfStore := customformats.NewStore(db.DB())
	cfFormats, _ := cfStore.List(ctx) // best-effort; empty is OK at boot
	cfEngine := customformats.NewEngine(cfFormats)
	autoSearchEngine := autosearch.NewEngine(
		indexerSvc, qpStore, cfEngine, cfStore,
		downloadSvc.Registry(), moviesSvc, logger,
	)
	srv.SetAutoSearchEngine(autoSearchEngine)

	// Wire *arr API compatibility shims
	srv.SetCompatRadarr(radarrv3.NewHandler(moviesSvc, libStore, qpStore, logger))
	srv.SetCompatSonarr(sonarrv3.NewHandler(seriesSvc, libStore, qpStore, logger))
	srv.SetCompatProwlarr(prowlarrv1.NewHandler(indexerSvc, logger))

	// Build and wire the import pipeline
	importMode := imports.ImportMode(cfg.MediaManagement.ImportMode)
	if importMode == "" {
		importMode = imports.ImportModeMove
	}
	importPipeline, err := imports.NewPipeline(imports.PipelineOptions{
		DB:              db.DB(),
		Bus:             srv.Bus(),
		DownloadSvc:     downloadSvc,
		RemotePathStore: remotePathStore,
		MoviesSvc:       moviesSvc,
		SeriesSvc:       seriesSvc,
		LibStore:        libStore,
		NotifSvc:        notifSvc,
		PostVal:         safety.NewPostValidator(safety.DefaultConfig()),
		ReviewStore:     safety.NewReviewStore(db.DB()),
		Logger:          logger,
		ImportMode:      importMode,
	})
	if err != nil {
		return fmt.Errorf("init import pipeline: %w", err)
	}
	importPipeline.Start()
	defer importPipeline.Stop()
	srv.SetImportPipeline(importPipeline)

	// Build and wire the rolling-search scheduler
	rsCfg := scheduler.DefaultRollingSearchConfig()
	rsStore := scheduler.NewStore(db.DB())
	rollingSearcher := scheduler.NewRollingSearcher(rsStore, indexerSvc, logger, rsCfg)
	srv.SetRollingSearch(rollingSearcher)
	rollingSearcher.Start(ctx)
	defer rollingSearcher.Stop()

	// Build and wire the health monitor
	healthMon := buildHealthMonitor(ctx, indexerSvc, downloadSvc, notifSvc, libStore, logger)
	srv.SetHealthMonitor(healthMon)
	healthMon.Start(ctx)
	defer healthMon.Stop()

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

// --- Health monitor wiring ---

// indexerHealthAdapter wraps *indexers.Service to satisfy healthmonitor.IndexerChecker.
type indexerHealthAdapter struct {
	svc *indexers.Service
}

func (a *indexerHealthAdapter) List(ctx context.Context) ([]healthmonitor.IndexerInfo, error) {
	defs, err := a.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]healthmonitor.IndexerInfo, len(defs))
	for i, d := range defs {
		status := "unknown"
		if d.Health != nil {
			status = string(d.Health.Status)
		}
		out[i] = healthmonitor.IndexerInfo{
			ID: d.ID, Name: d.Name, Enabled: d.Enabled, Status: status,
		}
	}
	return out, nil
}

// downloadHealthAdapter wraps *downloads.Service to satisfy healthmonitor.DownloadChecker.
type downloadHealthAdapter struct {
	svc *downloads.Service
}

func (a *downloadHealthAdapter) ListClients(ctx context.Context) ([]healthmonitor.ClientInfo, error) {
	defs, err := a.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]healthmonitor.ClientInfo, len(defs))
	for i, d := range defs {
		status := "unknown"
		if d.Health != nil {
			status = string(d.Health.Status)
		}
		out[i] = healthmonitor.ClientInfo{
			ID: d.ID, Name: d.Name, Enabled: d.Enabled, Status: status,
		}
	}
	return out, nil
}

func buildHealthMonitor(
	ctx context.Context,
	indexerSvc *indexers.Service,
	downloadSvc *downloads.Service,
	notifSvc notifications.Service,
	libStore *libraries.Store,
	logger *slog.Logger,
) *healthmonitor.Monitor {
	// Collect library paths for disk-space checks.
	var libPaths []string
	if libStore != nil {
		libs, err := libStore.List(ctx)
		if err == nil {
			for _, l := range libs {
				if l.Path != "" {
					libPaths = append(libPaths, l.Path)
				}
			}
		}
	}

	// Build notification sender closure.
	var notifier healthmonitor.NotificationSender
	if notifSvc != nil {
		notifier = func(ctx context.Context, title, body string) error {
			return notifSvc.Send(ctx, notifications.EventOnHealthIssue, title, body, nil)
		}
	}

	var idxChecker healthmonitor.IndexerChecker
	if indexerSvc != nil {
		idxChecker = &indexerHealthAdapter{svc: indexerSvc}
	}
	var dlChecker healthmonitor.DownloadChecker
	if downloadSvc != nil {
		dlChecker = &downloadHealthAdapter{svc: downloadSvc}
	}

	return healthmonitor.New(healthmonitor.Options{
		Indexers:  idxChecker,
		Downloads: dlChecker,
		Notifier:  notifier,
		LibPaths:  libPaths,
		Logger:    logger.With("component", "health-monitor"),
	})
}
