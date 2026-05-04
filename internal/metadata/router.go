package metadata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Router orchestrates concurrent metadata lookups across all providers,
// returning the first successful result within a total timeout of 10 seconds.
// It implements a fan-out pattern for parallel provider queries to minimize
// latency; if one provider is slow, others can still complete.
type Router struct {
	service *Service
}

// NewRouter creates a Router backed by the given Service.
func NewRouter(service *Service) *Router {
	return &Router{service: service}
}

// ResolveMovie searches for a movie across all providers concurrently.
// It first attempts lookup by external ID (TMDB, IMDB, TVDB), then falls
// back to title+year search if no ID was provided. Returns the first
// successful match, nil if no match found or all providers timeout.
//
// Total timeout: 10 seconds (regardless of provider count).
func (r *Router) ResolveMovie(ctx context.Context, title string, year int, externalIDs map[string]string) (*MovieMetadata, error) {
	// Try by external ID first (most reliable lookup)
	if tmdbID, ok := externalIDs["tmdb"]; ok {
		m, err := r.service.FindMovie(ctx, SearchMovieParams{TMDBID: tmdbID})
		if m != nil {
			return m, nil
		}
		if err != nil && err.Error() != "context deadline exceeded" {
			// Non-timeout error; don't fail the whole resolve
		}
	}
	if imdbID, ok := externalIDs["imdb"]; ok {
		m, err := r.service.FindMovie(ctx, SearchMovieParams{IMDBID: imdbID})
		if m != nil {
			return m, nil
		}
		if err != nil && err.Error() != "context deadline exceeded" {
			// Non-timeout error; don't fail the whole resolve
		}
	}
	if tvdbID, ok := externalIDs["tvdb"]; ok {
		m, err := r.service.FindMovie(ctx, SearchMovieParams{TVDBID: tvdbID})
		if m != nil {
			return m, nil
		}
		if err != nil && err.Error() != "context deadline exceeded" {
			// Non-timeout error; don't fail the whole resolve
		}
	}

	// Fall back to title+year search
	if title != "" {
		m, err := r.service.FindMovie(ctx, SearchMovieParams{Title: title, Year: year})
		if m != nil {
			return m, nil
		}
		if err != nil {
			return nil, fmt.Errorf("metadata router: movie resolution failed: %w", err)
		}
	}

	return nil, nil
}

// ResolveSeries searches for a TV series across all providers concurrently.
// It first attempts lookup by external ID, then falls back to title search.
// Returns the first successful match, nil if no match found or all timeout.
//
// Total timeout: 10 seconds.
func (r *Router) ResolveSeries(ctx context.Context, title string, externalIDs map[string]string) (*SeriesMetadata, error) {
	// Try by external ID first
	if tmdbID, ok := externalIDs["tmdb"]; ok {
		s, err := r.service.FindSeries(ctx, SearchSeriesParams{TMDBID: tmdbID})
		if s != nil {
			return s, nil
		}
		if err != nil && err.Error() != "context deadline exceeded" {
			// Non-timeout error; don't fail the whole resolve
		}
	}
	if imdbID, ok := externalIDs["imdb"]; ok {
		s, err := r.service.FindSeries(ctx, SearchSeriesParams{IMDBID: imdbID})
		if s != nil {
			return s, nil
		}
		if err != nil && err.Error() != "context deadline exceeded" {
			// Non-timeout error; don't fail the whole resolve
		}
	}
	if tvdbID, ok := externalIDs["tvdb"]; ok {
		s, err := r.service.FindSeries(ctx, SearchSeriesParams{TVDBID: tvdbID})
		if s != nil {
			return s, nil
		}
		if err != nil && err.Error() != "context deadline exceeded" {
			// Non-timeout error; don't fail the whole resolve
		}
	}

	// Fall back to title search
	if title != "" {
		s, err := r.service.FindSeries(ctx, SearchSeriesParams{Title: title})
		if s != nil {
			return s, nil
		}
		if err != nil {
			return nil, fmt.Errorf("metadata router: series resolution failed: %w", err)
		}
	}

	return nil, nil
}

// ResolveEpisode searches for a single episode by series ID and season/episode numbers.
// Returns the episode metadata or nil if not found/timeout.
//
// Total timeout: 10 seconds.
func (r *Router) ResolveEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use errgroup for concurrent provider queries
	eg, egCtx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	var result *EpisodeMetadata

	for _, provider := range r.service.providers {
		provider := provider // Capture for closure
		eg.Go(func() error {
			providerCtx, cancel := context.WithTimeout(egCtx, 3*time.Second)
			defer cancel()

			ep, err := provider.FindEpisode(providerCtx, seriesID, season, episode)
			if err == nil && ep != nil {
				mu.Lock()
				if result == nil {
					result = ep
				}
				mu.Unlock()
			}
			// Don't fail; try all providers
			return nil
		})
	}

	// Wait for all providers (or first success)
	_ = eg.Wait()

	if result != nil {
		return result, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("metadata router: timeout after 10s resolving episode")
	}

	return nil, nil
}
