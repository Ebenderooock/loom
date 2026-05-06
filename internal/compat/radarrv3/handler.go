// Package radarrv3 provides a Radarr v3 API compatibility shim so external
// tools (Overseerr, Ombi, Tautulli, Bazarr, etc.) can talk to Loom as if it
// were a Radarr instance.
package radarrv3

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loomctl/loom/internal/libraries"
	"github.com/loomctl/loom/internal/movies"
	"github.com/loomctl/loom/internal/qualityprofiles"
)

// Handler exposes Radarr v3–compatible HTTP endpoints backed by Loom services.
type Handler struct {
	svc      movies.Service
	libs     *libraries.Store
	qps      *qualityprofiles.Store
	log      *slog.Logger

	// ID mappers (Radarr int ↔ Loom string).
	movies *idMapper
	libIDs *idMapper
	qpIDs  *idMapper

	// Cached library paths for path↔ID resolution.
	libCacheMu  sync.RWMutex
	libPathCache map[string]string // libraryID → path
}

// NewHandler creates a Handler and returns it.
func NewHandler(svc movies.Service, libs *libraries.Store, qps *qualityprofiles.Store, log *slog.Logger) *Handler {
	return &Handler{
		svc:          svc,
		libs:         libs,
		qps:          qps,
		log:          log,
		movies:       newIDMapper(),
		libIDs:       newIDMapper(),
		qpIDs:        newIDMapper(),
		libPathCache: make(map[string]string),
	}
}

// Router returns a chi.Router with all Radarr v3 API routes registered.
func Router(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Get("/api/v3/movie", h.listMovies)
	r.Get("/api/v3/movie/lookup", h.lookupMovies)
	r.Post("/api/v3/movie", h.addMovie)
	r.Get("/api/v3/movie/{id}", h.getMovie)
	r.Put("/api/v3/movie/{id}", h.updateMovie)
	r.Delete("/api/v3/movie/{id}", h.deleteMovie)

	r.Get("/api/v3/rootfolder", h.listRootFolders)
	r.Get("/api/v3/qualityprofile", h.listQualityProfiles)

	r.Get("/api/v3/command", h.listCommands)
	r.Post("/api/v3/command", h.startCommand)

	r.Get("/api/v3/system/status", h.systemStatus)
	r.Get("/api/v3/tag", h.listTags)

	return r
}

// ---------------------------------------------------------------------------
// Movies
// ---------------------------------------------------------------------------

func (h *Handler) listMovies(w http.ResponseWriter, r *http.Request) {
	h.refreshLibraryCache(r.Context())

	mvs, err := h.svc.ListMovies(r.Context(), 0, 0)
	if err != nil {
		h.jsonError(w, "listing movies", err, http.StatusInternalServerError)
		return
	}

	out := make([]radarrMovie, 0, len(mvs))
	for _, m := range mvs {
		hasFile := h.hasMovieFile(r.Context(), m.ID)
		out = append(out, h.movieToRadarr(m, hasFile))
	}
	h.jsonOK(w, out)
}

func (h *Handler) getMovie(w http.ResponseWriter, r *http.Request) {
	h.refreshLibraryCache(r.Context())

	loomID, ok := h.resolveMovieID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.GetMovie(r.Context(), loomID)
	if err != nil {
		h.jsonError(w, "getting movie", err, http.StatusNotFound)
		return
	}
	hasFile := h.hasMovieFile(r.Context(), m.ID)
	h.jsonOK(w, h.movieToRadarr(m, hasFile))
}

func (h *Handler) addMovie(w http.ResponseWriter, r *http.Request) {
	h.refreshLibraryCache(r.Context())

	var req radarrAddMovieRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "decoding request", err, http.StatusBadRequest)
		return
	}

	m := h.radarrToLoomMovie(req)
	if err := h.svc.AddMovie(r.Context(), m); err != nil {
		h.jsonError(w, "adding movie", err, http.StatusInternalServerError)
		return
	}

	h.log.Info("radarr compat: movie added", "title", m.Title, "id", m.ID)
	w.WriteHeader(http.StatusCreated)
	h.jsonOK(w, h.movieToRadarr(m, false))
}

