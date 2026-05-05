package mediainfo

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with media-info and media-preferences endpoints.
func Router(store *Store, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()

	r.Get("/preferences", getPreferences(store, logger))
	r.Put("/preferences", putPreferences(store, logger))
	r.Post("/parse", parseReleaseName())

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getPreferences(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefs, err := store.GetPreferences(r.Context())
		if err != nil {
			logger.Error("mediainfo: get preferences", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, prefs)
	}
}

func putPreferences(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var prefs MediaPreferences
		if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		prefs.ID = "default"

		if prefs.PreferredAudioCodecs == nil {
			prefs.PreferredAudioCodecs = []string{}
		}
		if prefs.PreferredSubLanguages == nil {
			prefs.PreferredSubLanguages = []string{}
		}

		if err := store.UpsertPreferences(r.Context(), &prefs); err != nil {
			logger.Error("mediainfo: upsert preferences", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		// Re-read to get timestamps
		updated, err := store.GetPreferences(r.Context())
		if err != nil {
			logger.Error("mediainfo: re-read preferences", "err", err)
			writeJSON(w, http.StatusOK, prefs)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func parseReleaseName() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		info := Parse(req.Name)
		writeJSON(w, http.StatusOK, info)
	}
}
