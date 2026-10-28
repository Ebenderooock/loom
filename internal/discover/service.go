package discover

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

const (
	maxSeeds       = 16
	maxCollections = 8
	maxPeople      = 4
	maxRelated     = 4
	maxResults     = 60
	cacheTTL       = 6 * time.Hour
	maxStaleAge    = 7 * 24 * time.Hour
	maxCacheItems  = 512
)

var ErrNotConfigured = errors.New("TMDB provider is not configured")
var ErrProvider = errors.New("TMDB provider unavailable")

// Narrow interfaces keep discovery read-only and use the real media services.
type MovieLibrary interface {
	ListMovies(context.Context, int, int) ([]*movies.Movie, error)
	ListMovieFiles(context.Context, string) ([]*movies.MovieFile, error)
}

type SeriesLibrary interface {
	ListSeries(context.Context) ([]*series.Series, error)
}

type cacheEntry struct {
	value any // immutable provider response; never includes user/library state
	at    time.Time
}

type Service struct {
	client *tmdb.Client
	movies MovieLibrary
	series SeriesLibrary
	store  *Store
	gate   chan struct{}
	mu     sync.Mutex
	cache  map[string]cacheEntry
	now    func() time.Time
}

func NewService(client *tmdb.Client, movies MovieLibrary, series SeriesLibrary, store *Store) *Service {
	return &Service{client: client, movies: movies, series: series, store: store,
		gate: make(chan struct{}, 1), cache: make(map[string]cacheEntry), now: time.Now}
}

// begin bounds queue time and total provider work. Serialization also coalesces
// concurrent cache misses without creating unbounded background refreshes.
func (s *Service) begin(ctx context.Context) (context.Context, func(), error) {
	if !s.client.Configured() {
		return nil, nil, ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	select {
	case s.gate <- struct{}{}:
		return ctx, func() { <-s.gate; cancel() }, nil
	case <-ctx.Done():
		cancel()
		return nil, nil, ctx.Err()
	}
}

type catalogSession struct {
	s *Service
	ResultStatus
	successes int
	failures  int
}

func (s *Service) session() *catalogSession {
	return &catalogSession{s: s, ResultStatus: ResultStatus{
		Warnings: []string{}, GeneratedAt: s.now().UTC(),
	}}
}

func (c *catalogSession) warn(message string) {
	for _, w := range c.Warnings {
		if w == message {
			return
		}
	}
	c.Warnings = append(c.Warnings, message)
}

func (c *catalogSession) status() ResultStatus {
	sort.Strings(c.Warnings)
	return c.ResultStatus
}

func (c *catalogSession) resultError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c.failures > 0 && c.successes == 0 {
		return ErrProvider
	}
	return nil
}

