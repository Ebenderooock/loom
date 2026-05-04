package movies

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// derefString returns the value of a pointer or a default value if nil.
func derefString(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

// derefInt returns the value of a pointer or a default value if nil.
func derefInt(i *int, def int) int {
	if i == nil {
		return def
	}
	return *i
}

// derefFloat64 returns the value of a pointer or a default value if nil.
func derefFloat64(f *float64, def float64) float64 {
	if f == nil {
		return def
	}
	return *f
}

// Router mounts movies endpoints on the given chi router.
func Router(service Service) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listMovies(service))
	r.Post("/", addMovie(service))
	r.Get("/search", searchMovies(service))
	r.Get("/{id}", getMovie(service))
	r.Put("/{id}", updateMovie(service))
	r.Delete("/{id}", deleteMovie(service))
	r.Put("/{id}/monitoring", setMonitoringStatus(service))

	r.Get("/files/{movieID}", listMovieFiles(service))

	r.Get("/root-folders", listRootFolders(service))
	r.Post("/root-folders", addRootFolder(service))
	r.Delete("/root-folders/{id}", deleteRootFolder(service))

	return r
}

func movieToResponse(m *Movie) map[string]interface{} {
	return map[string]interface{}{
		"id":               m.ID,
		"title":            m.Title,
		"year":             m.Year,
		"imdbId":           m.IMDBID,
		"tmdbId":           m.TMDBID,
		"tvdbId":           m.TVDBID,
		"overview":         m.Overview,
		"genres":           m.Genres,
		"runtime":          m.Runtime,
		"rating":           m.Rating,
		"backdropPath":     m.BackdropPath,
		"posterPath":       m.PosterPath,
		"metadataProvider": m.MetadataProvider,
		"monitoringStatus": string(m.MonitoringStatus),
		"createdAt":        m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updatedAt":        m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// Handlers

func listMovies(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 25
		offset := 0

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
				limit = l
			}
		}
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
				offset = o
			}
		}

		movies, err := svc.ListMovies(r.Context(), limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]interface{}, 0, len(movies))
		for _, m := range movies {
			response = append(response, movieToResponse(m))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":   response,
			"total":  len(movies),
			"limit":  limit,
			"offset": offset,
		})
	}
}

func searchMovies(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "search query required", http.StatusBadRequest)
			return
		}

		movies, err := svc.SearchMovies(r.Context(), query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]interface{}, 0, len(movies))
		for _, m := range movies {
			response = append(response, movieToResponse(m))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": response,
		})
	}
}

func getMovie(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		movie, err := svc.GetMovie(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if movie == nil {
			http.Error(w, "movie not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(movieToResponse(movie))
	}
}

func addMovie(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		movie := &Movie{
			ID:               strings.ToLower(strings.ReplaceAll(req.Title, " ", "-")),
			Title:            req.Title,
			Year:             req.Year,
			IMDBID:           req.IMDBID,
			TMDBID:           req.TMDBID,
			TVDBID:           req.TVDBID,
			Overview:         req.Overview,
			Genres:           req.Genres,
			Runtime:          req.Runtime,
			Rating:           req.Rating,
			BackdropPath:     req.BackdropPath,
			PosterPath:       req.PosterPath,
			MetadataProvider: req.MetadataProvider,
			MonitoringStatus: MonitoringStatus(derefString(req.MonitoringStatus, string(MonitoringStatusMonitored))),
		}

		if err := svc.AddMovie(r.Context(), movie); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(movieToResponse(movie))
	}
}

func updateMovie(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		movie, err := svc.GetMovie(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if movie == nil {
			http.Error(w, "movie not found", http.StatusNotFound)
			return
		}

		var req UpdateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// Update only provided fields
		if req.Title != nil {
			movie.Title = *req.Title
		}
		if req.Year != nil {
			movie.Year = *req.Year
		}
		if req.Overview != nil {
			movie.Overview = *req.Overview
		}
		if req.Genres != nil {
			movie.Genres = req.Genres
		}
		if req.Runtime != nil {
			movie.Runtime = *req.Runtime
		}
		if req.Rating != nil {
			movie.Rating = *req.Rating
		}
		if req.BackdropPath != nil {
			movie.BackdropPath = *req.BackdropPath
		}
		if req.PosterPath != nil {
			movie.PosterPath = *req.PosterPath
		}
		if req.MonitoringStatus != nil {
			movie.MonitoringStatus = MonitoringStatus(*req.MonitoringStatus)
		}

		if err := svc.UpdateMovie(r.Context(), movie); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(movieToResponse(movie))
	}
}

func deleteMovie(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		if err := svc.DeleteMovie(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func setMonitoringStatus(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		var req SetMonitoringStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := svc.SetMonitoringStatus(r.Context(), id, MonitoringStatus(req.Status)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		movie, _ := svc.GetMovie(r.Context(), id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(movieToResponse(movie))
	}
}

func listMovieFiles(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		movieID := chi.URLParam(r, "movieID")
		if movieID == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		files, err := svc.ListMovieFiles(r.Context(), movieID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]map[string]interface{}, 0, len(files))
		for _, f := range files {
			response = append(response, map[string]interface{}{
				"id":        f.ID,
				"movieId":   f.MovieID,
				"filePath":  f.FilePath,
				"size":      f.Size,
				"quality":   f.Quality,
				"format":    f.Format,
				"mediaInfo": f.MediaInfo,
				"createdAt": f.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func listRootFolders(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		folders, err := svc.ListRootFolders(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]map[string]interface{}, 0, len(folders))
		for _, f := range folders {
			response = append(response, map[string]interface{}{
				"id":           f.ID,
				"path":         f.Path,
				"freeSpace":    f.FreeSpace,
				"unmappedCount": f.UnmappedCount,
				"createdAt":    f.CreatedAt.Format("2006-01-02T15:04:05Z"),
				"updatedAt":    f.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func addRootFolder(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRootFolderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		folder, err := svc.AddRootFolder(r.Context(), req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           folder.ID,
			"path":         folder.Path,
			"freeSpace":    folder.FreeSpace,
			"unmappedCount": folder.UnmappedCount,
			"createdAt":    folder.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updatedAt":    folder.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
}

func deleteRootFolder(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "folder ID required", http.StatusBadRequest)
			return
		}

		if err := svc.DeleteRootFolder(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
