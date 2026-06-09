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
