package organizer

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts organizer endpoints on the movies router.
func RegisterRoutes(r chi.Router, org *Organizer) {
	r.Route("/organize", func(r chi.Router) {
		r.Get("/naming", getNamingConfig(org))
		r.Put("/naming", updateNamingConfig(org))
		r.Post("/naming/preview", previewNamingConfig(org))
		r.Post("/preview", previewRenames(org))
		r.Post("/rename", executeRenames(org))
		r.Get("/import-mode", getImportMode(org))
		r.Put("/import-mode", updateImportMode(org))
	})

	// Single movie organize (under /{id}/organize)
	r.Post("/{id}/organize", OrganizeSingleMovie(org))
}

// getNamingConfig returns the current naming configuration.
func getNamingConfig(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := org.GetConfig(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	}
}

// updateNamingConfig updates the naming configuration.
func updateNamingConfig(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req NamingConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		cfg := &NamingConfig{
			ID:                "default",
			MovieFolderFormat: req.MovieFolderFormat,
			MovieFileFormat:   req.MovieFileFormat,
			ColonReplacement:  req.ColonReplacement,
			RenameMovies:      req.RenameMovies,
		}

		if cfg.MovieFolderFormat == "" {
			cfg.MovieFolderFormat = DefaultNamingConfig().MovieFolderFormat
		}
		if cfg.MovieFileFormat == "" {
			cfg.MovieFileFormat = DefaultNamingConfig().MovieFileFormat
		}

		if err := org.SaveConfig(r.Context(), cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	}
}

// previewNamingConfig returns a sample preview of current naming settings.
func previewNamingConfig(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req NamingConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// If no body, use current config
			cfg, err := org.GetConfig(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(PreviewSample(cfg))
			return
		}

		cfg := &NamingConfig{
			MovieFolderFormat: req.MovieFolderFormat,
			MovieFileFormat:   req.MovieFileFormat,
			ColonReplacement:  req.ColonReplacement,
			RenameMovies:      req.RenameMovies,
		}
		if cfg.MovieFolderFormat == "" {
			cfg.MovieFolderFormat = DefaultNamingConfig().MovieFolderFormat
		}
		if cfg.MovieFileFormat == "" {
			cfg.MovieFileFormat = DefaultNamingConfig().MovieFileFormat
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PreviewSample(cfg))
	}
}

// previewRenames returns rename previews for specified movies (or all).
func previewRenames(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PreviewRequest
		json.NewDecoder(r.Body).Decode(&req)

		var previews []RenamePreview
		var err error

		if len(req.MovieIDs) > 0 {
			for _, id := range req.MovieIDs {
				p, e := org.PreviewMovie(r.Context(), id)
				if e != nil {
					previews = append(previews, RenamePreview{
						MovieID: id,
						Error:   e.Error(),
					})
					continue
				}
				previews = append(previews, p...)
			}
		} else {
			previews, err = org.PreviewAll(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(previews)
	}
}

// executeRenames performs the actual file renames.
func executeRenames(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req OrganizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.MovieIDs) == 0 {
			http.Error(w, "movie_ids is required", http.StatusBadRequest)
			return
		}

		results, err := org.OrganizeMovies(r.Context(), req.MovieIDs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

// OrganizeSingleMovie is a handler for POST /api/v1/movies/{id}/organize.
func OrganizeSingleMovie(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		movieID := chi.URLParam(r, "id")
		if movieID == "" {
			http.Error(w, "movie id is required", http.StatusBadRequest)
			return
		}

		results, err := org.OrganizeMovie(r.Context(), movieID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

// getImportMode returns the current import mode.
func getImportMode(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"import_mode": org.importMode,
		})
	}
}

// updateImportMode sets the import mode.
func updateImportMode(org *Organizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ImportMode string `json:"import_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		org.SetImportMode(req.ImportMode)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"import_mode": org.importMode,
		})
	}
}
