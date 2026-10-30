package discover

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

func mountGrowth(router chi.Router, service *Service) {
	router.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := auth.IdentityFrom(r.Context())
				// Never use a shared anonymous/config key identity for preferences.
				if id == nil || id.UserID <= 0 {
					http.Error(w, "authenticated user required", http.StatusUnauthorized)
					return
				}
				if service == nil {
					writeDiscoveryError(w, ErrNotConfigured)
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/recommendations", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			mediaType := "movie"
			if q.Has("media_type") {
				mediaType = q.Get("media_type")
			}
			if !validMediaType(mediaType) {
				http.Error(w, "media_type must be movie or series", http.StatusBadRequest)
				return
			}
			includeUpcoming := false
			if q.Has("include_upcoming") {
				switch q.Get("include_upcoming") {
				case "true":
					includeUpcoming = true
				case "false":
				default:
					http.Error(w, "include_upcoming must be true or false", http.StatusBadRequest)
					return
				}
			}
			result, err := service.Recommendations(r.Context(), auth.IdentityFrom(r.Context()).UserID, mediaType, includeUpcoming)
			writeDiscoveryResult(w, result, err)
		})
		r.Post("/dismissals", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				TMDBID    int    `json:"tmdb_id"`
				MediaType string `json:"media_type"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&body); err != nil {
				http.Error(w, "invalid dismissal", http.StatusBadRequest)
				return
			}
			if decoder.Decode(new(any)) != io.EOF || body.TMDBID <= 0 || !validMediaType(body.MediaType) {
				http.Error(w, "invalid dismissal", http.StatusBadRequest)
				return
			}
			if service.store == nil {
				http.Error(w, "dismissal storage unavailable", http.StatusServiceUnavailable)
				return
			}
			if err := service.store.Dismiss(r.Context(), auth.IdentityFrom(r.Context()).UserID, body.MediaType, body.TMDBID); err != nil {
				http.Error(w, "could not save dismissal", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		r.Get("/collections", func(w http.ResponseWriter, r *http.Request) {
			result, err := service.Collections(r.Context())
			writeDiscoveryResult(w, result, err)
		})
		r.Get("/collections/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.Atoi(chi.URLParam(r, "id"))
			if err != nil || id <= 0 {
				http.Error(w, "invalid collection ID", http.StatusBadRequest)
				return
			}
			result, err := service.Collection(r.Context(), id)
			writeDiscoveryResult(w, result, err)
		})
	})
}

func writeDiscoveryResult(w http.ResponseWriter, result any, err error) {
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// Results include user preferences and live library status; do not allow
	// shared intermediary caches to serve one user's response to another.
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(result)
}

func writeDiscoveryError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "discovery request failed"
	var providerErr *tmdb.ClientError
	switch {
	case errors.Is(err, ErrNotConfigured):
		status, message = http.StatusServiceUnavailable, "TMDB provider is not configured"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status, message = http.StatusGatewayTimeout, "TMDB discovery request timed out or was cancelled"
	case errors.Is(err, ErrProvider):
		status, message = http.StatusBadGateway, "TMDB provider unavailable"
	case errors.As(err, &providerErr):
		status, message = http.StatusBadGateway, "TMDB provider unavailable"
		switch providerErr.Code {
		case tmdb.ErrCodeNotFound:
			status, message = http.StatusNotFound, "TMDB resource not found"
		case tmdb.ErrCodeUnauthorized:
			status, message = http.StatusBadGateway, "TMDB provider authentication failed"
		}
	}
	http.Error(w, message, status)
}
