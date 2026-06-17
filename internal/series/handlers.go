package series

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/libraries"
)

// GrabChecker checks which media IDs have active workflows/grabs.
type GrabChecker interface {
	ActiveMediaIDs(ctx context.Context, mediaType string, ids []string) (map[string]bool, error)
}

// UnmonitorOnDeleteChecker checks if a library has unmonitor-on-delete enabled.
type UnmonitorOnDeleteChecker interface {
	ShouldUnmonitorOnDelete(ctx context.Context, libraryID string) bool
}

// seriesRouterConfig holds optional dependencies for the series router.
type seriesRouterConfig struct {
	unmonitorChecker UnmonitorOnDeleteChecker
	libraryStore     interface {
		List(ctx context.Context) ([]libraries.Library, error)
	}
	libraryScanner interface {
		ScanLibrary(ctx context.Context, lib *libraries.Library) error
	}
}

// SeriesRouterOption configures the series router.
type SeriesRouterOption func(*seriesRouterConfig)

// WithUnmonitorChecker provides an unmonitor-on-delete checker.
func WithUnmonitorChecker(c UnmonitorOnDeleteChecker) SeriesRouterOption {
	return func(cfg *seriesRouterConfig) { cfg.unmonitorChecker = c }
}

// WithLibraryRescan enables page-level rescan of all series libraries.
func WithLibraryRescan(
	store interface {
		List(ctx context.Context) ([]libraries.Library, error)
	},
	scanner interface {
		ScanLibrary(ctx context.Context, lib *libraries.Library) error
	},
) SeriesRouterOption {
	return func(cfg *seriesRouterConfig) {
		cfg.libraryStore = store
		cfg.libraryScanner = scanner
	}
}

// Router mounts series endpoints on a chi router.
func Router(svc Service) chi.Router {
	return RouterWithSearch(svc, nil, nil)
}

// RouterWithSearch mounts series endpoints with optional search-on-add support.
func RouterWithSearch(svc Service, indexerSvc *indexers.Service, grabStore GrabChecker, opts ...SeriesRouterOption) chi.Router {
	var cfg seriesRouterConfig
	for _, o := range opts {
		o(&cfg)
	}

	r := chi.NewRouter()

	// Literal routes first (before /{id} wildcard)
	r.Get("/", listSeries(svc))
	r.Post("/", addSeries(svc, indexerSvc))
	r.Get("/search", searchSeries(svc))
	r.Get("/lookup", lookupSeries(svc))

	// Bulk operations (must be before /{id} wildcard)
	r.Post("/bulk", bulkUpdateSeries(svc))
	r.Post("/bulk-archive", bulkArchiveSeries(svc))
	r.Post("/bulk-unarchive", bulkUnarchiveSeries(svc))
	r.Post("/refresh", refreshAllSeries(svc))
	if cfg.libraryStore != nil && cfg.libraryScanner != nil {
		r.Post("/rescan", rescanAllSeriesLibraries(cfg.libraryStore, cfg.libraryScanner))
	}

	// Wildcard routes
	r.Get("/{id}", getSeries(svc))
	r.Put("/{id}", updateSeries(svc))
	r.Delete("/{id}", deleteSeries(svc, cfg.unmonitorChecker))
	r.Put("/{id}/monitoring", setMonitoringStatus(svc))
	r.Post("/{id}/refresh", refreshSeries(svc))
	r.Post("/{id}/archive", archiveSeries(svc))
	r.Post("/{id}/unarchive", unarchiveSeries(svc))
	r.Get("/{id}/credits", getCredits(svc))
	r.Get("/{id}/seasons", listSeasons(svc))
	r.Get("/{id}/seasons/{seasonNum}/episodes", listEpisodesHandler(svc, grabStore))

	return r
}

