package packs

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all pack API endpoints mounted.
func Router(store *Store) chi.Router {
	r := chi.NewRouter()

	r.Get("/history", listPackHistory(store))
	r.Post("/detect", detectPack())

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func listPackHistory(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := r.URL.Query().Get("seriesId")
		history, err := store.ListHistory(r.Context(), seriesID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if history == nil {
			history = []PackHistory{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": history})
	}
}

func detectPack() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
			return
		}
		result := Detect(req.Title)
		writeJSON(w, http.StatusOK, result)
	}
}
