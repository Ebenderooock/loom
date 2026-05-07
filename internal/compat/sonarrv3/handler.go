package sonarrv3

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/series"
)

// Handler serves the Sonarr v3 compatibility API.
type Handler struct {
	svc    series.Service
	libs   *libraries.Store
	qp     *qualityprofiles.Store
	logger *slog.Logger
	ids    *idCache
}

// NewHandler creates a Handler wired to the given services.
func NewHandler(svc series.Service, libs *libraries.Store, qp *qualityprofiles.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		svc:    svc,
		libs:   libs,
		qp:     qp,
		logger: logger.With("compat", "sonarr/v3"),
		ids:    newIDCache(),
	}
}

// Router returns a chi.Router with all Sonarr v3 endpoints mounted.
func Router(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Route("/api/v3", func(r chi.Router) {
		// Series CRUD
		r.Get("/series", h.listSeries)
		r.Post("/series", h.addSeries)
		r.Get("/series/lookup", h.lookupSeries)
		r.Get("/series/{id}", h.getSeries)
		r.Put("/series/{id}", h.updateSeries)
		r.Delete("/series/{id}", h.deleteSeries)

		// Episodes
		r.Get("/episode", h.listEpisodes)
		r.Get("/episode/{id}", h.getEpisode)

		// Supporting endpoints
		r.Get("/rootfolder", h.listRootFolders)
		r.Get("/qualityprofile", h.listQualityProfiles)
		r.Get("/languageprofile", h.listLanguageProfiles)
		r.Get("/tag", h.listTags)

		// Commands
		r.Get("/command", h.listCommands)
		r.Post("/command", h.postCommand)

		// System
		r.Get("/system/status", h.systemStatus)
	})

	return r
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

// tvLibs returns libraries filtered to media_type "tv".
func (h *Handler) tvLibs(ctx context.Context) ([]libraries.Library, error) {
	all, err := h.libs.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]libraries.Library, 0)
	for _, l := range all {
		if l.MediaType == "tv" {
			out = append(out, l)
		}
	}
	return out, nil
}

// --- series handlers ---

