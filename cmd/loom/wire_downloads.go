package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/autosearch"
	"github.com/ebenderooock/loom/internal/calendar"
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
	"github.com/ebenderooock/loom/internal/scheduler"
	"github.com/ebenderooock/loom/internal/series"
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
	router             *downloads.Router
	autoSearchEngine   *autosearch.Engine
	musicAutoSearcher  *musicsearch.AutoSearcher
	musicRefresher     *music.ReleaseRefresher
	musicSearchEngine  *musicsearch.Engine
}

// wireDownloads constructs download-related services (remote paths,
// blocklist, workflows, autosearch, import pipeline, download monitor)
// and records them into wiring.
func wireDownloads(
	ctx context.Context,
	cfg *config.Config,
	db storage.DB,
	wiring *server.Wiring,
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
	wiring.BlocklistStore = blocklistStore
	downloadSvc.SetBlocklist(blocklistStore)
	downloadHistoryStore := downloads.NewHistoryStore(db.DB())
	downloadSvc.SetHistoryStore(downloadHistoryStore)

	// Workflow engine for tracking active downloads
	wfStore, err := workflows.NewStore(db.DB())
	if err != nil {
		return nil, fmt.Errorf("init workflow store: %w", err)
	}
	wfEngine := workflows.NewEngine(wfStore, workflowMediaAdapter{moviesSvc, media.seriesSvc}, logger)
	wiring.CalendarHandler = calendar.Router(db.DB(), wfStore)

	downloadSvc.SetWorkflowEngine(wfEngine)
	downloadSvc.SetMovieStatusUpdater(movieStatusAdapter{moviesSvc})
	downloadSvc.SetBuiltinTorrentEnabled(wiring.FeatureFlags.EnabledFunc(featureflags.KeyBuiltinTorrent))
	wiring.WorkflowEngine = wfEngine

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
		autosearch.WithDebugStore(wiring.SearchDebugStore),
		autosearch.WithDebugHub(wiring.SearchDebugHub),
		autosearch.WithSearchLogEnabled(wiring.FeatureFlags.EnabledFunc(featureflags.KeySearchLog)),
		autosearch.WithBlocklist(blocklistStore),
	)
	wiring.AutoSearchEngine = autoSearchEngine

	// When a download is abandoned (stalled, failed, vanished, unimportable)
	// the orchestrator asks for a replacement release. The stalled release is
	// already blocklisted, so the search picks a different one.
	orchestrator.SetResearchFn(newResearchFunc(autoSearchEngine, moviesSvc, media.seriesSvc, logger))

	// Music acquisition engine — self-contained parallel to autosearch that
	// reuses only the media-agnostic indexer transport and download registry.
	var musicAutoSearcher *musicsearch.AutoSearcher
	var musicRefresher *music.ReleaseRefresher
	var musicSearchEngine *musicsearch.Engine
	if media.musicRepo != nil {
		musicSearchEngine = musicsearch.NewEngine(
			indexerSvc, downloadSvc.Registry(), media.musicRepo, logger,
		)
		musicSearchEngine.SetCustomFormats(cfEngine)
		wiring.MusicSearch = musicSearchEngine
		musicAutoSearcher = musicsearch.NewAutoSearcher(
			musicSearchEngine, wiring.FeatureFlags.EnabledFunc(featureflags.KeyMusic),
		)
		if media.musicSvc != nil {
			musicRefresher = music.NewReleaseRefresher(
				media.musicSvc, media.musicRepo,
				wiring.FeatureFlags.EnabledFunc(featureflags.KeyMusic), logger,
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
		Bus:             wiring.Bus,
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
	// The monitor delivers completions to the orchestrator, including orphans.
	// Do not also subscribe the importer to the asynchronous completion event.
	wiring.ImportPipeline = importPipeline
	orchestrator.SetImportFn(importPipeline.RunImport)

	// Keep the download in the client until import succeeds so failures remain
	// recoverable. Pause it and cache paths before touching the files.
	orchestrator.SetPreImportFn(func(ctx context.Context, clientID, downloadID string) (string, string, error) {
		c, ok := downloadSvc.Registry().Get(clientID)
		if !ok {
			return "", "", fmt.Errorf("download client %q not found in registry", clientID)
		}
		// Resolve the on-disk paths while the torrent still exists.
		var contentPath, savePath string
		items, err := c.Status(ctx, downloadID)
		if err != nil {
			return "", "", fmt.Errorf("resolve pre-import download paths: %w", err)
		}
		for _, item := range items {
			if item.ID == downloadID {
				contentPath = item.ContentPath
				savePath = item.SavePath
				break
			}
		}
		if err := c.Pause(ctx, downloadID); err != nil {
			return contentPath, savePath, fmt.Errorf("pause download before import: %w", err)
		}
		return contentPath, savePath, nil
	})

	// Wire post-import cleanup: remove download from client queue (if still
	// present) + clean source folder junk, only after successful import.
	folderCleaner := &imports.FolderCleaner{}
	orchestrator.SetCleanupFn(func(ctx context.Context, clientID, downloadID, sourcePath string, importedPaths []string) error {
		c, ok := downloadSvc.Registry().Get(clientID)
		if !ok {
			return fmt.Errorf("download client %q not found in registry", clientID)
		}
		contentPath := remotePathStore.MapPath(ctx, clientID, sourcePath)
		// The torrent may already have been removed during pre-import; tolerate
		// a missing item. If still present, capture its path and remove it.
		items, err := c.Status(ctx, downloadID)
		if err != nil {
			return fmt.Errorf("check download before source cleanup: %w", err)
		}
		if len(items) > 0 {
			if contentPath == "" {
				p := items[0].ContentPath
				if p == "" {
					p = items[0].SavePath
				}
				contentPath = remotePathStore.MapPath(ctx, clientID, p)
			}
			// Remove from client queue (keep files; already imported).
			if err := c.Remove(ctx, []string{downloadID}, false); err != nil {
				return fmt.Errorf("remove imported download from client: %w", err)
			}
		}
		if err := importPipeline.FinalizeImports(ctx, importedPaths); err != nil {
			return err
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
		if len(mediaIDs) == 0 {
			return nil
		}
		for _, id := range mediaIDs {
			switch mediaType {
			case "movie":
				if err := moviesSvc.RefreshMovie(ctx, id); err != nil {
					return fmt.Errorf("refresh movie %s: %w", id, err)
				}
			case "episode":
				ep, err := media.seriesSvc.GetEpisode(ctx, id)
				if err != nil {
					return fmt.Errorf("get episode %s: %w", id, err)
				}
				if ep == nil || ep.SeriesID == "" {
					return fmt.Errorf("episode %s has no series_id", id)
				}
				if err := media.seriesSvc.RefreshSeries(ctx, ep.SeriesID); err != nil {
					return fmt.Errorf("refresh series %s (episode %s): %w", ep.SeriesID, id, err)
				}
			}
		}
		return nil
	})

	// Download monitor — polls clients for completion
	stallTimeout := time.Duration(cfg.Downloads.StallTimeoutMin) * time.Minute
	if stallTimeout <= 0 {
		stallTimeout = 30 * time.Minute
	}
	stallHandler := downloads.NewStallHandler(downloads.StallHandlerOptions{
		Registry:   downloadSvc.Registry(),
		Blocklist:  blocklistStore,
		Bus:        wiring.Bus,
		Logger:     logger,
		Action:     downloads.StallAction(cfg.Downloads.StallAction),
		MaxRetries: cfg.Downloads.MaxRetries,
	})
	downloadMonitor, err := downloads.NewMonitor(downloads.MonitorOptions{
		Service:           downloadSvc,
		Bus:               wiring.Bus,
		Logger:            logger,
		CheckInterval:     30 * time.Second,
		StallTimeout:      stallTimeout,
		CheckForStalled:   cfg.Downloads.CheckForStalled,
		ConnectivityCheck: internetReachable,
		StallHandler:      stallHandler,
		HistoryStore:      downloadHistoryStore,
		OrchNotifier:      orchestrator,
	})
	if err != nil {
		return nil, fmt.Errorf("init download monitor: %w", err)
	}
	monCtx, monCancel := context.WithCancel(ctx)
	go downloadMonitor.RunLoop(monCtx)

	// Event-bus router from indexer results to download clients.
	router := downloads.NewRouter(
		downloadSvc,
		nil, // metadata router wiring is separate; router safely skips enrichment when nil
		wiring.Bus,
		logger,
		downloads.SystemClock{},
		indexerSvc.Get,
		indexerSvc.FetchDownload,
	)
	router.SetLifecycleContext(ctx)

	// Start orchestrator goroutine
	orchCtx, orchCancel := context.WithCancel(ctx)
	go orchestrator.Run(orchCtx)
	wiring.Orchestrator = orchestrator

	// Register workflow API routes
	// (handled in server.go newMux via wfEngine field)

	return &downloadWiring{
		importPipeline:     importPipeline,
		orchestratorCancel: orchCancel,
		monitorCancel:      monCancel,
		router:             router,
		autoSearchEngine:   autoSearchEngine,
		musicAutoSearcher:  musicAutoSearcher,
		musicRefresher:     musicRefresher,
		musicSearchEngine:  musicSearchEngine,
	}, nil
}

// movieStatusAdapter adapts movies.Service to downloads.MovieStatusUpdater.
type movieStatusAdapter struct {
	svc movies.Service
}

func (a movieStatusAdapter) SetMovieStatus(ctx context.Context, movieID string, status string) error {
	return a.svc.SetMovieStatus(ctx, movieID, movies.MovieStatus(status))
}

// workflowMediaAdapter adapts movies.Service and series.Service to workflows.MediaStatusUpdater.
type workflowMediaAdapter struct {
	moviesSvc movies.Service
	seriesSvc series.Service
}

func (a workflowMediaAdapter) SetMovieDownloading(ctx context.Context, movieID string) error {
	return a.moviesSvc.SetMovieStatus(ctx, movieID, movies.MovieStatusDownloading)
}

// SetMovieMissing reverts a movie that is shown as downloading. Movies in any
// other status (e.g. already available while an upgrade failed) are left
// untouched so a failed or cancelled upgrade never hides an existing file.
func (a workflowMediaAdapter) SetMovieMissing(ctx context.Context, movieID string) error {
	m, err := a.moviesSvc.GetMovie(ctx, movieID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != movies.MovieStatusDownloading {
		return nil
	}
	return a.moviesSvc.SetMovieStatus(ctx, movieID, movies.MovieStatusMissing)
}

func (a workflowMediaAdapter) SetEpisodeDownloading(ctx context.Context, episodeID string) error {
	// Episodes don't have a status field like movies; just mark as monitored
	// The workflow system tracks the downloading status separately
	// For now, this is a no-op since episodes are tracked via has_file
	return nil
}

func (a workflowMediaAdapter) SetEpisodeMissing(ctx context.Context, episodeID string) error {
	// Episodes don't have a status field like movies; just mark as monitored
	// When a workflow fails, the episode can be re-searched
	// For now, this is a no-op since episodes are tracked via has_file
	return nil
}

// autoSearchGrabber adapts autosearch.Engine to the scheduler.Grabber interface
// so the rolling searcher can trigger full search-and-grab cycles.
type autoSearchGrabber struct {
	engine *autosearch.Engine
}

func (g *autoSearchGrabber) Grab(ctx context.Context, c scheduler.SearchCandidate) error {
	if g.engine == nil {
		return nil
	}
	mediaType := c.MediaType
	mediaID := c.MediaID
	// Align scheduler episode searches with manual/regular series searches:
	// search as series + S/E targeting, while still tracking episode state
	// separately in scheduler.search_state.
	if c.MediaType == "episode" && c.SeriesID != "" {
		mediaType = "series"
		mediaID = c.SeriesID
	}
	req := autosearch.SearchRequest{
		RequireSuccessfulSearch: true,

		MediaType:        mediaType,
		MediaID:          mediaID,
		Title:            c.Title,
		Year:             c.Year,
		QualityProfileID: c.QualityProfileID,
		IMDBID:           c.IMDBID,
		TVDBID:           c.TVDBID,
		TMDBID:           c.TMDBID,
		Season:           c.Season,
		Episode:          c.Episode,
	}
	_, err := g.engine.SearchAndGrab(ctx, req)
	return err
}

// internetReachable is the download monitor's connectivity probe. It lets
// stall detection distinguish a dead download from a network outage: stall
// time is not counted while no well-known host is reachable.
func internetReachable(ctx context.Context) bool {
	targets := []string{"1.1.1.1:443", "8.8.8.8:53", "9.9.9.9:443"}
	d := net.Dialer{Timeout: 3 * time.Second}
	for _, addr := range targets {
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// newResearchFunc returns the orchestrator hook that searches for a
// replacement release after a download is abandoned. Only monitored media is
// searched; episodes are grouped per season so a failed season pack is
// replaced by a season search rather than many individual searches.
func newResearchFunc(engine *autosearch.Engine, moviesSvc movies.Service, seriesSvc series.Service, logger *slog.Logger) workflows.ResearchFunc {
	logger = logger.With("module", "research")
	search := func(ctx context.Context, req autosearch.SearchRequest) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		res, err := engine.SearchAndGrab(ctx, req)
		if err != nil {
			logger.Warn("re-search after abandoned download failed",
				"media_type", req.MediaType, "media_id", req.MediaID, "title", req.Title, "error", err)
			return
		}
		if res != nil {
			logger.Info("re-search after abandoned download",
				"media_type", req.MediaType, "media_id", req.MediaID, "title", req.Title,
				"grabbed", res.Grabbed != nil, "reason", res.Reason)
		}
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}

	return func(ctx context.Context, mediaType string, mediaIDs []string) {
		switch mediaType {
		case "movie":
			if moviesSvc == nil {
				return
			}
			for _, id := range mediaIDs {
				m, err := moviesSvc.GetMovie(ctx, id)
				if err != nil || m == nil {
					continue
				}
				if m.MonitoringStatus != movies.MonitoringStatusMonitored {
					continue
				}
				search(ctx, autosearch.SearchRequest{
					MediaType:        "movie",
					MediaID:          m.ID,
					Title:            m.Title,
					Year:             m.Year,
					QualityProfileID: m.QualityProfileID,
					IMDBID:           deref(m.IMDBID),
					TMDBID:           deref(m.TMDBID),
					TVDBID:           deref(m.TVDBID),
				})
			}
		case "episode":
			if seriesSvc == nil {
				return
			}
			type seasonKey struct {
				seriesID string
				seasonID string
			}
			groups := make(map[seasonKey][]int)
			var order []seasonKey
			for _, id := range mediaIDs {
				ep, err := seriesSvc.GetEpisode(ctx, id)
				if err != nil || ep == nil || !ep.Monitored {
					continue
				}
				k := seasonKey{ep.SeriesID, ep.SeasonID}
				if _, ok := groups[k]; !ok {
					order = append(order, k)
				}
				groups[k] = append(groups[k], ep.EpisodeNumber)
			}
			for _, k := range order {
				sr, err := seriesSvc.GetSeries(ctx, k.seriesID)
				if err != nil || sr == nil {
					continue
				}
				seasons, err := seriesSvc.ListSeasons(ctx, k.seriesID)
				if err != nil {
					continue
				}
				seasonNum := -1
				for _, se := range seasons {
					if se.ID == k.seasonID {
						seasonNum = se.SeasonNumber
						break
					}
				}
				if seasonNum < 0 {
					continue
				}
				req := autosearch.SearchRequest{
					MediaType:        "series",
					MediaID:          sr.ID,
					Title:            sr.Title,
					Year:             sr.Year,
					QualityProfileID: sr.QualityProfileID,
					IMDBID:           deref(sr.IMDBID),
					TMDBID:           deref(sr.TMDBID),
					TVDBID:           deref(sr.TVDBID),
					Season:           seasonNum,
				}
				if eps := groups[k]; len(eps) == 1 {
					req.Episode = eps[0]
				}
				search(ctx, req)
			}
		}
	}
}