func seriesToResponse(s *Series) map[string]interface{} {
	resp := map[string]interface{}{
		"id":               s.ID,
		"title":            s.Title,
		"year":             s.Year,
		"imdbId":           s.IMDBID,
		"tmdbId":           s.TMDBID,
		"tvdbId":           s.TVDBID,
		"overview":         s.Overview,
		"genres":           s.Genres,
		"runtime":          s.Runtime,
		"rating":           s.Rating,
		"backdropPath":     s.BackdropPath,
		"posterPath":       s.PosterPath,
		"network":          s.Network,
		"status":           string(s.Status),
		"seriesType":       string(s.SeriesType),
		"metadataProvider": s.MetadataProvider,
		"qualityProfileId": s.QualityProfileID,
		"libraryId":        s.LibraryID,
		"monitoringStatus": string(s.MonitoringStatus),
		"seasonFolder":     s.SeasonFolder,
		"releaseDate":      s.ReleaseDate,
		"createdAt":        s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updatedAt":        s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if s.Seasons != nil {
		seasons := make([]map[string]interface{}, 0, len(s.Seasons))
		for _, sn := range s.Seasons {
			seasons = append(seasons, seasonToResponse(sn))
		}
		resp["seasons"] = seasons
	}
	if s.Episodes != nil {
		episodes := make([]map[string]interface{}, 0, len(s.Episodes))
		for _, ep := range s.Episodes {
			episodes = append(episodes, episodeToResponse(ep))
		}
		resp["episodes"] = episodes
	}
	return resp
}

func seasonToResponse(s *Season) map[string]interface{} {
	return map[string]interface{}{
		"id":           s.ID,
		"seriesId":     s.SeriesID,
		"seasonNumber": s.SeasonNumber,
		"title":        s.Title,
		"overview":     s.Overview,
		"posterPath":   s.PosterPath,
		"monitored":    s.Monitored,
		"episodeCount": s.EpisodeCount,
		"createdAt":    s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updatedAt":    s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func refreshAllSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListSeries(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ids := make([]string, 0, len(list))
		for _, item := range list {
			ids = append(ids, item.ID)
		}

		go func(seriesIDs []string) {
			ctx := context.Background()
			for _, id := range seriesIDs {
				if err := svc.RefreshSeries(ctx, id); err != nil {
					slog.Warn("series: bulk refresh failed", "series_id", id, "error", err)
				}
			}
		}(ids)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "series refresh started",
			"count":   len(ids),
		})
	}
}

func rescanAllSeriesLibraries(
	store interface {
		List(ctx context.Context) ([]libraries.Library, error)
	},
	scanner interface {
		ScanLibrary(ctx context.Context, lib *libraries.Library) error
	},
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		librariesList, err := store.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		seriesLibraries := make([]libraries.Library, 0, len(librariesList))
		for _, lib := range librariesList {
			if lib.MediaType == "series" {
				seriesLibraries = append(seriesLibraries, lib)
			}
		}

		go func(libs []libraries.Library) {
			ctx := context.Background()
			for _, lib := range libs {
				lib := lib
				if err := scanner.ScanLibrary(ctx, &lib); err != nil {
					slog.Warn("series: bulk rescan failed", "library_id", lib.ID, "error", err)
				}
			}
		}(seriesLibraries)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message":      "series library rescan started",
			"libraryCount": len(seriesLibraries),
		})
	}
}

func episodeToResponse(e *Episode) map[string]interface{} {
	return map[string]interface{}{
		"id":            e.ID,
		"seriesId":      e.SeriesID,
		"seasonId":      e.SeasonID,
		"episodeNumber": e.EpisodeNumber,
		"title":         e.Title,
		"overview":      e.Overview,
		"airDate":       e.AirDate,
		"runtime":       e.Runtime,
		"stillPath":     e.StillPath,
		"monitored":     e.Monitored,
		"hasFile":       e.HasFile,
		"createdAt":     e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updatedAt":     e.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func listSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListSeries(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// In-memory filtering
		q := r.URL.Query()
		if search := q.Get("search"); search != "" {
			search = strings.ToLower(search)
			filtered := list[:0]
			for _, s := range list {
				if strings.Contains(strings.ToLower(s.Title), search) {
					filtered = append(filtered, s)
				}
			}
			list = filtered
		}
		if status := q.Get("status"); status != "" {
			filtered := list[:0]
			for _, s := range list {
				if string(s.Status) == status {
					filtered = append(filtered, s)
				}
			}
			list = filtered
		}
		if mon := q.Get("monitored"); mon != "" {
			filtered := list[:0]
			for _, s := range list {
				if mon == "true" && s.MonitoringStatus == MonitoringMonitored {
					filtered = append(filtered, s)
				} else if mon == "false" && s.MonitoringStatus != MonitoringMonitored {
					filtered = append(filtered, s)
				}
			}
			list = filtered
		}

		// Sorting
		sortField := q.Get("sort")
		sortOrder := q.Get("order")
		if sortField != "" {
			sort.Slice(list, func(i, j int) bool {
				var less bool
				switch sortField {
				case "title":
					less = strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title)
				case "year":
					less = list[i].Year < list[j].Year
				case "added":
					less = list[i].CreatedAt.Before(list[j].CreatedAt)
				case "network":
					less = strings.ToLower(list[i].Network) < strings.ToLower(list[j].Network)
				case "rating":
					less = list[i].Rating < list[j].Rating
				default:
					less = strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title)
				}
				if sortOrder == "desc" {
					return !less
				}
				return less
			})
		}

		allStats, err := svc.GetAllEpisodeStats(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]interface{}, 0, len(list))
		for _, s := range list {
			resp := seriesToResponse(s)
			if stats, ok := allStats[s.ID]; ok {
				resp["episodeStats"] = stats
			} else {
				resp["episodeStats"] = &EpisodeStats{}
			}
			response = append(response, resp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":  response,
			"total": len(list),
		})
	}
}

