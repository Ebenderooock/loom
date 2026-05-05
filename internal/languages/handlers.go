package languages

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Mount registers language-profile routes on the given router.
func Mount(r chi.Router, store *Store) {
	r.Get("/api/v1/languages", listLanguages())
	r.Route("/api/v1/language-profiles", func(r chi.Router) {
		r.Get("/", listProfiles(store))
		r.Post("/", createProfile(store))
		r.Get("/{id}", getProfile(store))
		r.Put("/{id}", updateProfile(store))
		r.Delete("/{id}", deleteProfile(store))
	})
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

// listLanguages returns the full catalogue of known languages.
func listLanguages() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": AllLanguages})
	}
}

func listProfiles(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := store.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if profiles == nil {
			profiles = []LanguageProfile{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": profiles})
	}
}

func createProfile(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p LanguageProfile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if p.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if p.ID == "" {
			p.ID = Slugify(p.Name)
		}
		if err := store.Create(r.Context(), &p); err != nil {
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

		var req LanguageProfile
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Languages != nil {
			existing.Languages = req.Languages
		}
		if req.CutoffLanguage != "" {
			existing.CutoffLanguage = req.CutoffLanguage
		}
		existing.UpgradeAllowed = req.UpgradeAllowed

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
