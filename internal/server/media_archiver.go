package server

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

// mediaArchiver implements connect.MediaArchiver by bridging the connect,
// movies, series, and libraries modules.
type mediaArchiver struct {
	moviesSvc movies.Service
	seriesSvc series.Service
	libStore  *libraries.Store
}

// traktWatchedMovie is the minimal Trakt watched-movie JSON shape.
type traktWatchedMovie struct {
	LastWatchedAt string `json:"last_watched_at"`
	Movie         struct {
		IDs struct {
			TMDB int `json:"tmdb"`
		} `json:"ids"`
	} `json:"movie"`
}

// traktWatchedShow is the minimal Trakt watched-show JSON shape.
type traktWatchedShow struct {
	LastWatchedAt string `json:"last_watched_at"`
	Show          struct {
		IDs struct {
			TVDB int `json:"tvdb"`
			TMDB int `json:"tmdb"`
		} `json:"ids"`
	} `json:"show"`
}

// ArchiveWatchedMovies archives movies that match Trakt watched entries and
// whose library has auto_archive_watched enabled.
func (a *mediaArchiver) ArchiveWatchedMovies(ctx context.Context, raw json.RawMessage) (int, error) {
	var items []traktWatchedMovie
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, err
	}

	allMovies, err := a.moviesSvc.ListMovies(ctx, 0, 0)
	if err != nil {
		return 0, err
	}

	// Index local movies by TMDB ID string.
	byTMDB := make(map[string]*movies.Movie, len(allMovies))
	for _, m := range allMovies {
		if m.TMDBID != nil && *m.TMDBID != "" {
			byTMDB[*m.TMDBID] = m
		}
	}

	libCache := map[string]*libraries.Library{}

	archived := 0
	for _, item := range items {
		tmdbStr := strconv.Itoa(item.Movie.IDs.TMDB)
		m, ok := byTMDB[tmdbStr]
		if !ok || m.MonitoringStatus == movies.MonitoringStatusArchived {
			continue
		}

		lib, err := a.getLib(ctx, m.LibraryID, libCache)
		if err != nil || !lib.AutoArchiveWatched {
			continue
		}

		if lib.AutoArchiveDaysAfterWatch > 0 {
			watched, err := time.Parse(time.RFC3339, item.LastWatchedAt)
			if err != nil {
				continue
			}
			if time.Since(watched) < time.Duration(lib.AutoArchiveDaysAfterWatch)*24*time.Hour {
				continue
			}
		}

		if err := a.moviesSvc.SetMonitoringStatus(ctx, m.ID, movies.MonitoringStatusArchived); err == nil {
			archived++
		}
	}
	return archived, nil
}

// ArchiveWatchedShows archives series that match Trakt watched entries and
// whose library has auto_archive_watched enabled.
func (a *mediaArchiver) ArchiveWatchedShows(ctx context.Context, raw json.RawMessage) (int, error) {
	var items []traktWatchedShow
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, err
	}

	allSeries, err := a.seriesSvc.ListSeries(ctx)
	if err != nil {
		return 0, err
	}

	// Index local series by TVDB ID string.
	byTVDB := make(map[string]*series.Series, len(allSeries))
	for _, s := range allSeries {
		if s.TVDBID != nil && *s.TVDBID != "" {
			byTVDB[*s.TVDBID] = s
		}
	}

	libCache := map[string]*libraries.Library{}

	archived := 0
	for _, item := range items {
		tvdbStr := strconv.Itoa(item.Show.IDs.TVDB)
		sr, ok := byTVDB[tvdbStr]
		if !ok || sr.MonitoringStatus == series.MonitoringArchived {
			continue
		}

		lib, err := a.getLib(ctx, sr.LibraryID, libCache)
		if err != nil || !lib.AutoArchiveWatched {
			continue
		}

		if lib.AutoArchiveDaysAfterWatch > 0 {
			watched, err := time.Parse(time.RFC3339, item.LastWatchedAt)
			if err != nil {
				continue
			}
			if time.Since(watched) < time.Duration(lib.AutoArchiveDaysAfterWatch)*24*time.Hour {
				continue
			}
		}

		if err := a.seriesSvc.SetMonitoringStatus(ctx, sr.ID, series.MonitoringArchived); err == nil {
			archived++
		}
	}
	return archived, nil
}

func (a *mediaArchiver) getLib(ctx context.Context, id string, cache map[string]*libraries.Library) (*libraries.Library, error) {
	if lib, ok := cache[id]; ok {
		return lib, nil
	}
	lib, err := a.libStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	cache[id] = lib
	return lib, nil
}
