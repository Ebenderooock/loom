// Package server wires the HTTP server: native /api/v1, wire-compat
// surfaces, /metrics, /healthz/readyz/livez, and (when built with the
// embed tag) the React app. Routing uses go-chi/chi/v5 with a standard
// middleware chain (request-ID, structured access log, panic recovery,
// gzip, ETag for system status, CORS).
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/loomctl/loom/internal/alttitles"
	"github.com/loomctl/loom/internal/anime"
	"github.com/loomctl/loom/internal/apikeys"
	"github.com/loomctl/loom/internal/appconfig"
	"github.com/loomctl/loom/internal/autosearch"
	"github.com/loomctl/loom/internal/commands"
	"github.com/loomctl/loom/internal/compat/prowlarrv1"
	"github.com/loomctl/loom/internal/compat/radarrv3"
	"github.com/loomctl/loom/internal/compat/sonarrv3"
	"github.com/loomctl/loom/internal/connect"
	"github.com/loomctl/loom/internal/episodeorder"
	"github.com/loomctl/loom/internal/auth"
	"github.com/loomctl/loom/internal/libraries"
	"github.com/loomctl/loom/internal/buildinfo"
	"github.com/loomctl/loom/internal/customformats"
	"github.com/loomctl/loom/internal/downloads"
	"github.com/loomctl/loom/internal/grabs"
	"github.com/loomctl/loom/internal/healthmonitor"
	"github.com/loomctl/loom/internal/importlists"
	"github.com/loomctl/loom/internal/imports"
	"github.com/loomctl/loom/internal/indexers"
	"github.com/loomctl/loom/internal/indexers/newznabserver"
	"github.com/loomctl/loom/internal/languages"
	"github.com/loomctl/loom/internal/mediainfo"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/eventbus"
	"github.com/loomctl/loom/internal/kernel/telemetry"
	"github.com/loomctl/loom/internal/movies"
	"github.com/loomctl/loom/internal/notifications"
	"github.com/loomctl/loom/internal/organizer"
	"github.com/loomctl/loom/internal/packs"
	"github.com/loomctl/loom/internal/qualityprofiles"
	"github.com/loomctl/loom/internal/rss"
	"github.com/loomctl/loom/internal/safety"
	"github.com/loomctl/loom/internal/scanner"
	"github.com/loomctl/loom/internal/scheduler"
	"github.com/loomctl/loom/internal/series"
	"github.com/loomctl/loom/internal/storage"
	"github.com/loomctl/loom/internal/validation"
)

// Server holds wired dependencies for the HTTP listener.
type Server struct {
	cfg        *config.Config
	appCfg     *appconfig.Config
	logger     *slog.Logger
	httpSrv    *http.Server
	tel        *telemetry.Telemetry
	db         storage.DB
	bus        eventbus.Bus
	authSvc    *auth.Service
	indexerSvc *indexers.Service
	downloadSvc *downloads.Service
	blocklistStore *downloads.BlocklistStore
	moviesSvc  movies.Service
	rssSvc     *rss.SourcesService
	scannerSvc *scanner.Scanner
	seriesScannerSvc *scanner.SeriesScanner
	organizerSvc *organizer.Organizer
	seriesSvc  series.Service
	notifSvc   notifications.Service
	connectSvc connect.Service
	reviewStore *safety.ReviewStore
	importPipeline *imports.ImportPipeline
	langStore   *languages.Store
	customFormatStore *customformats.Store
	rollingSearch  *scheduler.RollingSearcher
	aggSvc     *newznabserver.Server
	altTitleStore *alttitles.Store
	animeStore *anime.Store
	validator        *validation.Validator
	indexerRuleStore *indexers.RuleStore
	importListStore  *importlists.Store
	importListSync   *importlists.SyncManager
	mediaInfoStore *mediainfo.Store
	packsStore *packs.Store
	episodeOrderStore *episodeorder.Store
	libStore   *libraries.Store
	libScanner *libraries.Scanner
	apiKeyStore *apikeys.Store
	cmdQueue    *commands.Queue
	qpStore     *qualityprofiles.Store
	calendarHandler http.Handler
	compatRadarr    *radarrv3.Handler
	compatSonarr    *sonarrv3.Handler
	compatProwlarr  *prowlarrv1.Handler
	healthMonitor   *healthmonitor.Monitor
	autoSearchEngine *autosearch.Engine
	grabStore        *grabs.Store
	httpMetrics *telemetry.HTTPMetrics
	ready      atomic.Bool
}

