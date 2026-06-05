package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// MediaArchiver auto-archives watched movies/series based on library settings.
// Implemented by the server layer to bridge connect ↔ movies/series modules.
type MediaArchiver interface {
	ArchiveWatchedMovies(ctx context.Context, traktMovies json.RawMessage) (archived int, err error)
	ArchiveWatchedShows(ctx context.Context, traktShows json.RawMessage) (archived int, err error)
}

// TraktSyncRouter returns routes for Trakt sync operations.
func TraktSyncRouter(svc Service, archiver ...MediaArchiver) chi.Router {
	var ma MediaArchiver
	if len(archiver) > 0 {
		ma = archiver[0]
	}
	r := chi.NewRouter()
	r.Post("/watched/{id}", handleSyncWatched(svc, ma))
	r.Post("/collection/{id}", handleSyncCollection(svc))
	r.Post("/watchlist/{id}", handleSyncWatchlist(svc))
	return r
}

// getTraktConnection loads a connection and validates it for Trakt API access.
func getTraktConnection(svc Service, r *http.Request) (*Connection, int, string) {
	id := chi.URLParam(r, "id")

	conn, err := svc.GetConnection(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, http.StatusNotFound, err.Error()
		}
		return nil, http.StatusInternalServerError, err.Error()
	}

	if conn.Provider != ProviderTrakt {
		return nil, http.StatusBadRequest, "connection is not a trakt provider"
	}
	if conn.Settings.AccessToken == "" {
		return nil, http.StatusBadRequest, "trakt connection has no access token; complete OAuth first"
	}

	return conn, 0, ""
}

// traktAPIGet performs an authenticated GET against the Trakt API.
func traktAPIGet(r *http.Request, s ProviderSettings, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.trakt.tv"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("trakt api: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", s.ClientID)
	req.Header.Set("Authorization", "Bearer "+s.AccessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trakt api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trakt api: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trakt api %s: unexpected status %d: %s", path, resp.StatusCode, string(body))
	}

	return json.RawMessage(body), nil
}

func handleSyncWatched(svc Service, archiver MediaArchiver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, status, msg := getTraktConnection(svc, r)
		if conn == nil {
			writeError(w, status, msg)
			return
		}

		movies, err := traktAPIGet(r, conn.Settings, "/sync/watched/movies")
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		shows, err := traktAPIGet(r, conn.Settings, "/sync/watched/shows")
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		var movieList, showList []any
		if err := json.Unmarshal(movies, &movieList); err != nil {
			writeError(w, http.StatusBadGateway, "trakt: invalid watched movies response: "+err.Error())
			return
		}
		if err := json.Unmarshal(shows, &showList); err != nil {
			writeError(w, http.StatusBadGateway, "trakt: invalid watched shows response: "+err.Error())
			return
		}

		resp := map[string]any{
			"synced":        "watched",
			"movies_count":  len(movieList),
			"shows_count":   len(showList),
			"connection_id": conn.ID,
		}

		// Auto-archive watched items if an archiver is available.
		if archiver != nil {
			if n, err := archiver.ArchiveWatchedMovies(r.Context(), movies); err == nil {
				resp["movies_archived"] = n
			}
			if n, err := archiver.ArchiveWatchedShows(r.Context(), shows); err == nil {
				resp["shows_archived"] = n
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func handleSyncCollection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, status, msg := getTraktConnection(svc, r)
		if conn == nil {
			writeError(w, status, msg)
			return
		}

		movies, err := traktAPIGet(r, conn.Settings, "/sync/collection/movies")
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		shows, err := traktAPIGet(r, conn.Settings, "/sync/collection/shows")
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		var movieList, showList []any
		if err := json.Unmarshal(movies, &movieList); err != nil {
			writeError(w, http.StatusBadGateway, "trakt: invalid collection movies response: "+err.Error())
			return
		}
		if err := json.Unmarshal(shows, &showList); err != nil {
			writeError(w, http.StatusBadGateway, "trakt: invalid collection shows response: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"synced":        "collection",
			"movies_count":  len(movieList),
			"shows_count":   len(showList),
			"connection_id": conn.ID,
		})
	}
}

func handleSyncWatchlist(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, status, msg := getTraktConnection(svc, r)
		if conn == nil {
			writeError(w, status, msg)
			return
		}

		movies, err := traktAPIGet(r, conn.Settings, "/users/me/watchlist/movies")
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		shows, err := traktAPIGet(r, conn.Settings, "/users/me/watchlist/shows")
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		var movieList, showList []any
		if err := json.Unmarshal(movies, &movieList); err != nil {
			writeError(w, http.StatusBadGateway, "trakt: invalid watchlist movies response: "+err.Error())
			return
		}
		if err := json.Unmarshal(shows, &showList); err != nil {
			writeError(w, http.StatusBadGateway, "trakt: invalid watchlist shows response: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"synced":        "watchlist",
			"movies_count":  len(movieList),
			"shows_count":   len(showList),
			"connection_id": conn.ID,
		})
	}
}
