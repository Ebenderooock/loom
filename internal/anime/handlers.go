package anime

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all anime API endpoints mounted.
func Router(store *Store) chi.Router {
	r := chi.NewRouter()

	r.Get("/preferences/{seriesId}", getPreferences(store))
	r.Put("/preferences/{seriesId}", putPreferences(store))
	r.Get("/mappings/{seriesId}", getMappings(store))
	r.Put("/mappings/{seriesId}", putMappings(store))
	r.Post("/parse", parseRelease())
	r.Get("/groups", listGroups())

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getPreferences(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "seriesId")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seriesId required"})
			return
		}
		prefs, err := store.GetPreferences(r.Context(), seriesID)
		if err != nil {
			slog.Error("anime: get preferences", "err", err, "seriesId", seriesID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, prefs)
	}
}

func putPreferences(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "seriesId")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seriesId required"})
			return
		}

		var prefs AnimePreferences
		if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		prefs.SeriesID = seriesID

		// Validate numbering scheme
		switch prefs.NumberingScheme {
		case NumberingAbsolute, NumberingSeason, NumberingAniDB:
			// valid
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "numberingScheme must be absolute, season, or anidb",
			})
			return
		}

		if err := store.UpsertPreferences(r.Context(), &prefs); err != nil {
			slog.Error("anime: upsert preferences", "err", err, "seriesId", seriesID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, prefs)
	}
}

func getMappings(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "seriesId")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seriesId required"})
			return
		}
		mappings, err := store.GetMappings(r.Context(), seriesID)
		if err != nil {
			slog.Error("anime: get mappings", "err", err, "seriesId", seriesID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if mappings == nil {
			mappings = []EpisodeMapping{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"seriesId": seriesID,
			"mappings": mappings,
		})
	}
}

func putMappings(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "seriesId")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seriesId required"})
			return
		}

		var req struct {
			Mappings []EpisodeMapping `json:"mappings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if err := ValidateMappings(req.Mappings); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		if err := store.ReplaceMappings(r.Context(), seriesID, req.Mappings); err != nil {
			slog.Error("anime: replace mappings", "err", err, "seriesId", seriesID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"seriesId": seriesID,
			"mappings": req.Mappings,
		})
	}
}

func parseRelease() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		result := Parse(req.Name)
		writeJSON(w, http.StatusOK, result)
	}
}

func listGroups() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"groups": DefaultGroups(),
		})
	}
}