// New constructs a Server but does not start listening. The caller must
// have already constructed *telemetry.Telemetry (typically via
// telemetry.Init in serve.go) and an open storage.DB (with migrations
// applied). The Server takes ownership of db and will Close() it on
// Shutdown. authSvc may be nil for low-level tests; production callers
// pass a fully wired *auth.Service. indexerSvc may be nil to disable
// the /api/v1/indexers/* surface. moviesSvc may be nil to disable the
// /api/v1/movies/* surface. aggSvc may be nil to disable the
// Newznab/Torznab aggregator at /api and /api/v1/aggregate.
func New(cfg *config.Config, appCfg *appconfig.Config, logger *slog.Logger, tel *telemetry.Telemetry, db storage.DB, authSvc *auth.Service, indexerSvc *indexers.Service, moviesSvc movies.Service, aggSvc *newznabserver.Server) (*Server, error) {
	if tel == nil {
		return nil, errors.New("server: telemetry must not be nil")
	}
	if db == nil {
		return nil, errors.New("server: db must not be nil")
	}
	if appCfg == nil {
		return nil, errors.New("server: appCfg must not be nil")
	}

	s := &Server{
		cfg:         cfg,
		appCfg:      appCfg,
		logger:      logger,
		tel:         tel,
		db:          db,
		bus:         eventbus.NewInProc(),
		authSvc:     authSvc,
		indexerSvc:  indexerSvc,
		moviesSvc:   moviesSvc,
		reviewStore: safety.NewReviewStore(db.DB()),
		aggSvc:      aggSvc,
		httpMetrics: telemetry.NewHTTPMetrics(tel.Registry()),
	}

	mux := s.newMux()

	s.httpSrv = &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(cfg.HTTP.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTP.WriteTimeout) * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	return s, nil
}

