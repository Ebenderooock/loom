package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ebenderooock/loom/internal/alttitles"
	"github.com/ebenderooock/loom/internal/anime"
	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/calendar"
	"github.com/ebenderooock/loom/internal/devmode"
	"github.com/ebenderooock/loom/internal/discover"
	"github.com/ebenderooock/loom/internal/importlists"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/languages"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/mediainfo"
	"github.com/ebenderooock/loom/internal/metadata/musicbrainz"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/music"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/scanner"
	"github.com/ebenderooock/loom/internal/scheduler"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
)

// mediaWiring holds services produced by wireMedia that downstream
// wire functions need.
type mediaWiring struct {
	libStore          *libraries.Store
	seriesSvc         series.Service
	qpStore           *qualityprofiles.Store
	notifSvc          notifications.Service
	importListSyncMgr *importlists.SyncManager
	musicRepo         music.Repository
	musicSvc          music.Service
}

// wireMedia constructs all media-related services (scanner, organizer,
// series, libraries, languages, quality profiles, etc.) and records
// them into wiring. It returns shared references that other wire
// functions depend on.
func wireMedia(
	ctx context.Context,
	cfg *config.Config,
	db storage.DB,
	wiring *server.Wiring,
	moviesSvc movies.Service,
	auditLogger *auditlog.Logger,
	logger *slog.Logger,
	stubs *devmode.Stubs,
) (*mediaWiring, error) {
	// Libraries store — needed by scanner, organizer, imports, health monitor.
	libStore := libraries.NewStore(db.DB())

	// Library scanner
	scannerSvc := buildScanner(moviesSvc, cfg, auditLogger, logger, stubs)
	wiring.ScannerService = scannerSvc

	// File organizer
	organizerSvc := buildOrganizer(moviesSvc, libStore, db, logger)
	if mode := cfg.MediaManagement.ImportMode; mode != "" {
		organizerSvc.SetImportMode(mode)
	}
	wiring.OrganizerService = organizerSvc

	// TV series
	seriesSvc := buildSeriesService(db, stubs)
	wiring.SeriesService = seriesSvc

	seriesScannerSvc := buildSeriesScanner(seriesSvc, logger)
	wiring.SeriesScanner = seriesScannerSvc

	// Music (artists/albums/tracks) — MusicBrainz metadata provider.
	mbCfg := musicbrainz.DefaultConfig()
	if stubs != nil {
		mbCfg.BaseURL = stubs.MusicBrainzURL
	}
	mbProvider := musicbrainz.NewProvider(musicbrainz.NewClient(mbCfg))
	musicRepo := music.NewRepository(db.DB())
	musicSvc := music.NewService(musicRepo, mbProvider, logger)
	wiring.MusicService = musicSvc

	musicScannerSvc := scanner.NewMusicScanner(musicSvc, logger)
	wiring.MusicScanner = musicScannerSvc

	// After a refresh re-creates a series' episodes (new IDs), re-link on-disk
	// files by triggering a per-series scan. Best-effort; never fails refresh.
	// Use a detached, bounded context so a client disconnect mid-refresh can't
	// leave the re-link half-applied (which would re-orphan episode files).
	seriesSvc.SetPostRefreshHook(func(ctx context.Context, seriesID string) {
		rescanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		sr, err := seriesSvc.GetSeries(rescanCtx, seriesID)
		if err != nil || sr.LibraryID == "" {
			return
		}
		lib, err := libStore.Get(rescanCtx, sr.LibraryID)
		if err != nil {
			return
		}
		if _, err := seriesScannerSvc.RescanSeries(rescanCtx, seriesID, lib.Path); err != nil {
			logger.Debug("post-refresh rescan failed", "series", seriesID, "error", err)
		}
	})

	// Periodic library scan (every 6 hours)
	periodicScanner := scheduler.NewPeriodicScanner(
		seriesScannerSvc,
		&libStoreProvider{store: libStore},
		6*time.Hour,
		logger,
	)
	wiring.PeriodicScanner = periodicScanner
	periodicScanner.Start(ctx)

	// Notifications
	notifSvc := buildNotificationsService(db)
	wiring.NotificationSvc = notifSvc

	// Calendar
	wiring.CalendarHandler = calendar.Router(db.DB())

	// Language profiles
	langStore := languages.NewStore(db.DB())
	if err := langStore.EnsureDefault(ctx); err != nil {
		return nil, fmt.Errorf("init language profiles: %w", err)
	}
	wiring.LanguageStore = langStore

	// Anime
	wiring.AnimeStore = anime.NewStore(db.DB(), logger)

	// Import lists
	importListStore := importlists.NewStore(db.DB())
	importListSyncMgr := importlists.NewSyncManager(importListStore, logger)
	importListSyncMgr.SetTMDBAPIKey(func() string {
		k := os.Getenv("LOOM_TMDB_API_KEY")
		if k == "" {
			k = defaultTMDBKey
		}
		return k
	}())
	importListSyncMgr.SetMoviesService(moviesSvc)
	importListSyncMgr.SetSeriesService(seriesSvc)
	importListSyncMgr.SetMusicService(musicSvc)
	importListSyncMgr.SetLibraryService(libStore)
	importListSyncMgr.SetTMDBClient(buildTMDBClient(stubs))
	wiring.ImportListStore = importListStore
	wiring.ImportListSync = importListSyncMgr

	// Libraries scanner
	libScanner := libraries.NewScanner(libStore, logger)
	wiring.LibraryStore = libStore
	wiring.LibraryScanner = libScanner

	// Media info
	wiring.MediaInfoStore = mediainfo.NewStore(db.DB(), logger)

	// Alt titles
	wiring.AltTitleStore = alttitles.NewStore(db.DB())

	// Quality profiles
	qpStore := qualityprofiles.NewStore(db.DB())
	wiring.QualityProfile = qpStore
	qualityprofiles.SeedDefaults(ctx, qpStore, moviesSvc)

	// Person discovery (TMDB-backed)
	discoverTMDB := buildTMDBClient(stubs)
	wiring.DiscoverRouter = discover.Router(discoverTMDB)

	return &mediaWiring{
		libStore:          libStore,
		seriesSvc:         seriesSvc,
		qpStore:           qpStore,
		notifSvc:          notifSvc,
		importListSyncMgr: importListSyncMgr,
		musicRepo:         musicRepo,
		musicSvc:          musicSvc,
	}, nil
}

// libStoreProvider adapts libraries.Store to scheduler.LibraryProvider.
type libStoreProvider struct {
	store *libraries.Store
}

func (p *libStoreProvider) ListAll(ctx context.Context) ([]scheduler.LibraryInfo, error) {
	libs, err := p.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.LibraryInfo, len(libs))
	for i, l := range libs {
		out[i] = scheduler.LibraryInfo{
			ID:        l.ID,
			Path:      l.Path,
			MediaType: l.MediaType,
		}
	}
	return out, nil
}
