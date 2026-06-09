package musicsearch

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes attaches music-search endpoints onto an existing router. It is
// intended to be called with the album router (mounted at /api/v1/albums), so
// the search endpoint resolves to POST /api/v1/albums/{id}/search.
func RegisterRoutes(r chi.Router, engine *Engine) {
	if engine == nil {
		return
	}
	r.Post("/{id}/search", handleSearchAlbum(engine))
	r.Post("/{id}/search/upgrade", handleSearchUpgrade(engine))
	r.Get("/{id}/releases", handleListReleases(engine))
	r.Post("/{id}/grab", handleGrabRelease(engine))
}

func handleSearchAlbum(engine *Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result, err := engine.SearchAlbum(r.Context(), id)
		if err != nil {
			writeSearchErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleSearchUpgrade(engine *Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result, err := engine.SearchAlbumUpgrade(r.Context(), id)
		if err != nil {
			writeSearchErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleListReleases(engine *Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		releases, err := engine.ListReleases(r.Context(), id)
		if err != nil {
			writeSearchErr(w, err)
			return
		}
		if releases == nil {
			releases = []ReleaseCandidate{}
		}
		writeJSON(w, http.StatusOK, releases)
	}
}

func handleGrabRelease(engine *Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req GrabRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		result, err := engine.GrabRelease(r.Context(), id, req)
		if err != nil {
			writeSearchErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeSearchErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrNoResults):
		status = http.StatusNotFound
	case errors.Is(err, ErrNoDownloaders):
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
