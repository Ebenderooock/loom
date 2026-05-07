package syncprofiles

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all sync-profile endpoints mounted.
func Router(store *Store) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listProfiles(store))
	r.Post("/", createProfile(store))
	r.Get("/{id}", getProfile(store))
	r.Put("/{id}", updateProfile(store))
	r.Delete("/{id}", deleteProfile(store))

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}

func listProfiles(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := store.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if profiles == nil {
			profiles = []*SyncProfile{}
		}
		writeJSON(w, http.StatusOK, profiles)
	}
}

func createProfile(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		p := &SyncProfile{
			Name:       req.Name,
			AppType:    req.AppType,
			Enabled:    enabled,
			Indexers:   req.Indexers,
			Categories: req.Categories,
		}

		if err := store.Create(r.Context(), p); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}
}

func getProfile(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, err := store.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func updateProfile(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		existing, err := store.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		var req UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name != nil {
			existing.Name = *req.Name
		}
		if req.AppType != nil {
			existing.AppType = *req.AppType
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if req.Indexers != nil {
			existing.Indexers = req.Indexers
		}
		if req.Categories != nil {
			existing.Categories = req.Categories
		}

		if err := store.Update(r.Context(), existing); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, existing)
	}
}

func deleteProfile(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Delete(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
