package metadata

import (
	"context"
	"crypto/md5"
	"fmt"
	"time"
)

// Service orchestrates metadata lookups across cache, database, and
// metadata providers. It implements a tiered search strategy:
//
// 1. Check in-process cache (TTL 30min for searches, 7d for full details)
// 2. Check database (previous lookups)
// 3. Query providers in order with per-provider timeout (3s each)
// 4. Write successful results to DB and cache
// 5. Return partial results or nil on timeout
//
// Metadata is immutable once cached; updates require explicit re-fetch
// via a provider API call (not cache invalidation).
type Service struct {
	repo      Repository
	providers []MetadataProvider
	cache     *Cache
}

// NewService builds a Service with the given repository, cache, and
// ordered list of metadata providers. Providers are queried left-to-right
// until a match is found or all timeout/fail.
func NewService(repo Repository, providers []MetadataProvider) *Service {
	return &Service{
		repo:      repo,
		providers: providers,
		cache:     NewCache(),
	}
}

// --- Movie methods -------------------------------------------------

// FindMovie searches for a movie by title+year or external IDs.
// Returns the first non-nil result from cache, DB, or providers.
// Returns nil if no providers yield a result or all timeout.
//
// Timeout strategy: 3s per provider, 10s total. If provider 1 succeeds
// but provider 2 times out, we return provider 1's result.
func (s *Service) FindMovie(ctx context.Context, params SearchMovieParams) (*MovieMetadata, error) {
	// Try cache by external ID first (full details, longer TTL)
	if params.TMDBID != "" {
		if m, ok := s.cache.Get("movie:tmdb:" + params.TMDBID); ok {
			return m.(*MovieMetadata), nil
		}
	}
	if params.IMDBID != "" {
		if m, ok := s.cache.Get("movie:imdb:" + params.IMDBID); ok {
			return m.(*MovieMetadata), nil
		}
	}
	if params.TVDBID != "" {
		if m, ok := s.cache.Get("movie:tvdb:" + params.TVDBID); ok {
			return m.(*MovieMetadata), nil
		}
	}

	// Try cache by search query only when we have a title (external-ID-only
	// lookups must not share the degenerate md5(":0") cache key).
	var searchKey string
	hasExternalID := params.TMDBID != "" || params.IMDBID != "" || params.TVDBID != ""
	if params.Title != "" && !hasExternalID {
		searchKey = s.movieSearchKey(params.Title, params.Year)
		if m, ok := s.cache.Get("search:movie:" + searchKey); ok {
			return m.(*MovieMetadata), nil
		}
	}

	// Try database by external ID
	if params.TMDBID != "" {
		if m, err := s.repo.GetMovieByExternalID(ctx, "tmdb", params.TMDBID); err != nil {
			return nil, err
		} else if m != nil {
			s.cacheMovie(m)
			return m, nil
		}
	}
	if params.IMDBID != "" {
		if m, err := s.repo.GetMovieByExternalID(ctx, "imdb", params.IMDBID); err != nil {
			return nil, err
		} else if m != nil {
			s.cacheMovie(m)
			return m, nil
		}
	}
	if params.TVDBID != "" {
		if m, err := s.repo.GetMovieByExternalID(ctx, "tvdb", params.TVDBID); err != nil {
			return nil, err
		} else if m != nil {
			s.cacheMovie(m)
			return m, nil
		}
	}

	// Query providers with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	externalIDs := map[string]string{}
	if params.IMDBID != "" {
		externalIDs["imdb"] = params.IMDBID
	}
	if params.TMDBID != "" {
		externalIDs["tmdb"] = params.TMDBID
	}
	if params.TVDBID != "" {
		externalIDs["tvdb"] = params.TVDBID
	}

	for _, provider := range s.providers {
		providerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		results, err := provider.FindMovie(providerCtx, params.Title, params.Year, externalIDs)
		cancel()

		if err != nil {
			// Provider timeout or error; continue to next
			continue
		}

		if len(results) > 0 && results[0] != nil {
			movie := results[0]
			// Cache by external ID if available
			if movie.TMDBID != nil {
				s.cache.Set("movie:tmdb:"+*movie.TMDBID, movie, TTLFullDetails)
			}
			if movie.IMDBID != nil {
				s.cache.Set("movie:imdb:"+*movie.IMDBID, movie, TTLFullDetails)
			}
			if movie.TVDBID != nil {
				s.cache.Set("movie:tvdb:"+*movie.TVDBID, movie, TTLFullDetails)
			}
			// Cache search result only when a title was the primary lookup key
			// (prevents the degenerate md5(":0") key from cross-contaminating
			// unrelated TMDB-ID-only lookups).
			if searchKey != "" {
				s.cache.Set("search:movie:"+searchKey, movie, TTLSearchResult)
			}
			// Write to database
			if err := s.putMovieWithID(ctx, movie); err != nil {
				// Log but don't fail; cache is sufficient fallback
				_ = err
			}
			return movie, nil
		}
	}

	return nil, nil
}

