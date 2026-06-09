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
	"github.com/ebenderooock/loom/internal/featureflags"
	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/music"
	"github.com/ebenderooock/loom/internal/musicsearch"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
	"github.com/ebenderooock/loom/internal/workflows"
)

// downloadWiring holds lifecycle objects produced by wireDownloads
// so the caller can manage their shutdown.
type downloadWiring struct {
	importPipeline     *imports.ImportPipeline
	orchestratorCancel context.CancelFunc
	monitorCancel      context.CancelFunc
	autoSearchEngine   *autosearch.Engine
	musicAutoSearcher  *musicsearch.AutoSearcher
	musicRefresher     *music.ReleaseRefresher
}

// wireDownloads constructs download-related services (remote paths,
// blocklist, workflows, autosearch, import pipeline, download monitor)
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

	// Workflow engine for tracking active downloads
	wfStore, err := workflows.NewStore(db.DB())
	if err != nil {
		return nil, fmt.Errorf("init workflow store: %w", err)
	}
	wfEngine := workflows.NewEngine(wfStore, workflowMediaAdapter{moviesSvc}, logger)

	downloadSvc.SetWorkflowEngine(wfEngine)
	downloadSvc.SetMovieStatusUpdater(movieStatusAdapter{moviesSvc})
	srv.SetWorkflowEngine(wfEngine)

	// Workflow orchestrator — unified state coordinator (created early so callers can reference it)
	orchestrator := workflows.NewOrchestrator(workflows.OrchestratorOpts{
		Store:          wfStore,
		Engine:         wfEngine,
		Logger:         logger,
		DownloadStatus: downloadSvc,
	})
	downloadSvc.SetOrchestrator(orchestrator)

	// Autosearch decision engine
	cfStore := customformats.NewStore(db.DB())
	cfFormats, _ := cfStore.List(ctx) // best-effort; empty is OK at boot
	cfEngine := customformats.NewEngine(cfFormats)
	autoSearchEngine := autosearch.NewEngine(
		indexerSvc, media.qpStore, cfEngine, cfStore,
		downloadSvc.Registry(), moviesSvc, media.seriesSvc, logger,
		autosearch.WithAuditLogger(auditLogger),
		autosearch.WithOrchestrator(orchestrator),
		autosearch.WithDebugStore(srv.SearchDebugStore()),
		autosearch.WithDebugHub(srv.SearchDebugHub()),
		autosearch.WithSearchLogEnabled(srv.Features().EnabledFunc(featureflags.KeySearchLog)),
	)
	srv.SetAutoSearchEngine(autoSearchEngine)

	// Music acquisition engine — self-contained parallel to autosearch that
	// reuses only the media-agnostic indexer transport and download registry.
	var musicAutoSearcher *musicsearch.AutoSearcher
	var musicRefresher *music.ReleaseRefresher
	if media.musicRepo != nil {
		musicSearchEngine := musicsearch.NewEngine(
			indexerSvc, downloadSvc.Registry(), media.musicRepo, logger,
		)
		srv.SetMusicSearch(musicSearchEngine)
		musicAutoSearcher = musicsearch.NewAutoSearcher(
			musicSearchEngine, srv.Features().EnabledFunc(featureflags.KeyMusic),
		)
		if media.musicSvc != nil {
			musicRefresher = music.NewReleaseRefresher(
				media.musicSvc, media.musicRepo,
				srv.Features().EnabledFunc(featureflags.KeyMusic), logger,
			)
		}
	}

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
		WorkflowEngine:  wfEngine,
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
	orchestrator.SetImportFn(importPipeline.RunImport)

	// Wire post-import cleanup: remove download from client queue + clean source folder.
	folderCleaner := &imports.FolderCleaner{}
	orchestrator.SetCleanupFn(func(ctx context.Context, clientID, downloadID string, _ []string) error {
		c, ok := downloadSvc.Registry().Get(clientID)
		if !ok {
			return fmt.Errorf("download client %q not found in registry", clientID)
		}
		// Get content path before removing so we can clean the folder.
		items, err := c.Status(ctx, downloadID)
		var contentPath string
		if err == nil && len(items) > 0 {
			contentPath = items[0].ContentPath
			if contentPath == "" {
				contentPath = items[0].SavePath
			}
		}
		// Remove from client queue (keep files; they've been moved/hardline to library).
		if err := c.Remove(ctx, []string{downloadID}, false); err != nil {
			return fmt.Errorf("remove download from client: %w", err)
		}
		// Clean up any leftover junk in the source folder.
		if contentPath != "" {
			if _, err := folderCleaner.CleanFolder(contentPath); err != nil {
				// Non-fatal: log but don't fail the cleanup step.
				logger.Warn("folder cleanup failed", "path", contentPath, "error", err)
			}
		}
		return nil
	})

	// Wire post-import media refresh.
	orchestrator.SetMediaRefreshFn(func(ctx context.Context, mediaType string, mediaIDs []string) error {
		for _, id := range mediaIDs {
			switch mediaType {
			case "movie":
				if err := moviesSvc.RefreshMovie(ctx, id); err != nil {
					return fmt.Errorf("refresh movie %s: %w", id, err)
				}
			case "episode":
				if err := media.seriesSvc.RefreshSeries(ctx, id); err != nil {
					return fmt.Errorf("refresh series %s: %w", id, err)
				}
			}
		}
		return nil
	})

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
		OrchNotifier:    orchestrator,
	})
	if err != nil {
		return nil, fmt.Errorf("init download monitor: %w", err)
	}
	monCtx, monCancel := context.WithCancel(ctx)
	go downloadMonitor.RunLoop(monCtx)

	// Start orchestrator goroutine
	orchCtx, orchCancel := context.WithCancel(ctx)
	go orchestrator.Run(orchCtx)
	srv.SetOrchestrator(orchestrator)

	// Register workflow API routes
	// (handled in server.go newMux via wfEngine field)

	return &downloadWiring{
		importPipeline:     importPipeline,
		orchestratorCancel: orchCancel,
		monitorCancel:      monCancel,
		autoSearchEngine:   autoSearchEngine,
		musicAutoSearcher:  musicAutoSearcher,
		musicRefresher:     musicRefresher,
	}, nil
}

// movieStatusAdapter adapts movies.Service to downloads.MovieStatusUpdater.
type movieStatusAdapter struct {
	svc movies.Service
}

func (a movieStatusAdapter) SetMovieStatus(ctx context.Context, movieID string, status string) error {
	return a.svc.SetMovieStatus(ctx, movieID, movies.MovieStatus(status))
}

// workflowMediaAdapter adapts movies.Service to workflows.MediaStatusUpdater.
type workflowMediaAdapter struct {
	svc movies.Service
}

func (a workflowMediaAdapter) SetMovieDownloading(ctx context.Context, movieID string) error {
	return a.svc.SetMovieStatus(ctx, movieID, movies.MovieStatusDownloading)
}

func (a workflowMediaAdapter) SetMovieMissing(ctx context.Context, movieID string) error {
	return a.svc.SetMovieStatus(ctx, movieID, movies.MovieStatusMissing)
}