// SetDownloads installs the download-client service and rebuilds the
// HTTP handler so the new routes are reachable. Must be called before
// Start.
func (s *Server) SetDownloads(svc *downloads.Service) {
	s.downloadSvc = svc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetBlocklistStore installs the blocklist store. Must be called before
// Start so the blocklist API routes are mounted.
func (s *Server) SetBlocklistStore(store *downloads.BlocklistStore) {
	s.blocklistStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetMovies installs the movies service and rebuilds the HTTP handler
// so the new routes are reachable. Must be called before Start.
func (s *Server) SetMovies(svc movies.Service) {
	s.moviesSvc = svc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetScanner installs the scanner and rebuilds the HTTP handler.
func (s *Server) SetScanner(sc *scanner.Scanner) {
	s.scannerSvc = sc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetSeriesScanner installs the series scanner and rebuilds the HTTP handler.
func (s *Server) SetSeriesScanner(ss *scanner.SeriesScanner) {
	s.seriesScannerSvc = ss
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetOrganizer installs the file organizer and rebuilds the HTTP handler.
func (s *Server) SetOrganizer(org *organizer.Organizer) {
	s.organizerSvc = org
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetRSS installs the RSS sources service and rebuilds the HTTP handler
// so the new routes are reachable. Must be called before Start.
func (s *Server) SetRSS(svc *rss.SourcesService) {
	s.rssSvc = svc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetSeries installs the TV series service and rebuilds the HTTP handler.
func (s *Server) SetSeries(svc series.Service) {
	s.seriesSvc = svc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetNotifications installs the notification service and rebuilds the HTTP handler.
func (s *Server) SetNotifications(svc notifications.Service) {
	s.notifSvc = svc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetConnect installs the connect service and rebuilds the HTTP handler.
func (s *Server) SetConnect(svc connect.Service) {
	s.connectSvc = svc
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetCalendar installs the calendar handler and rebuilds the HTTP handler.
func (s *Server) SetCalendar(h http.Handler) {
	s.calendarHandler = h
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetRollingSearch installs the rolling-search scheduler and rebuilds the HTTP handler.
func (s *Server) SetRollingSearch(rs *scheduler.RollingSearcher) {
	s.rollingSearch = rs
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetImportPipeline installs the import pipeline and rebuilds the HTTP handler.
func (s *Server) SetImportPipeline(p *imports.ImportPipeline) {
	s.importPipeline = p
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetLanguages installs the language-profile store and rebuilds the HTTP handler.
func (s *Server) SetLanguages(store *languages.Store) {
	s.langStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetCustomFormats installs the custom formats store and rebuilds the HTTP handler.
func (s *Server) SetCustomFormats(store *customformats.Store) {
	s.customFormatStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetAltTitles installs the alt-title store and rebuilds the HTTP handler.
func (s *Server) SetAltTitles(store *alttitles.Store) {
	s.altTitleStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetAnime installs the anime store and rebuilds the HTTP handler.
func (s *Server) SetAnime(store *anime.Store) {
	s.animeStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetImportLists installs the import list store and sync manager and
// rebuilds the HTTP handler. Must be called before Start.
func (s *Server) SetImportLists(store *importlists.Store, syncMgr *importlists.SyncManager) {
	s.importListStore = store
	s.importListSync = syncMgr
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetMediaInfo installs the media-info store and rebuilds the HTTP handler.
func (s *Server) SetMediaInfo(store *mediainfo.Store) {
	s.mediaInfoStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetPacks installs the packs store and rebuilds the HTTP handler.
func (s *Server) SetPacks(store *packs.Store) {
	s.packsStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetEpisodeOrder installs the episode-order store and rebuilds the HTTP handler.
func (s *Server) SetEpisodeOrder(store *episodeorder.Store) {
	s.episodeOrderStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetValidator installs the file validator and rebuilds the HTTP handler.
func (s *Server) SetValidator(v *validation.Validator) {
	s.validator = v
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetIndexerRuleStore installs the indexer rule store and rebuilds the HTTP handler.
func (s *Server) SetIndexerRuleStore(store *indexers.RuleStore) {
	s.indexerRuleStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetLibraries installs the libraries store and scanner, then rebuilds the HTTP handler.
func (s *Server) SetLibraries(store *libraries.Store, scanner *libraries.Scanner) {
	s.libStore = store
	s.libScanner = scanner
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetAPIKeys installs the API key store and rebuilds the HTTP handler.
func (s *Server) SetAPIKeys(store *apikeys.Store) {
	s.apiKeyStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetCommands installs the command queue and rebuilds the HTTP handler.
func (s *Server) SetCommands(q *commands.Queue) {
	s.cmdQueue = q
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetQualityProfiles installs the quality profiles store and rebuilds the HTTP handler.
func (s *Server) SetQualityProfiles(store *qualityprofiles.Store) {
	s.qpStore = store
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetCompatRadarr installs the Radarr v3 API compatibility shim.
func (s *Server) SetCompatRadarr(h *radarrv3.Handler) {
	s.compatRadarr = h
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetCompatSonarr installs the Sonarr v3 API compatibility shim.
func (s *Server) SetCompatSonarr(h *sonarrv3.Handler) {
	s.compatSonarr = h
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetCompatProwlarr installs the Prowlarr v1 API compatibility shim.
func (s *Server) SetCompatProwlarr(h *prowlarrv1.Handler) {
	s.compatProwlarr = h
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetHealthMonitor installs the health monitor and rebuilds the HTTP
// handler so the /api/v1/system/health routes are reachable.
func (s *Server) SetHealthMonitor(m *healthmonitor.Monitor) {
	s.healthMonitor = m
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetAutoSearchEngine installs the autosearch engine and rebuilds the HTTP
// handler so the /api/v1/autosearch route is reachable.
func (s *Server) SetAutoSearchEngine(e *autosearch.Engine) {
	s.autoSearchEngine = e
	if s.httpSrv != nil {
		s.httpSrv.Handler = s.newMux()
	}
}

// SetGrabStore sets the active-grabs store for tracking download→media linkage.
func (s *Server) SetGrabStore(gs *grabs.Store) {
	s.grabStore = gs
}

// Bus returns the server's event bus for wiring pipelines.
func (s *Server) Bus() eventbus.Bus {
	return s.bus
}

func (s *Server) newMux() http.Handler {
	r := chi.NewRouter()

	// HTTP metrics middleware — registered early so it captures all requests.
	r.Use(s.httpMetrics.Handler)

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.accessLog)
	r.Use(s.recoverer)
	r.Use(middleware.Compress(5))

	if origins := s.cfg.CORS.AllowedOrigins; len(origins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   origins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Api-Key", "X-Request-Id"},
			ExposedHeaders:   []string{"X-Request-Id"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.ready.Load() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "starting"})
			return
		}
		if err := s.db.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unreachable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Handle("/metrics", s.tel.Handler())

	if s.authSvc != nil {
		s.authSvc.Mount(r)
	}

	// All application routes require auth (except the auth endpoints mounted above).
	// Public health/metrics routes are exceptions.
	r.Group(func(r chi.Router) {
		// Apply auth to all routes in this group (RequireAuth is a no-op when auth is disabled)
		if s.authSvc != nil {
			r.Use(s.authSvc.RequireAuth)
		}

		// Indexer routes
		if s.indexerSvc != nil {
			s.indexerSvc.Mount(r)
		}

		// Download-client routes
		if s.downloadSvc != nil {
			s.downloadSvc.Mount(r)
		}

		// Blocklist routes
		if s.blocklistStore != nil {
			downloads.MountBlocklist(r, s.blocklistStore)
		}

		// RSS sources routes
		if s.rssSvc != nil {
			s.rssSvc.Mount(r)
		}

		// Movies routes
		if s.moviesSvc != nil {
			moviesRouter := movies.RouterWithSearch(s.moviesSvc, s.indexerSvc)
			if s.scannerSvc != nil {
				scanner.RegisterRoutes(moviesRouter, s.scannerSvc, s.libStore)
			}
			if s.organizerSvc != nil {
				organizer.RegisterRoutes(moviesRouter, s.organizerSvc)
			}
			r.Mount("/api/v1/movies", moviesRouter)
		}

		// Series (TV Shows) routes
		if s.seriesSvc != nil {
			seriesRouter := series.RouterWithSearch(s.seriesSvc, s.indexerSvc, s.grabStore)
			if s.seriesScannerSvc != nil {
				scanner.RegisterSeriesRoutes(seriesRouter, s.seriesScannerSvc)
			}
			r.Mount("/api/v1/series", seriesRouter)
		}

		// Notifications routes
		if s.notifSvc != nil {
			r.Mount("/api/v1/notifications", notifications.Router(s.notifSvc))
		}

		// Connect (media server integrations) routes
		if s.connectSvc != nil {
			r.Mount("/api/v1/connect", connect.Router(s.connectSvc))
		}

		// Calendar routes
		if s.calendarHandler != nil {
			r.Mount("/api/v1/calendar", s.calendarHandler)
		}

		// Manual review routes (download safety)
		r.Mount("/api/v1/reviews", safety.Router(s.reviewStore))

		// Rolling search routes
		if s.rollingSearch != nil {
			r.Mount("/api/v1/rolling-search", scheduler.Router(s.rollingSearch))
		}

		// Import pipeline routes
		if s.importPipeline != nil {
			r.Mount("/api/v1/imports", imports.Router(s.importPipeline))
		}

		// Custom formats routes
		if s.customFormatStore != nil {
			r.Mount("/api/v1/custom-formats", customformats.Router(s.customFormatStore, s.logger))
		}

		// Language profile routes
		if s.langStore != nil {
			languages.Mount(r, s.langStore)
		}

		// Alt-title routes
		if s.altTitleStore != nil {
			r.Mount("/api/v1/alt-titles", alttitles.Router(s.altTitleStore))
		}

		// Anime routes
		if s.animeStore != nil {
			r.Mount("/api/v1/anime", anime.Router(s.animeStore))
		}

		// Import lists routes
		if s.importListStore != nil && s.importListSync != nil {
			r.Mount("/api/v1/import-lists", importlists.Router(s.importListStore, s.importListSync, s.logger))
		}

		// Media info / preferences routes
		if s.mediaInfoStore != nil {
			r.Mount("/api/v1/media-info", mediainfo.Router(s.mediaInfoStore, s.logger))
		}

		// Packs routes
		if s.packsStore != nil {
			r.Mount("/api/v1/packs", packs.Router(s.packsStore))
		}

		// Episode ordering routes
		if s.episodeOrderStore != nil {
			r.Mount("/api/v1/episode-order", episodeorder.Router(s.episodeOrderStore))
		}

		// Validation rules routes
		if s.validator != nil {
			r.Mount("/api/v1/validation", validation.Router(s.validator))
		}

		// Indexer rules and Jackett import routes
		if s.indexerRuleStore != nil && s.indexerSvc != nil {
			indexers.MountRuleRoutes(r, s.indexerRuleStore, s.indexerSvc)
		}

		// Library routes
		if s.libStore != nil {
			r.Mount("/api/v1/libraries", libraries.Router(s.libStore, s.libScanner, s.logger))
		}

		// API key management routes
		if s.apiKeyStore != nil {
			r.Mount("/api/v1/api-keys", apikeys.Router(s.apiKeyStore, s.logger))
		}

		// Command queue routes
		if s.cmdQueue != nil {
			r.Mount("/api/v1/command", commands.Router(s.cmdQueue, s.logger))
		}

		// Quality profile routes
		if s.qpStore != nil {
			r.Mount("/api/v1/quality-profiles", qualityprofiles.Router(s.qpStore, s.logger))
		}

		// System status (authenticated)
		r.Group(func(r chi.Router) {
			r.Use(etagMiddleware)
			r.Get("/api/v1/system/status", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, map[string]any{
					"version":   buildinfo.Version,
					"commit":    buildinfo.Commit,
					"buildDate": buildinfo.Date,
					"engine":    string(s.db.Engine()),
				})
			})
		})

		// System health monitoring (authenticated)
		if s.healthMonitor != nil {
			r.Mount("/api/v1/system/health", healthmonitor.Router(s.healthMonitor))
		}

		// Automated search + grab (authenticated)
		if s.autoSearchEngine != nil {
			asHandler := autosearch.NewHandler(s.autoSearchEngine, s.logger)
			r.Post("/api/v1/autosearch", asHandler.HandleAutoSearch)
		}

		// Filesystem browsing (authenticated)
		r.Get("/api/v1/filesystem", handleFilesystemBrowse())
	})

	// Newznab/Torznab aggregator. Mounted OUTSIDE the auth group because clients 
	// (Sonarr, Radarr, Prowlarr) supply credentials via the ?apikey= query param and
	// expect Newznab XML errors, not JSON.
	if s.aggSvc != nil {
		s.aggSvc.Mount(r)
	}

	// *arr compatibility shims — lets Overseerr, Ombi, etc. talk to Loom.
	if s.compatRadarr != nil {
		r.Mount("/compat/radarr/api/v3", radarrv3.Router(s.compatRadarr))
	}
	if s.compatSonarr != nil {
		r.Mount("/compat/sonarr/api/v3", sonarrv3.Router(s.compatSonarr))
	}
	if s.compatProwlarr != nil {
		r.Mount("/compat/prowlarr/api/v1", prowlarrv1.Router(s.compatProwlarr))
	}

	if s.cfg.Debug.Pprof {
		s.mountPprof(r)
	}

	// Embedded SPA catch-all — serves the React frontend when built with
	// `-tags embed`. In dev mode spaHandler() returns nil and the Vite dev
	// server on :5173 handles the UI.
	if spa := spaHandler(); spa != nil {
		r.NotFound(spa.ServeHTTP)
	}

	return r
}

func (s *Server) mountPprof(r chi.Router) {
	r.Route("/debug/pprof", func(r chi.Router) {
		r.Get("/", pprof.Index)
		r.Get("/cmdline", pprof.Cmdline)
		r.Get("/profile", pprof.Profile)
		r.Post("/symbol", pprof.Symbol)
		r.Get("/symbol", pprof.Symbol)
		r.Get("/trace", pprof.Trace)
		r.Get("/{name}", func(w http.ResponseWriter, req *http.Request) {
			pprof.Handler(chi.URLParam(req, "name")).ServeHTTP(w, req)
		})
	})
}

// accessLog emits a structured slog record per request and propagates the
// chi-generated X-Request-Id back to the client.
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		reqID := middleware.GetReqID(r.Context())
		if reqID != "" {
			ww.Header().Set("X-Request-Id", reqID)
		}
		next.ServeHTTP(ww, r)
		s.logger.LogAttrs(r.Context(), slog.LevelInfo, "http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", ww.Status()),
			slog.Int("bytes", ww.BytesWritten()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("remote", r.RemoteAddr),
			slog.String("request_id", reqID),
		)
	})
}

// recoverer turns panics into a structured 500 JSON response and a stack
// trace logged at error level.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				if rv == http.ErrAbortHandler {
					panic(rv)
				}
				stack := debug.Stack()
				s.logger.Error("panic",
					"err", fmt.Sprintf("%v", rv),
					"path", r.URL.Path,
					"method", r.Method,
					"request_id", middleware.GetReqID(r.Context()),
					"stack", string(stack),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// etagMiddleware buffers the response, hashes it, sets ETag, and replies
// with 304 if the client's If-None-Match matches. Designed for tiny GET
// endpoints (e.g. system status); not appropriate for large or streaming
// responses.
func etagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		buf := &etagWriter{ResponseWriter: w, body: &bytes.Buffer{}, status: http.StatusOK}
		next.ServeHTTP(buf, r)

		sum := sha256.Sum256(buf.body.Bytes())
		etag := `"` + hex.EncodeToString(sum[:16]) + `"`
		w.Header().Set("ETag", etag)
		if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(buf.status)
		_, _ = w.Write(buf.body.Bytes())
	})
}

type etagWriter struct {
	http.ResponseWriter
	body        *bytes.Buffer
	status      int
	wroteHeader bool
}

func (e *etagWriter) WriteHeader(code int) {
	if !e.wroteHeader {
		e.status = code
		e.wroteHeader = true
	}
}

func (e *etagWriter) Write(b []byte) (int, error) {
	if !e.wroteHeader {
		e.WriteHeader(http.StatusOK)
	}
	return e.body.Write(b)
}

// Start begins serving and blocks until ListenAndServe returns.
func (s *Server) Start() error {
	s.ready.Store(true)
	s.logger.Info("listening", "addr", s.cfg.HTTP.Addr)
	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the listener. The DB and Telemetry are owned by the
// caller (serve.go) and shut down separately.
func (s *Server) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	return s.httpSrv.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleFilesystemBrowse returns directories at a given path for the
// folder-picker dialog. Query params: ?path=/some/dir
// Returns { parent, directories[] } where each directory has name + path.
func handleFilesystemBrowse() http.HandlerFunc {
	type dirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		reqPath := r.URL.Query().Get("path")

		// Default to filesystem roots when no path given
		if reqPath == "" {
			if runtime.GOOS == "windows" {
				// List common drive letters
				var drives []dirEntry
				for _, letter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
					drive := string(letter) + `:\`
					if _, err := os.Stat(drive); err == nil {
						drives = append(drives, dirEntry{Name: drive, Path: drive})
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"parent":      "",
					"current":     "",
					"directories": drives,
				})
				return
			}
			reqPath = "/"
		}

		// Clean and resolve
		reqPath = filepath.Clean(reqPath)

		info, err := os.Stat(reqPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "path does not exist: " + reqPath,
			})
			return
		}
		if !info.IsDir() {
			reqPath = filepath.Dir(reqPath)
		}

		entries, err := os.ReadDir(reqPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "cannot read directory: " + err.Error(),
			})
			return
		}

		dirs := make([]dirEntry, 0)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			// Skip hidden directories
			if strings.HasPrefix(name, ".") {
				continue
			}
			dirs = append(dirs, dirEntry{
				Name: name,
				Path: filepath.Join(reqPath, name),
			})
		}
		sort.Slice(dirs, func(i, j int) bool {
			return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
		})

		parent := filepath.Dir(reqPath)
		if parent == reqPath {
			parent = "" // at root
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"parent":      parent,
			"current":     reqPath,
			"directories": dirs,
		})
	}
}