// FindSeries searches for a series by title or external IDs.
func (s *Service) FindSeries(ctx context.Context, params SearchSeriesParams) (*SeriesMetadata, error) {
	// Try cache by external ID first
	if params.TMDBID != "" {
		if ser, ok := s.cache.Get("series:tmdb:" + params.TMDBID); ok {
			return ser.(*SeriesMetadata), nil
		}
	}
	if params.IMDBID != "" {
		if ser, ok := s.cache.Get("series:imdb:" + params.IMDBID); ok {
			return ser.(*SeriesMetadata), nil
		}
	}
	if params.TVDBID != "" {
		if ser, ok := s.cache.Get("series:tvdb:" + params.TVDBID); ok {
			return ser.(*SeriesMetadata), nil
		}
	}

	// Try cache by search query only when a title was provided.
	hasSeriesExternalID := params.TMDBID != "" || params.IMDBID != "" || params.TVDBID != ""
	if params.Title != "" && !hasSeriesExternalID {
		searchKey := s.seriesSearchKey(params.Title)
		if ser, ok := s.cache.Get("search:series:" + searchKey); ok {
			return ser.(*SeriesMetadata), nil
		}
	}

	// Try database by external ID
	if params.TMDBID != "" {
		if ser, err := s.repo.GetSeriesByExternalID(ctx, "tmdb", params.TMDBID); err != nil {
			return nil, err
		} else if ser != nil {
			s.cacheSeriesData(ser)
			return ser, nil
		}
	}
	if params.IMDBID != "" {
		if ser, err := s.repo.GetSeriesByExternalID(ctx, "imdb", params.IMDBID); err != nil {
			return nil, err
		} else if ser != nil {
			s.cacheSeriesData(ser)
			return ser, nil
		}
	}
	if params.TVDBID != "" {
		if ser, err := s.repo.GetSeriesByExternalID(ctx, "tvdb", params.TVDBID); err != nil {
			return nil, err
		} else if ser != nil {
			s.cacheSeriesData(ser)
			return ser, nil
		}
	}

	// Query providers with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	externalIDs := map[string]string{}
	if params.IMDBID != "" {
		externalIDs["imdb"] = params.IMDBID
	}
	if params.TMDBID != "" {
		externalIDs["tmdb"] = params.TMDBID
	}
	if params.TVDBID != "" {
		externalIDs["tvdb"] = params.TVDBID
	}

	for _, provider := range s.providers {
		providerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		results, err := provider.FindSeries(providerCtx, params.Title, externalIDs)
		cancel()

		if err != nil {
			continue
		}

		if len(results) > 0 && results[0] != nil {
			series := results[0]
			s.cacheSeriesData(series)
			// Write to database
			if err := s.putSeriesWithID(ctx, series); err != nil {
				_ = err
			}
			return series, nil
		}
	}

	return nil, nil
}

// FindEpisode searches for an episode by series ID and season/episode numbers.
func (s *Service) FindEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error) {
	// Cache key combines series ID with season/episode
	cacheKey := fmt.Sprintf("episode:%s:S%dE%d", seriesID, season, episode)

	// Try in-process cache first
	if ep, ok := s.cache.Get(cacheKey); ok {
		return ep.(*EpisodeMetadata), nil
	}

	// Try database
	if ep, err := s.repo.GetEpisode(ctx, seriesID, season, episode); err != nil {
		return nil, err
	} else if ep != nil {
		s.cache.Set(cacheKey, ep, TTLFullDetails)
		return ep, nil
	}

	// Query providers
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, provider := range s.providers {
		providerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ep, err := provider.FindEpisode(providerCtx, seriesID, season, episode)
		cancel()

		if err != nil {
			continue
		}

		if ep != nil {
			// Cache and write to database
			s.cache.Set(cacheKey, ep, TTLFullDetails)
			if err := s.putEpisodeWithID(ctx, seriesID, season, episode, ep); err != nil {
				_ = err
			}
			return ep, nil
		}
	}

	return nil, nil
}

// FindMovieByQuery searches for movies matching a query and returns multiple results.
// Used for UI search results, returns first 10 results.
func (s *Service) FindMovieByQuery(ctx context.Context, query string, year int) ([]*MovieMetadata, error) {
	results := make([]*MovieMetadata, 0)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, provider := range s.providers {
		providerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		movies, err := provider.FindMovie(providerCtx, query, year, nil)
		cancel()

		if err != nil {
			continue
		}

		results = append(results, movies...)
		if len(results) >= 10 {
			break
		}
	}

	return results, nil
}

