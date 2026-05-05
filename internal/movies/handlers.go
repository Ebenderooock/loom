package movies

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/loomctl/loom/internal/indexers"
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
	return RouterWithSearch(service, nil)
}

// RouterWithSearch mounts movies endpoints with optional search-on-add support.
func RouterWithSearch(service Service, indexerSvc *indexers.Service) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listMovies(service))
	r.Post("/", addMovie(service, indexerSvc))
	r.Get("/search", searchMovies(service))
	r.Get("/lookup", lookupMovies(service))

	// Root folder routes (must be before /{id} wildcard)
	r.Route("/root-folders", func(r chi.Router) {
		r.Get("/", listRootFolders(service))
		r.Post("/", addRootFolder(service))
		r.Delete("/{id}", deleteRootFolder(service))
	})

	// Quality definition routes
	r.Route("/quality-definitions", func(r chi.Router) {
		r.Get("/", listQualityDefinitions(service))
		r.Post("/", addQualityDefinition(service))
		r.Get("/{id}", getQualityDefinition(service))
		r.Put("/{id}", updateQualityDefinition(service))
		r.Delete("/{id}", deleteQualityDefinition(service))
	})

	// Quality profile routes
	r.Route("/quality-profiles", func(r chi.Router) {
		r.Get("/", listQualityProfiles(service))
		r.Post("/", addQualityProfile(service))
		r.Get("/{id}", getQualityProfile(service))
		r.Put("/{id}", updateQualityProfile(service))
		r.Delete("/{id}", deleteQualityProfile(service))
	})

	// Custom format routes
	r.Route("/custom-formats", func(r chi.Router) {
		r.Get("/", listCustomFormats(service))
		r.Post("/", addCustomFormat(service))
		r.Get("/{id}", getCustomFormat(service))
		r.Put("/{id}", updateCustomFormat(service))
		r.Delete("/{id}", deleteCustomFormat(service))
		r.Post("/{id}/test", testCustomFormat(service))
	})

	r.Get("/files/{movieID}", listMovieFiles(service))

	// Wildcard movie routes (must be last)
	r.Get("/{id}", getMovie(service))
	r.Put("/{id}", updateMovie(service))
	r.Delete("/{id}", deleteMovie(service))
	r.Put("/{id}/monitoring", setMonitoringStatus(service))
	r.Post("/{id}/refresh", refreshMovie(service))
	r.Get("/{id}/credits", getMovieCredits(service))

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
		"qualityProfileId": m.QualityProfileID,
		"rootFolderId":     m.RootFolderID,
		"status":           string(m.Status),
		"releaseDate":      m.ReleaseDate,
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

