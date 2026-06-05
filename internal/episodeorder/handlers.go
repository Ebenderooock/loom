package episodeorder

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with episode-ordering endpoints.
// Intended to be mounted at /api/v1/episode-order.
func Router(store *Store) chi.Router {
	r := chi.NewRouter()

	// Series-scoped routes
	r.Get("/series/{id}/mappings", listMappings(store))
	r.Post("/series/{id}/mappings", createMapping(store))
	r.Put("/series/{id}/ordering-type", setOrderingType())

	// Mapping-scoped routes
	r.Delete("/mappings/{id}", deleteMapping(store))

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func listMappings(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "id")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "series id required"})
			return
		}
		orderingType := r.URL.Query().Get("type")
		mappings, err := store.ListMappings(r.Context(), seriesID, orderingType)
		if err != nil {
			slog.Error("episodeorder: list mappings", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if mappings == nil {
			mappings = []EpisodeMapping{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": mappings})
	}
}

func createMapping(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "id")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "series id required"})
			return
		}

		var req CreateMappingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		ot := OrderingType(req.OrderingType)
		if !ot.Valid() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "orderingType must be aired, dvd, absolute, or scene"})
			return
		}

		src := MappingSource(req.Source)
		if src == "" {
			src = SourceManual
		}

		m := &EpisodeMapping{
			SeriesID:     seriesID,
			OrderingType: ot,
			SeasonFrom:   req.SeasonFrom,
			EpisodeFrom:  req.EpisodeFrom,
			AbsoluteFrom: req.AbsoluteFrom,
			SeasonTo:     req.SeasonTo,
			EpisodeTo:    req.EpisodeTo,
			AbsoluteTo:   req.AbsoluteTo,
			Source:       src,
		}

		if err := store.CreateMapping(r.Context(), m); err != nil {
			slog.Error("episodeorder: create mapping", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusCreated, m)
	}
}

func deleteMapping(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mapping id required"})
			return
		}
		if err := store.DeleteMapping(r.Context(), id); err != nil {
			if err == sql.ErrNoRows {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "mapping not found"})
				return
			}
			slog.Error("episodeorder: delete mapping", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func setOrderingType() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "id")
		if seriesID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "series id required"})
			return
		}

		var req SetOrderingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		ot := OrderingType(req.OrderingType)
		if !ot.Valid() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "orderingType must be aired, dvd, absolute, or scene"})
			return
		}

		// Return the acknowledged preference (actual persistence is on the
		// series record itself; this endpoint is informational).
		writeJSON(w, http.StatusOK, map[string]any{
			"seriesId":     seriesID,
			"orderingType": ot,
		})
	}
}