func (h *Handler) listSeries(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListSeries(r.Context())
	if err != nil {
		h.logger.Error("list series", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	libs, _ := h.tvLibs(r.Context())

	out := make([]sonarrSeries, 0, len(list))
	for _, s := range list {
		out = append(out, seriesToSonarr(s, h.ids, libs))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getSeries(w http.ResponseWriter, r *http.Request) {
	numID, err := intIDStr(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	uuid, ok := h.ids.toStr(numID)
	if !ok {
		// Populate cache and retry.
		if _, lerr := h.svc.ListSeries(r.Context()); lerr == nil {
			// Trigger cache fill via list+convert.
			h.fillSeriesCache(r.Context())
		}
		uuid, ok = h.ids.toStr(numID)
		if !ok {
			writeError(w, http.StatusNotFound, "series not found")
			return
		}
	}

	s, err := h.svc.GetSeries(r.Context(), uuid)
	if err != nil {
		h.logger.Error("get series", "err", err)
		writeError(w, http.StatusNotFound, "series not found")
		return
	}

	libs, _ := h.tvLibs(r.Context())
	writeJSON(w, http.StatusOK, seriesToSonarr(s, h.ids, libs))
}

func (h *Handler) addSeries(w http.ResponseWriter, r *http.Request) {
	var req sonarrAddSeriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Resolve quality profile UUID from numeric ID.
	qpID := ""
	if req.QualityProfileID > 0 {
		if s, ok := h.ids.toStr(req.QualityProfileID); ok {
			qpID = s
		}
	}

	// Resolve library ID from root folder path.
	libID := ""
	if req.RootFolderPath != "" {
		libs, _ := h.tvLibs(r.Context())
		for _, lib := range libs {
			if lib.Path == req.RootFolderPath {
				libID = lib.ID
				break
			}
		}
	}

	// Resolve TVDB ID to TMDB ID by searching.
	tmdbID := ""
	if req.TvdbID > 0 {
		tmdbID = strconv.Itoa(req.TvdbID)
	}

	search := false
	if req.AddOptions != nil {
		search = req.AddOptions.SearchForMissingEpisodes
	}

	addReq := &series.AddSeriesRequest{
		TMDBID:           tmdbID,
		QualityProfileID: qpID,
		LibraryID:        libID,
		SeriesType:       req.SeriesType,
		SeasonFolder:     req.SeasonFolder,
		Search:           search,
	}

	if req.Monitored {
		addReq.MonitoringStatus = string(series.MonitoringAll)
	} else {
		addReq.MonitoringStatus = string(series.MonitoringNone)
	}

	created, err := h.svc.AddSeries(r.Context(), addReq)
	if err != nil {
		h.logger.Error("add series", "err", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to add series: %v", err))
		return
	}

	libs, _ := h.tvLibs(r.Context())
	writeJSON(w, http.StatusCreated, seriesToSonarr(created, h.ids, libs))
}

func (h *Handler) updateSeries(w http.ResponseWriter, r *http.Request) {
	numID, err := intIDStr(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	uuid, ok := h.ids.toStr(numID)
	if !ok {
		h.fillSeriesCache(r.Context())
		uuid, ok = h.ids.toStr(numID)
		if !ok {
			writeError(w, http.StatusNotFound, "series not found")
			return
		}
	}

	// Decode the incoming Sonarr-shaped update.
	var body sonarrSeries
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Fetch the existing series and apply changes.
	existing, err := h.svc.GetSeries(r.Context(), uuid)
	if err != nil {
		writeError(w, http.StatusNotFound, "series not found")
		return
	}

	existing.Title = body.Title
	existing.Year = body.Year
	existing.Overview = body.Overview
	existing.SeasonFolder = body.SeasonFolder
	existing.SeriesType = series.SeriesType(body.SeriesType)

	if body.Monitored {
		existing.MonitoringStatus = series.MonitoringAll
	} else {
		existing.MonitoringStatus = series.MonitoringNone
	}

	if err := h.svc.UpdateSeries(r.Context(), existing); err != nil {
		h.logger.Error("update series", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update series")
		return
	}

	libs, _ := h.tvLibs(r.Context())
	writeJSON(w, http.StatusOK, seriesToSonarr(existing, h.ids, libs))
}

func (h *Handler) deleteSeries(w http.ResponseWriter, r *http.Request) {
	numID, err := intIDStr(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	uuid, ok := h.ids.toStr(numID)
	if !ok {
		h.fillSeriesCache(r.Context())
		uuid, ok = h.ids.toStr(numID)
		if !ok {
			writeError(w, http.StatusNotFound, "series not found")
			return
		}
	}

	if err := h.svc.DeleteSeries(r.Context(), uuid); err != nil {
		h.logger.Error("delete series", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to delete series")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) lookupSeries(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	results, err := h.svc.SearchTMDB(r.Context(), term)
	if err != nil {
		h.logger.Error("lookup series", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	out := make([]map[string]interface{}, 0, len(results))
	for _, m := range results {
		out = append(out, tmdbResultToSonarr(m))
	}
	writeJSON(w, http.StatusOK, out)
}

// --- episode handlers ---

func (h *Handler) listEpisodes(w http.ResponseWriter, r *http.Request) {
	seriesIDParam := r.URL.Query().Get("seriesId")
	if seriesIDParam == "" {
		writeError(w, http.StatusBadRequest, "seriesId is required")
		return
	}

	numID, err := strconv.Atoi(seriesIDParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid seriesId")
		return
	}

	uuid, ok := h.ids.toStr(numID)
	if !ok {
		h.fillSeriesCache(r.Context())
		uuid, ok = h.ids.toStr(numID)
		if !ok {
			writeError(w, http.StatusNotFound, "series not found")
			return
		}
	}

	episodes, err := h.svc.ListEpisodes(r.Context(), uuid, nil)
	if err != nil {
		h.logger.Error("list episodes", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build season ID → season number map.
	seasonMap := h.buildSeasonMap(r.Context(), uuid)

	out := make([]sonarrEpisode, 0, len(episodes))
	for _, ep := range episodes {
		sn := seasonMap[ep.SeasonID]
		out = append(out, episodeToSonarr(ep, sn, h.ids))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getEpisode(w http.ResponseWriter, r *http.Request) {
	numID, err := intIDStr(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	uuid, ok := h.ids.toStr(numID)
	if !ok {
		writeError(w, http.StatusNotFound, "episode not found")
		return
	}

	// We need to find this episode. Walk all series to locate it.
	allSeries, err := h.svc.ListSeries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	for _, s := range allSeries {
		episodes, err := h.svc.ListEpisodes(r.Context(), s.ID, nil)
		if err != nil {
			continue
		}
		seasonMap := h.buildSeasonMap(r.Context(), s.ID)
		for _, ep := range episodes {
			if ep.ID == uuid {
				sn := seasonMap[ep.SeasonID]
				writeJSON(w, http.StatusOK, episodeToSonarr(ep, sn, h.ids))
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "episode not found")
}

// --- supporting endpoints ---

func (h *Handler) listRootFolders(w http.ResponseWriter, r *http.Request) {
	libs, err := h.tvLibs(r.Context())
	if err != nil {
		h.logger.Error("list root folders", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]sonarrRootFolder, 0, len(libs))
	for _, lib := range libs {
		out = append(out, rootFolderToSonarr(lib, h.ids))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) listQualityProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.qp.List(r.Context())
	if err != nil {
		h.logger.Error("list quality profiles", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]sonarrQualityProfile, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, qualityProfileToSonarr(p, h.ids))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) listLanguageProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []sonarrLanguageProfile{
		{
			ID:             1,
			Name:           "English",
			UpgradeAllowed: true,
			Cutoff:         sonarrLanguage{ID: 1, Name: "English"},
			Languages: []sonarrLangItem{
				{Language: sonarrLanguage{ID: 1, Name: "English"}, Allowed: true},
			},
		},
	})
}

func (h *Handler) listTags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *Handler) listCommands(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *Handler) postCommand(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)

	name, _ := body["name"].(string)
	if name == "" {
		name = "Unknown"
	}

	now := startTime.UTC().Format("2006-01-02T15:04:05Z")
	writeJSON(w, http.StatusCreated, sonarrCommand{
		ID:              1,
		Name:            name,
		Status:          "completed",
		StartedOn:       now,
		StateChangeTime: now,
	})
}

func (h *Handler) systemStatus(w http.ResponseWriter, _ *http.Request) {
	now := startTime.UTC().Format("2006-01-02T15:04:05Z")
	writeJSON(w, http.StatusOK, sonarrSystemStatus{
		AppName:        "Loom (Sonarr compat)",
		Version:        "3.0.0.0",
		BuildTime:      now,
		IsProduction:   true,
		IsDocker:       false,
		Branch:         "main",
		Authentication: "none",
		URLBase:        "",
		RuntimeVersion: runtime.Version(),
		RuntimeName:    "go",
		StartTime:      now,
		OsName:         runtime.GOOS,
	})
}

// --- internal helpers ---

// fillSeriesCache populates the ID cache by listing all series.
func (h *Handler) fillSeriesCache(ctx context.Context) {
	list, err := h.svc.ListSeries(ctx)
	if err != nil {
		return
	}
	for _, s := range list {
		h.ids.toInt(s.ID)
	}
}

// buildSeasonMap returns a map of season ID → season number for a series.
func (h *Handler) buildSeasonMap(ctx context.Context, seriesID string) map[string]int {
	seasons, err := h.svc.ListSeasons(ctx, seriesID)
	if err != nil {
		return map[string]int{}
	}
	m := make(map[string]int, len(seasons))
	for _, sn := range seasons {
		m[sn.ID] = sn.SeasonNumber
	}
	return m
}
