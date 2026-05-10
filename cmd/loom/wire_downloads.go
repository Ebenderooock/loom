package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/autosearch"
	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/grabs"
	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
)

// downloadWiring holds lifecycle objects produced by wireDownloads
// so the caller can manage their shutdown.
type downloadWiring struct {
	importPipeline *imports.ImportPipeline
	monitorCancel  context.CancelFunc
}

// wireDownloads constructs download-related services (remote paths,
// blocklist, grabs, autosearch, import pipeline, download monitor)
// and mounts them on srv.
func wireDownloads(
	ctx context.Context,
	cfg *config.Config,
	db storage.DB,
	srv *server.Server,
	downloadSvc *downloads.Service,
	moviesSvc movies.Service,
	indexerSvc *indexers.Service,
	media *mediaWiring,
	auditLogger *auditlog.Logger,
	logger *slog.Logger,
) (*downloadWiring, error) {
	// Remote path mappings
	remotePathStore := downloads.NewRemotePathStore(db.DB())
	downloadSvc.AddRouteExtension(downloads.MountRemotePathRoutes(remotePathStore))

	// Blocklist
	blocklistStore := downloads.NewBlocklistStore(db.DB())
	srv.SetBlocklistStore(blocklistStore)

	// Grab store for tracking active downloads
	grabStore := grabs.NewStore(db.DB())
	downloadSvc.SetGrabStore(grabStore)
	srv.SetGrabStore(grabStore)

	// Autosearch decision engine
	cfStore := customformats.NewStore(db.DB())
	cfFormats, _ := cfStore.List(ctx) // best-effort; empty is OK at boot
	cfEngine := customformats.NewEngine(cfFormats)
	autoSearchEngine := autosearch.NewEngine(
		indexerSvc, media.qpStore, cfEngine, cfStore,
		downloadSvc.Registry(), moviesSvc, media.seriesSvc, grabStore, logger,
		autosearch.WithAuditLogger(auditLogger),
	)
	srv.SetAutoSearchEngine(autoSearchEngine)

	// Import pipeline
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
		SeriesSvc:       media.seriesSvc,
		LibStore:        media.libStore,
		GrabStore:       grabStore,
		NotifSvc:        media.notifSvc,
		PostVal:         safety.NewPostValidator(safety.DefaultConfig()),
		ReviewStore:     safety.NewReviewStore(db.DB()),
		Logger:          logger,
		ImportMode:      importMode,
	})
	if err != nil {
		return nil, fmt.Errorf("init import pipeline: %w", err)
	}
	importPipeline.Start()
	srv.SetImportPipeline(importPipeline)

	// Download monitor — polls clients for completion
	downloadHistoryStore := downloads.NewHistoryStore(db.DB())
	stallHandler := downloads.NewStallHandler(downloads.StallHandlerOptions{
		Registry:  downloadSvc.Registry(),
		Blocklist: blocklistStore,
		Bus:       srv.Bus(),
		Logger:    logger,
	})
	downloadMonitor, err := downloads.NewMonitor(downloads.MonitorOptions{
		Service:         downloadSvc,
		Bus:             srv.Bus(),
		Logger:          logger,
		CheckInterval:   30 * time.Second,
		StallTimeout:    30 * time.Minute,
		CheckForStalled: true,
		StallHandler:    stallHandler,
		HistoryStore:    downloadHistoryStore,
		GrabStore:       grabStore,
	})
	if err != nil {
		return nil, fmt.Errorf("init download monitor: %w", err)
	}
	monCtx, monCancel := context.WithCancel(ctx)
	go downloadMonitor.RunLoop(monCtx)

	return &downloadWiring{
		importPipeline: importPipeline,
		monitorCancel:  monCancel,
	}, nil
}
