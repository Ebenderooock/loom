package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/loomctl/loom/internal/indexers/newznabserver"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/logging"
	"github.com/loomctl/loom/internal/kernel/telemetry"
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

	if cfg.HotReload {
		config.OnConfigChange(func(_ *config.Config) {
			logger.Info("config reloaded")
		})
		if config.StartWatch() {
			logger.Info("config hot-reload watching", "path", *configPath)
		}
	}

	authSvc, err := buildAuthService(ctx, cfg, db, logger)
	if err != nil {
		return fmt.Errorf("init auth: %w", err)
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

	srv, err := server.New(cfg, logger, tel, db, authSvc, indexerSvc, aggSvc)
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}
	srv.SetDownloads(downloadSvc)

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