func lookupMovies(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		term := r.URL.Query().Get("term")
		if term == "" {
			http.Error(w, "lookup term required", http.StatusBadRequest)
			return
		}

		results, err := svc.LookupMovies(r.Context(), term)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
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

func addMovie(svc Service, indexerSvc *indexers.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// Use a slug-based ID but add a unique suffix to avoid collisions
		slug := slugify(req.Title)
		if req.Year > 0 {
			slug = slug + "-" + strconv.Itoa(req.Year)
		}

		// Determine initial status based on release date
		status := MovieStatusMissing
		if req.ReleaseDate != "" {
			if t, err := time.Parse("2006-01-02", req.ReleaseDate); err == nil {
				if t.After(time.Now()) {
					status = MovieStatusUnreleased
				}
			}
		}

		movie := &Movie{
			ID:               slug,
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
			QualityProfileID: req.QualityProfileID,
			RootFolderID:     req.RootFolderID,
			Status:           status,
			ReleaseDate:      req.ReleaseDate,
			MonitoringStatus: MonitoringStatus(derefString(req.MonitoringStatus, string(MonitoringStatusMonitored))),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		if err := svc.AddMovie(r.Context(), movie); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fire async indexer search if requested
		if req.Search && indexerSvc != nil {
			go fireMovieSearch(movie, indexerSvc)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(movieToResponse(movie))
	}
}

// fireMovieSearch runs an indexer search for a movie in the background.
func fireMovieSearch(movie *Movie, indexerSvc *indexers.Service) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	q := indexers.Query{
		Term:       movie.Title,
		Categories: []indexers.Category{indexers.CategoryMovies},
	}
	if movie.IMDBID != nil && *movie.IMDBID != "" {
		q.IMDBID = *movie.IMDBID
	}
	if movie.TMDBID != nil && *movie.TMDBID != "" {
		q.TMDBID = *movie.TMDBID
	}

	result := indexerSvc.Search(ctx, q, nil, 30*time.Second)
	if len(result.Errors) > 0 {
		slog.Warn("search-on-add had errors for movie",
			"movie", movie.Title, "errors", result.Errors)
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
		if req.QualityProfileID != nil {
			movie.QualityProfileID = *req.QualityProfileID
		}
		if req.RootFolderID != nil {
			movie.RootFolderID = *req.RootFolderID
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

func refreshMovie(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		if err := svc.RefreshMovie(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}

func getMovieCredits(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "movie ID required", http.StatusBadRequest)
			return
		}

		credits, err := svc.GetMovieCredits(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(credits)
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

// Quality Definition Handlers

func listQualityDefinitions(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defs, err := svc.ListQualityDefinitions(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(defs)
	}
}

func addQualityDefinition(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateQualityDefinitionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		title := req.Title
		if title == "" {
			title = req.Name
		}

		qd := &QualityDefinition{
			ID:          strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")),
			Name:        req.Name,
			Title:       title,
			Source:      req.Source,
			Resolution:  req.Resolution,
			Modifier:    req.Modifier,
			MinFileSize: req.MinFileSize,
			MaxFileSize: req.MaxFileSize,
			PreferredAt: req.PreferredAt,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := svc.AddQualityDefinition(r.Context(), qd); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(qd)
	}
}

func getQualityDefinition(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "quality definition ID required", http.StatusBadRequest)
			return
		}

		qd, err := svc.GetQualityDefinition(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qd)
	}
}

func updateQualityDefinition(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "quality definition ID required", http.StatusBadRequest)
			return
		}

		var req UpdateQualityDefinitionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		qd, err := svc.GetQualityDefinition(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		if req.Name != nil {
			qd.Name = *req.Name
		}
		if req.Title != nil {
			qd.Title = *req.Title
		}
		if req.Source != nil {
			qd.Source = *req.Source
		}
		if req.Resolution != nil {
			qd.Resolution = *req.Resolution
		}
		if req.Modifier != nil {
			qd.Modifier = *req.Modifier
		}
		if req.MinFileSize != nil {
			qd.MinFileSize = *req.MinFileSize
		}
		if req.MaxFileSize != nil {
			qd.MaxFileSize = *req.MaxFileSize
		}
		if req.PreferredAt != nil {
			qd.PreferredAt = *req.PreferredAt
		}

		qd.UpdatedAt = time.Now()

		if err := svc.UpdateQualityDefinition(r.Context(), qd); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qd)
	}
}

func deleteQualityDefinition(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "quality definition ID required", http.StatusBadRequest)
			return
		}

		if err := svc.DeleteQualityDefinition(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// Quality Profile Handlers

func listQualityProfiles(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := svc.ListQualityProfiles(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profiles)
	}
}

func addQualityProfile(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateQualityProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		language := req.Language
		if language == "" {
			language = "en"
		}

		qp := &QualityProfile{
			ID:             strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")),
			Name:           req.Name,
			UpgradeAllowed: req.UpgradeAllowed,
			Cutoff:         req.Cutoff,
			Language:       language,
			Items:          req.Items,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if err := svc.AddQualityProfile(r.Context(), qp); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(qp)
	}
}

func getQualityProfile(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "quality profile ID required", http.StatusBadRequest)
			return
		}

		qp, err := svc.GetQualityProfile(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qp)
	}
}

func updateQualityProfile(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "quality profile ID required", http.StatusBadRequest)
			return
		}

		var req UpdateQualityProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		qp, err := svc.GetQualityProfile(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		if req.Name != nil {
			qp.Name = *req.Name
		}
		if req.UpgradeAllowed != nil {
			qp.UpgradeAllowed = *req.UpgradeAllowed
		}
		if req.Cutoff != nil {
			qp.Cutoff = *req.Cutoff
		}
		if req.Language != nil {
			qp.Language = *req.Language
		}
		if len(req.Items) > 0 {
			qp.Items = req.Items
		}

		qp.UpdatedAt = time.Now()

		if err := svc.UpdateQualityProfile(r.Context(), qp); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qp)
	}
}