// FindMovieByTMDBID looks up a single movie by its TMDB ID.
func (s *Service) FindMovieByTMDBID(ctx context.Context, tmdbID string) (*MovieMetadata, error) {
	return s.FindMovie(ctx, SearchMovieParams{TMDBID: tmdbID})
}

// FindSeriesByQuery searches for series matching a query and returns multiple results.
// Used for UI search results, returns first 10 results.
func (s *Service) FindSeriesByQuery(ctx context.Context, query string) ([]*SeriesMetadata, error) {
	results := make([]*SeriesMetadata, 0)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, provider := range s.providers {
		providerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		series, err := provider.FindSeries(providerCtx, query, nil)
		cancel()

		if err != nil {
			continue
		}

		results = append(results, series...)
		if len(results) >= 10 {
			break
		}
	}

	return results, nil
}

// --- Private helpers -----------------------------------------------

// movieSearchKey generates a cache key for movie search by title+year.
func (s *Service) movieSearchKey(title string, year int) string {
	hash := md5.Sum([]byte(fmt.Sprintf("%s:%d", title, year)))
	return fmt.Sprintf("%x", hash)
}

// seriesSearchKey generates a cache key for series search by title.
func (s *Service) seriesSearchKey(title string) string {
	hash := md5.Sum([]byte(title))
	return fmt.Sprintf("%x", hash)
}

// cacheMovie caches a movie by all available external IDs.
func (s *Service) cacheMovie(m *MovieMetadata) {
	if m.TMDBID != nil {
		s.cache.Set("movie:tmdb:"+*m.TMDBID, m, TTLFullDetails)
	}
	if m.IMDBID != nil {
		s.cache.Set("movie:imdb:"+*m.IMDBID, m, TTLFullDetails)
	}
	if m.TVDBID != nil {
		s.cache.Set("movie:tvdb:"+*m.TVDBID, m, TTLFullDetails)
	}
}

// cacheSeriesData caches a series by all available external IDs.
func (s *Service) cacheSeriesData(ser *SeriesMetadata) {
	if ser.TMDBID != nil {
		s.cache.Set("series:tmdb:"+*ser.TMDBID, ser, TTLFullDetails)
	}
	if ser.IMDBID != nil {
		s.cache.Set("series:imdb:"+*ser.IMDBID, ser, TTLFullDetails)
	}
	if ser.TVDBID != nil {
		s.cache.Set("series:tvdb:"+*ser.TVDBID, ser, TTLFullDetails)
	}
}

// putMovieWithID persists a movie to the database. The ID is derived from
// the first available external ID for uniqueness.
func (s *Service) putMovieWithID(ctx context.Context, m *MovieMetadata) error {
	var id string
	switch {
	case m.TMDBID != nil:
		id = "tmdb:" + *m.TMDBID
	case m.IMDBID != nil:
		id = "imdb:" + *m.IMDBID
	case m.TVDBID != nil:
		id = "tvdb:" + *m.TVDBID
	default:
		// No external ID; skip persistence
		return nil
	}
	m.CachedAt = time.Now()
	return s.repo.PutMovie(ctx, id, m)
}

// putSeriesWithID persists a series to the database.
func (s *Service) putSeriesWithID(ctx context.Context, ser *SeriesMetadata) error {
	var id string
	switch {
	case ser.TMDBID != nil:
		id = "tvdb:" + *ser.TVDBID // Use TVDB for series for consistency
		if ser.TVDBID == nil {
			id = "tmdb:" + *ser.TMDBID
		}
	case ser.IMDBID != nil:
		id = "imdb:" + *ser.IMDBID
	case ser.TVDBID != nil:
		id = "tvdb:" + *ser.TVDBID
	default:
		return nil
	}
	ser.CachedAt = time.Now()
	return s.repo.PutSeries(ctx, id, ser)
}

// putEpisodeWithID persists an episode to the database.
func (s *Service) putEpisodeWithID(ctx context.Context, seriesID string, season, episode int, ep *EpisodeMetadata) error {
	var id string
	switch {
	case ep.TVDBID != nil:
		id = "tvdb:" + *ep.TVDBID
	case ep.TMDBID != nil:
		id = "tmdb:" + *ep.TMDBID
	default:
		id = fmt.Sprintf("%s:S%dE%d", seriesID, season, episode)
	}
	ep.CachedAt = time.Now()
	return s.repo.PutEpisode(ctx, id, seriesID, season, episode, ep)
}

// Close shuts down the service's cache cleanup goroutine.
func (s *Service) Close() error {
	s.cache.Close()
	return nil
}