func fetch[T any](c *catalogSession, ctx context.Context, key string, load func(context.Context) (*T, error)) (*T, error) {
	s := c.s
	now := s.now()
	s.mu.Lock()
	cached, ok := s.cache[key]
	s.mu.Unlock()
	record := func(at time.Time) {
		c.successes++
		if at.Before(c.GeneratedAt) {
			c.GeneratedAt = at.UTC()
		}
	}
	if ok && now.Sub(cached.at) < cacheTTL {
		record(cached.at)
		return cached.value.(*T), nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	value, err := load(callCtx)
	cancel()
	if err != nil {
		c.failures++
		// Only fixed labels and typed error codes reach clients. Provider bodies,
		// credentials, request URLs and transport errors must not be echoed.
		code := "unavailable"
		var providerErr *tmdb.ClientError
		if errors.As(err, &providerErr) {
			code = string(providerErr.Code)
		}
		c.warn("TMDB " + key + ": " + code)
		// A genuine 404 must stay a 404 (especially arbitrary collection IDs).
		// Invalid credentials and invalid requests also must not masquerade as
		// transient outages backed by a previously valid catalog response.
		transient := providerErr == nil || (providerErr.Code != tmdb.ErrCodeNotFound &&
			providerErr.Code != tmdb.ErrCodeUnauthorized && providerErr.Code != tmdb.ErrCodeClientError)
		if transient && ok && now.Sub(cached.at) < maxStaleAge {
			c.Stale = true
			c.warn("Using stale TMDB catalog data")
			record(cached.at)
			return cached.value.(*T), nil
		}
		return nil, err
	}
	s.mu.Lock()
	if _, replacing := s.cache[key]; !replacing && len(s.cache) >= maxCacheItems {
		oldestKey := ""
		var oldest time.Time
		for k, entry := range s.cache {
			if oldestKey == "" || entry.at.Before(oldest) || (entry.at.Equal(oldest) && k < oldestKey) {
				oldestKey, oldest = k, entry.at
			}
		}
		delete(s.cache, oldestKey)
	}
	s.cache[key] = cacheEntry{value: value, at: now}
	s.mu.Unlock()
	record(now)
	return value, nil
}

type libraryTitle struct {
	id        int
	mediaID   string
	title     string
	monitored bool
}

// Load ALL local provider IDs for exclusion, independent of the bounded seed
// sample used for external calls. Monitored titles without files are known too.
func (s *Service) library(ctx context.Context, mediaType string) (map[int]libraryTitle, error) {
	out := make(map[int]libraryTitle)
	add := func(raw *string, title libraryTitle) {
		if raw == nil {
			return
		}
		id, err := strconv.Atoi(*raw)
		if err != nil || id <= 0 {
			return
		}
		title.id = id
		if existing, ok := out[id]; !ok || title.mediaID < existing.mediaID {
			out[id] = title
		}
	}
	if mediaType == "movie" {
		if s.movies == nil {
			return nil, fmt.Errorf("movie library unavailable")
		}
		for offset := 0; ; offset += 1000 {
			page, err := s.movies.ListMovies(ctx, 1000, offset)
			if err != nil {
				return nil, err
			}
			for _, m := range page {
				if m == nil || m.DeletedAt != nil || m.MonitoringStatus == movies.MonitoringStatusDeleted {
					continue
				}
				add(m.TMDBID, libraryTitle{mediaID: m.ID, title: m.Title, monitored: m.MonitoringStatus == movies.MonitoringStatusMonitored})
			}
			if len(page) < 1000 {
				break
			}
		}
	} else {
		if s.series == nil {
			return nil, fmt.Errorf("series library unavailable")
		}
		list, err := s.series.ListSeries(ctx)
		if err != nil {
			return nil, err
		}
		for _, sr := range list {
			if sr == nil || sr.MonitoringStatus == "deleted" {
				continue
			}
			add(sr.TMDBID, libraryTitle{mediaID: sr.ID, title: sr.Title})
		}
	}
	return out, nil
}

func seeds(c *catalogSession, library map[int]libraryTitle) []libraryTitle {
	ids := make([]int, 0, len(library))
	for id := range library {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	if len(ids) > maxSeeds {
		c.warn("Library sources sampled: using the first 16 TMDB IDs; all library titles are excluded")
		ids = ids[:maxSeeds]
	}
	out := make([]libraryTitle, 0, len(ids))
	for _, id := range ids {
		out = append(out, library[id])
	}
	return out
}

type seedDetail struct {
	libraryTitle
	genres     []tmdb.GenreResponse
	credits    tmdb.CreditsResponse
	collection *tmdb.CollectionReference
}

func details(c *catalogSession, ctx context.Context, mediaType string, library map[int]libraryTitle) []seedDetail {
	out := []seedDetail{}
	for _, seed := range seeds(c, library) {
		if mediaType == "movie" {
			m, err := fetch(c, ctx, fmt.Sprintf("movie/%d", seed.id), func(ctx context.Context) (*tmdb.MovieResponse, error) {
				return c.s.client.GetMovieDetails(ctx, seed.id)
			})
			if err == nil && !m.Adult {
				out = append(out, seedDetail{seed, m.Genres, m.Credits, m.Collection})
			}
		} else {
			tv, err := fetch(c, ctx, fmt.Sprintf("series/%d", seed.id), func(ctx context.Context) (*tmdb.TVResponse, error) {
				return c.s.client.GetTVDetails(ctx, seed.id)
			})
			if err == nil && !tv.Adult {
				out = append(out, seedDetail{seed, tv.Genres, tv.Credits, nil})
			}
		}
	}
	return out
}

func collectionIDs(c *catalogSession, seeds []seedDetail) []int {
	set := make(map[int]bool)
	for _, seed := range seeds {
		if seed.collection != nil && seed.collection.ID > 0 {
			set[seed.collection.ID] = true
		}
	}
	ids := make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	if len(ids) > maxCollections {
		c.warn("Collection sources sampled: using the first 8 identified TMDB sets")
		ids = ids[:maxCollections]
	}
	return ids
}

func getCollection(c *catalogSession, ctx context.Context, id int) (*tmdb.CollectionResponse, error) {
	return fetch(c, ctx, fmt.Sprintf("collection/%d", id), func(ctx context.Context) (*tmdb.CollectionResponse, error) {
		return c.s.client.GetCollection(ctx, id)
	})
}