func deleteQualityProfile(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "quality profile ID required", http.StatusBadRequest)
			return
		}

		if err := svc.DeleteQualityProfile(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// Custom Format Handlers

func listCustomFormats(svc Service) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
formats, err := svc.ListCustomFormats(r.Context())
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(formats)
}
}

func addCustomFormat(svc Service) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
var req CreateCustomFormatRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}

// Generate ID from name (slug)
id := slugify(req.Name)

// Convert request filters to domain filters
var filters []CustomFormatFilter
for i, f := range req.Filters {
filters = append(filters, CustomFormatFilter{
ID:             "",
CustomFormatID: id,
Field:          f.Field,
Condition:      f.Condition,
Value:          f.Value,
Order:          i,
CreatedAt:      time.Now(),
UpdatedAt:      time.Now(),
})
}

cf := &CustomFormat{
ID:          id,
Name:        req.Name,
Description: req.Description,
Tags:        req.Tags,
Filters:     filters,
CreatedAt:   time.Now(),
UpdatedAt:   time.Now(),
}

if err := svc.AddCustomFormat(r.Context(), cf); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}

w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(cf)
}
}

func getCustomFormat(svc Service) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
cf, err := svc.GetCustomFormat(r.Context(), id)
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
if cf == nil {
http.Error(w, "custom format not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(cf)
}
}

func updateCustomFormat(svc Service) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")

var req UpdateCustomFormatRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}

// Retrieve existing custom format
cf, err := svc.GetCustomFormat(r.Context(), id)
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
if cf == nil {
http.Error(w, "custom format not found", http.StatusNotFound)
return
}

// Update fields
if req.Name != nil {
cf.Name = *req.Name
}
if req.Description != nil {
cf.Description = *req.Description
}
if req.Tags != nil {
cf.Tags = req.Tags
}
if req.Filters != nil {
var filters []CustomFormatFilter
for i, f := range req.Filters {
filters = append(filters, CustomFormatFilter{
ID:             "",
CustomFormatID: id,
Field:          f.Field,
Condition:      f.Condition,
Value:          f.Value,
Order:          i,
CreatedAt:      time.Now(),
UpdatedAt:      time.Now(),
})
}
cf.Filters = filters
}
cf.UpdatedAt = time.Now()

if err := svc.UpdateCustomFormat(r.Context(), cf); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(cf)
}
}

func deleteCustomFormat(svc Service) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
if err := svc.DeleteCustomFormat(r.Context(), id); err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.WriteHeader(http.StatusNoContent)
}
}

func testCustomFormat(svc Service) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")

var req struct {
ReleaseName string `json:"release_name"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}

cf, err := svc.GetCustomFormat(r.Context(), id)
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
if cf == nil {
http.Error(w, "custom format not found", http.StatusNotFound)
return
}

// Test if release name matches all filters
matches := true
for _, filter := range cf.Filters {
// Simple pattern matching for testing
if !filterMatches(filter, req.ReleaseName) {
matches = false
break
}
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
"custom_format_id": id,
"release_name":     req.ReleaseName,
"matches":          matches,
})
}
}

// filterMatches checks if a single filter matches the release name (copied from service for testing).
func filterMatches(filter CustomFormatFilter, releaseName string) bool {
switch filter.Condition {
case ConditionEquals:
return strings.EqualFold(releaseName, filter.Value)
case ConditionRegex:
re, err := regexp.Compile(filter.Value)
if err != nil {
return false
}
return re.MatchString(releaseName)
case ConditionIn:
values := strings.Split(filter.Value, ",")
for _, v := range values {
if strings.EqualFold(strings.TrimSpace(releaseName), strings.TrimSpace(v)) {
return true
}
}
return false
default:
return false
}
}

// slugify converts a name to a URL-safe slug (lowercase, spaces to hyphens).
func slugify(name string) string {
return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}
