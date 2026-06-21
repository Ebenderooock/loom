package music

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/libraries"
)

// ArtistRouter mounts artist endpoints (intended at /api/v1/artists).
func ArtistRouter(svc Service, opts ...ArtistRouterOption) chi.Router {
	var cfg artistRouterConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	r := chi.NewRouter()
	r.Get("/", handleListArtists(svc))
	r.Post("/", handleAddArtist(svc))
	r.Get("/lookup", handleLookupArtists(svc))
	r.Post("/refresh", handleRefreshAllArtists(svc))
	if cfg.libraryStore != nil && cfg.libraryScanner != nil {
		r.Post("/rescan", handleRescanAllArtistLibraries(cfg.libraryStore, cfg.libraryScanner))
	}
	r.Get("/{id}", handleGetArtist(svc))
	r.Patch("/{id}", handleUpdateArtist(svc))
	r.Put("/{id}", handleUpdateArtist(svc))
	r.Delete("/{id}", handleDeleteArtist(svc))
	r.Put("/{id}/monitoring", handleSetArtistMonitoring(svc))
	return r
}

type artistRouterConfig struct {
	libraryStore interface {
		List(ctx context.Context) ([]libraries.Library, error)
	}
	libraryScanner interface {
		ScanLibrary(ctx context.Context, lib *libraries.Library) error
	}
}

// ArtistRouterOption configures optional artist router dependencies.
type ArtistRouterOption func(*artistRouterConfig)

// WithLibraryRescan enables page-level rescan of all music libraries.
func WithLibraryRescan(
	store interface {
		List(ctx context.Context) ([]libraries.Library, error)
	},
	scanner interface {
		ScanLibrary(ctx context.Context, lib *libraries.Library) error
	},
) ArtistRouterOption {
	return func(cfg *artistRouterConfig) {
		cfg.libraryStore = store
		cfg.libraryScanner = scanner
	}
}

// AlbumRouter mounts album endpoints (intended at /api/v1/albums).
func AlbumRouter(svc Service) chi.Router {
	r := chi.NewRouter()
	r.Get("/{id}", handleGetAlbum(svc))
	r.Put("/{id}/monitoring", handleSetAlbumMonitored(svc))
	return r
}

// ProfileRouter mounts music profile/quality endpoints (intended at /api/v1/music).
func ProfileRouter(svc Service) chi.Router {
	r := chi.NewRouter()
	r.Get("/audio-quality-definitions", handleListAudioQualityDefinitions(svc))
	r.Get("/audio-quality-profiles", handleListAudioQualityProfiles(svc))
	r.Put("/audio-quality-profiles/{id}", handleUpdateAudioQualityProfile(svc))
	r.Get("/metadata-profiles", handleListMetadataProfiles(svc))
	return r
}

func handleListArtists(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artists, err := svc.ListArtists(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if artists == nil {
			artists = []*Artist{}
		}
		writeJSON(w, http.StatusOK, artists)
	}
}

func handleGetArtist(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := svc.GetArtist(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handleLookupArtists(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			q = r.URL.Query().Get("query")
		}
		limit := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			limit, _ = strconv.Atoi(v)
		}
		results, err := svc.LookupArtists(r.Context(), q, limit)
		if err != nil {
			writeErr(w, err)
			return
		}
		if results == nil {
			results = []*ArtistLookupResult{}
		}
		writeJSON(w, http.StatusOK, results)
	}
}

func handleAddArtist(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddArtistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		a, err := svc.AddArtist(r.Context(), req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, a)
	}
}

func handleUpdateArtist(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req UpdateArtistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		a, err := svc.UpdateArtist(r.Context(), chi.URLParam(r, "id"), req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handleDeleteArtist(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteArtist(r.Context(), chi.URLParam(r, "id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleSetArtistMonitoring(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SetMonitoringRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		a, err := svc.SetArtistMonitoring(r.Context(), chi.URLParam(r, "id"), MonitoringStatus(req.Status))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handleGetAlbum(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		al, err := svc.GetAlbum(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, al)
	}
}

func handleRefreshAllArtists(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artists, err := svc.ListArtists(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}

		ids := make([]string, 0, len(artists))
		for _, artist := range artists {
			ids = append(ids, artist.ID)
		}

		ctx := context.WithoutCancel(r.Context())
		go func(ctx context.Context, artistIDs []string) {
			for _, id := range artistIDs {
				if _, err := svc.RefreshArtistAlbums(ctx, id); err != nil {
					slog.Warn("music: bulk refresh failed", "artist_id", id, "error", err)
				}
			}
		}(ctx, ids)

		writeJSON(w, http.StatusAccepted, map[string]any{
			"message": "artist refresh started",
			"count":   len(ids),
		})
	}
}

func handleRescanAllArtistLibraries(
	store interface {
		List(ctx context.Context) ([]libraries.Library, error)
	},
	scanner interface {
		ScanLibrary(ctx context.Context, lib *libraries.Library) error
	},
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		librariesList, err := store.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		musicLibraries := make([]libraries.Library, 0, len(librariesList))
		for _, lib := range librariesList {
			if lib.MediaType == "music" {
				musicLibraries = append(musicLibraries, lib)
			}
		}

		ctx := context.WithoutCancel(r.Context())
		go func(ctx context.Context, libs []libraries.Library) {
			for _, lib := range libs {
				lib := lib
				if err := scanner.ScanLibrary(ctx, &lib); err != nil {
					slog.Warn("music: bulk rescan failed", "library_id", lib.ID, "error", err)
				}
			}
		}(ctx, musicLibraries)

		writeJSON(w, http.StatusAccepted, map[string]any{
			"message":      "music library rescan started",
			"libraryCount": len(musicLibraries),
		})
	}
}

func handleSetAlbumMonitored(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SetAlbumMonitoredRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		al, err := svc.SetAlbumMonitored(r.Context(), chi.URLParam(r, "id"), req.Monitored)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, al)
	}
}

func handleListAudioQualityDefinitions(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defs, err := svc.ListAudioQualityDefinitions(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if defs == nil {
			defs = []*AudioQualityDefinition{}
		}
		writeJSON(w, http.StatusOK, defs)
	}
}

func handleListAudioQualityProfiles(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := svc.ListAudioQualityProfiles(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if profiles == nil {
			profiles = []*AudioQualityProfile{}
		}
		writeJSON(w, http.StatusOK, profiles)
	}
}

func handleUpdateAudioQualityProfile(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateAudioQualityProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		profile, err := svc.UpdateAudioQualityProfile(r.Context(), id, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

func handleListMetadataProfiles(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := svc.ListMetadataProfiles(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if profiles == nil {
			profiles = []*MetadataProfile{}
		}
		writeJSON(w, http.StatusOK, profiles)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