func (h *Handler) updateMovie(w http.ResponseWriter, r *http.Request) {
	h.refreshLibraryCache(r.Context())

	loomID, ok := h.resolveMovieID(w, r)
	if !ok {
		return
	}

	existing, err := h.svc.GetMovie(r.Context(), loomID)
	if err != nil {
		h.jsonError(w, "getting movie for update", err, http.StatusNotFound)
		return
	}

	var payload radarrMovie
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "decoding request", err, http.StatusBadRequest)
		return
	}

	// Apply mutable fields.
	if payload.Title != "" {
		existing.Title = payload.Title
	}
	if payload.Monitored {
		existing.MonitoringStatus = movies.MonitoringStatusMonitored
	} else {
		existing.MonitoringStatus = movies.MonitoringStatusUnmonitored
	}
	if payload.QualityProfileID != 0 {
		if qpID, found := h.qpIDs.toString(payload.QualityProfileID); found {
			existing.QualityProfileID = qpID
		}
	}

	if err := h.svc.UpdateMovie(r.Context(), existing); err != nil {
		h.jsonError(w, "updating movie", err, http.StatusInternalServerError)
		return
	}
	hasFile := h.hasMovieFile(r.Context(), existing.ID)
	h.jsonOK(w, h.movieToRadarr(existing, hasFile))
}

func (h *Handler) deleteMovie(w http.ResponseWriter, r *http.Request) {
	loomID, ok := h.resolveMovieID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteMovie(r.Context(), loomID); err != nil {
		h.jsonError(w, "deleting movie", err, http.StatusInternalServerError)
		return
	}
	h.log.Info("radarr compat: movie deleted", "id", loomID)
	h.jsonOK(w, struct{}{})
}

func (h *Handler) lookupMovies(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		h.jsonOK(w, []radarrMovie{})
		return
	}

	results, err := h.svc.LookupMovies(r.Context(), term)
	if err != nil {
		h.jsonError(w, "looking up movies", err, http.StatusInternalServerError)
		return
	}

	out := make([]radarrMovie, 0, len(results))
	for _, md := range results {
		out = append(out, h.metadataToRadarr(md))
	}
	h.jsonOK(w, out)
}

// ---------------------------------------------------------------------------
// Root Folders
// ---------------------------------------------------------------------------

func (h *Handler) listRootFolders(w http.ResponseWriter, r *http.Request) {
	libs, err := h.libs.List(r.Context())
	if err != nil {
		h.jsonError(w, "listing libraries", err, http.StatusInternalServerError)
		return
	}
	out := make([]radarrRootFolder, 0, len(libs))
	for _, lib := range libs {
		out = append(out, h.libraryToRootFolder(lib))
		h.cacheLibraryPath(lib.ID, lib.Path)
	}
	h.jsonOK(w, out)
}

// ---------------------------------------------------------------------------
// Quality Profiles
// ---------------------------------------------------------------------------

func (h *Handler) listQualityProfiles(w http.ResponseWriter, r *http.Request) {
	qps, err := h.qps.List(r.Context())
	if err != nil {
		h.jsonError(w, "listing quality profiles", err, http.StatusInternalServerError)
		return
	}
	out := make([]radarrQualityProfile, 0, len(qps))
	for _, qp := range qps {
		out = append(out, h.qpToRadarr(qp))
	}
	h.jsonOK(w, out)
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (h *Handler) listCommands(w http.ResponseWriter, _ *http.Request) {
	h.jsonOK(w, []radarrCommand{})
}

func (h *Handler) startCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	h.log.Info("radarr compat: command received", "name", body.Name)

	cmd := radarrCommand{
		ID:       1,
		Name:     body.Name,
		Status:   "completed",
		Queued:   time.Now(),
		Started:  time.Now(),
		Ended:    time.Now(),
		Priority: "normal",
		Trigger:  "manual",
	}
	w.WriteHeader(http.StatusCreated)
	h.jsonOK(w, cmd)
}

