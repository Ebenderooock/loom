package alttitles

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router mounts alt-title endpoints on a chi router.
func Router(store *Store) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listAltTitles(store))
	r.Post("/", createAltTitle(store))
	r.Delete("/{id}", deleteAltTitle(store))

	return r
}

func listAltTitles(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.URL.Query().Get("media_id")
		mediaType := r.URL.Query().Get("media_type")
		if mediaID == "" || mediaType == "" {
			http.Error(w, "media_id and media_type query params required", http.StatusBadRequest)
			return
		}

		titles, err := store.GetByMediaID(r.Context(), mediaID, mediaType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if titles == nil {
			titles = []*AltTitle{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": titles,
		})
	}
}

func createAltTitle(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateAltTitleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.MediaID == "" || req.MediaType == "" || req.Title == "" {
			http.Error(w, "media_id, media_type, and title are required", http.StatusBadRequest)
			return
		}

		alt := &AltTitle{
			MediaID:   req.MediaID,
			MediaType: req.MediaType,
			Title:     req.Title,
			Language:  req.Language,
			Source:    req.Source,
		}
		if err := store.Create(r.Context(), alt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(alt)
	}
}

func deleteAltTitle(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		if err := store.Delete(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