func addSeries(svc Service, indexerSvc *indexers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddSeriesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.TMDBID == "" {
			http.Error(w, "tmdb_id required", http.StatusBadRequest)
			return
		}

		sr, err := svc.AddSeries(r.Context(), &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fire async indexer search if requested
		if req.Search && indexerSvc != nil {
			go fireSeriesSearch(sr, indexerSvc)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(seriesToResponse(sr))
	}
}

// fireSeriesSearch runs an indexer search for a series in the background.
func fireSeriesSearch(series *Series, indexerSvc *indexers.Service) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	q := indexers.Query{
		Term:       series.Title,
		Categories: []indexers.Category{indexers.CategoryTV},
	}
	if series.TVDBID != nil && *series.TVDBID != "" {
		q.TVDBID = *series.TVDBID
	}
	if series.TMDBID != nil && *series.TMDBID != "" {
		q.TMDBID = *series.TMDBID
	}

	result := indexerSvc.Search(ctx, q, nil, 120*time.Second)
	if len(result.Errors) > 0 {
		slog.Warn("search-on-add had errors for series",
			"series", series.Title, "errors", result.Errors)
	}
}

func searchSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "search query required", http.StatusBadRequest)
			return
		}

		results, err := svc.SearchTMDB(r.Context(), query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Transform raw TMDB results to camelCase frontend shape
		mapped := make([]map[string]interface{}, 0, len(results))
		for _, r := range results {
			item := map[string]interface{}{
				"title":      getString(r, "name"),
				"overview":   getString(r, "overview"),
				"posterPath": getString(r, "poster_path"),
				"network":    "",
				"status":     "",
			}
			if id, ok := r["id"]; ok {
				item["tmdbId"] = fmt.Sprintf("%v", id)
			}
			if fad := getString(r, "first_air_date"); len(fad) >= 4 {
				y, _ := strconv.Atoi(fad[:4])
				item["year"] = y
			}
			if nets, ok := r["origin_country"].([]interface{}); ok && len(nets) > 0 {
				item["network"] = fmt.Sprintf("%v", nets[0])
			}
			mapped = append(mapped, item)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": mapped,
		})
	}
}

func lookupSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmdbID := r.URL.Query().Get("tmdbId")
		if tmdbID == "" {
			http.Error(w, "tmdbId required", http.StatusBadRequest)
			return
		}

		result, err := svc.LookupTMDB(r.Context(), tmdbID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func getSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		sr, err := svc.GetSeries(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if sr == nil {
			http.Error(w, "series not found", http.StatusNotFound)
			return
		}

		resp := seriesToResponse(sr)
		if stats, err := svc.GetEpisodeStats(r.Context(), id); err == nil {
			resp["episodeStats"] = stats
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func updateSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		sr, err := svc.GetSeries(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if sr == nil {
			http.Error(w, "series not found", http.StatusNotFound)
			return
		}

		var req UpdateSeriesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Title != nil {
			sr.Title = *req.Title
		}
		if req.Year != nil {
			sr.Year = *req.Year
		}
		if req.Overview != nil {
			sr.Overview = *req.Overview
		}
		if req.Genres != nil {
			sr.Genres = req.Genres
		}
		if req.Runtime != nil {
			sr.Runtime = *req.Runtime
		}
		if req.Rating != nil {
			sr.Rating = *req.Rating
		}
		if req.BackdropPath != nil {
			sr.BackdropPath = *req.BackdropPath
		}
		if req.PosterPath != nil {
			sr.PosterPath = *req.PosterPath
		}
		if req.Network != nil {
			sr.Network = *req.Network
		}
		if req.Status != nil {
			sr.Status = SeriesStatus(*req.Status)
		}
		if req.SeriesType != nil {
			sr.SeriesType = SeriesType(*req.SeriesType)
		}
		if req.MonitoringStatus != nil {
			sr.MonitoringStatus = MonitoringStatus(*req.MonitoringStatus)
		}
		if req.QualityProfileID != nil {
			sr.QualityProfileID = *req.QualityProfileID
		}
		if req.LibraryID != nil {
			sr.LibraryID = *req.LibraryID
		}
		if req.SeasonFolder != nil {
			sr.SeasonFolder = *req.SeasonFolder
		}

		if err := svc.UpdateSeries(r.Context(), sr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(seriesToResponse(sr))
	}
}

func deleteSeries(svc Service, checker UnmonitorOnDeleteChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		if checker != nil {
			if sr, err := svc.GetSeries(r.Context(), id); err == nil && sr != nil && sr.LibraryID != "" {
				if checker.ShouldUnmonitorOnDelete(r.Context(), sr.LibraryID) {
					_ = svc.SetMonitoringStatus(r.Context(), id, MonitoringUnmonitored)
				}
			}
		}

		if err := svc.DeleteSeries(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func setMonitoringStatus(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		var req SetMonitoringStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := svc.SetMonitoringStatus(r.Context(), id, MonitoringStatus(req.Status)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sr, _ := svc.GetSeries(r.Context(), id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(seriesToResponse(sr))
	}
}

func refreshSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		if err := svc.RefreshSeries(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}

func creditToResponse(c *SeriesCredit) map[string]interface{} {
	return map[string]interface{}{
		"id":           c.TMDBPersonID,
		"name":         c.PersonName,
		"character":    c.CharacterName,
		"role":         c.Role,
		"profile_path": c.ProfilePath,
		"order":        c.DisplayOrder,
	}
}

func getCredits(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		credits, err := svc.GetCredits(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cast := make([]map[string]interface{}, 0)
		crew := make([]map[string]interface{}, 0)
		for _, c := range credits {
			resp := creditToResponse(c)
			if c.Role == "actor" {
				cast = append(cast, resp)
			} else {
				crew = append(crew, resp)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cast": cast,
			"crew": crew,
		})
	}
}

func listSeasons(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		seasons, err := svc.ListSeasons(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fetch per-season episode stats
		seasonStats, _ := svc.GetSeasonEpisodeStats(r.Context(), id)

		response := make([]interface{}, 0, len(seasons))
		for _, s := range seasons {
			resp := seasonToResponse(s)
			if stats, ok := seasonStats[s.ID]; ok {
				resp["episodeStats"] = stats
			}
			response = append(response, resp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": response,
		})
	}
}

func listEpisodesHandler(svc Service, grabStore GrabChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}

		seasonNumStr := chi.URLParam(r, "seasonNum")
		var seasonNum *int
		if seasonNumStr != "" {
			n, err := strconv.Atoi(seasonNumStr)
			if err != nil {
				http.Error(w, "invalid season number", http.StatusBadRequest)
				return
			}
			seasonNum = &n
		}

		episodes, err := svc.ListEpisodes(r.Context(), id, seasonNum)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Look up which episodes have active grabs
		var grabbedSet map[string]bool
		if grabStore != nil && len(episodes) > 0 {
			epIDs := make([]string, len(episodes))
			for i, ep := range episodes {
				epIDs[i] = ep.ID
			}
			grabbedSet, _ = grabStore.ActiveMediaIDs(r.Context(), "episode", epIDs)
		}

		response := make([]interface{}, 0, len(episodes))
		for _, ep := range episodes {
			m := episodeToResponse(ep)
			if grabbedSet[ep.ID] {
				m["grabbed"] = true
			}
			response = append(response, m)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": response,
		})
	}
}

func archiveSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}
		if err := svc.SetMonitoringStatus(r.Context(), id, MonitoringArchived); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sr, _ := svc.GetSeries(r.Context(), id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(seriesToResponse(sr))
	}
}

func unarchiveSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "series ID required", http.StatusBadRequest)
			return
		}
		if err := svc.SetMonitoringStatus(r.Context(), id, MonitoringMonitored); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sr, _ := svc.GetSeries(r.Context(), id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(seriesToResponse(sr))
	}
}