// ---------------------------------------------------------------------------
// System
// ---------------------------------------------------------------------------

func (h *Handler) systemStatus(w http.ResponseWriter, _ *http.Request) {
	h.jsonOK(w, radarrSystemStatus{
		AppName:          "Radarr",
		InstanceName:     "Loom (Radarr compat)",
		Version:          "3.2.2.5080",
		BuildTime:        time.Now().UTC().Format(time.RFC3339),
		IsProduction:     true,
		OsName:           runtime.GOOS,
		OsVersion:        runtime.GOARCH,
		Authentication:   "none",
		URLBase:          "",
		RuntimeVersion:   runtime.Version(),
		RuntimeName:      "go",
		Branch:           "main",
		MigrationVersion: 1,
	})
}

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

func (h *Handler) listTags(w http.ResponseWriter, _ *http.Request) {
	h.jsonOK(w, []radarrTag{})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *Handler) resolveMovieID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := chi.URLParam(r, "id")
	radarrID, err := parseRadarrID(raw)
	if err != nil {
		h.jsonError(w, "invalid movie id", err, http.StatusBadRequest)
		return "", false
	}
	loomID, ok := h.movies.toString(radarrID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"message": fmt.Sprintf("unknown movie id %d — list movies first to populate the ID cache", radarrID),
		})
		return "", false
	}
	return loomID, true
}

func (h *Handler) hasMovieFile(ctx context.Context, movieID string) bool {
	files, err := h.svc.ListMovieFiles(ctx, movieID)
	if err != nil {
		return false
	}
	return len(files) > 0
}

func (h *Handler) refreshLibraryCache(ctx context.Context) {
	libs, err := h.libs.List(ctx)
	if err != nil {
		return
	}
	for _, lib := range libs {
		h.cacheLibraryPath(lib.ID, lib.Path)
		h.libIDs.toInt(lib.ID)
	}
}

func (h *Handler) cacheLibraryPath(id, path string) {
	h.libCacheMu.Lock()
	h.libPathCache[id] = path
	h.libCacheMu.Unlock()
}

func (h *Handler) jsonOK(w http.ResponseWriter, v any) {
	// Only set status if header not already written (e.g., by addMovie's 201).
	writeJSON(w, 0, v)
}

func (h *Handler) jsonError(w http.ResponseWriter, msg string, err error, code int) {
	h.log.Error("radarr compat: "+msg, "error", err)
	writeJSON(w, code, map[string]string{"message": msg + ": " + err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	if code > 0 {
		w.WriteHeader(code)
	}
	_ = json.NewEncoder(w).Encode(v)
}

// populateQPCache pre-warms the quality-profile ID mapper so incoming
// add-movie requests with Radarr integer profile IDs can be resolved.
func (h *Handler) populateQPCache(ctx context.Context) {
	qps, err := h.qps.List(ctx)
	if err != nil {
		return
	}
	for _, qp := range qps {
		h.qpIDs.toInt(qp.ID)
	}
}

// Ensure Radarr integer IDs are properly quoted in lookup-movie responses
// where they might be 0 (unset TMDB IDs). We use the TMDB ID as the
// Radarr object ID for search results, falling back to a counter.
var lookupCounter int

func nextLookupID() int {
	lookupCounter++
	return lookupCounter
}

// tmdbIDInt parses a *string TMDB ID to int, returning 0 on failure.
func tmdbIDInt(s *string) int {
	if s == nil {
		return 0
	}
	n, _ := strconv.Atoi(*s)
	return n
}
