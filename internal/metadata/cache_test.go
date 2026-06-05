package metadata

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCacheGetSet(t *testing.T) {
	c := NewCache()
	defer c.Close()

	// Set and get
	val := &MovieMetadata{Title: "Test"}
	c.Set("key1", val, 10*time.Minute)

	retrieved, ok := c.Get("key1")
	if !ok {
		t.Errorf("Expected value in cache, got nothing")
	}
	if m, _ := retrieved.(*MovieMetadata); m.Title != "Test" {
		t.Errorf("Expected Title=Test, got %s", m.Title)
	}

	// Get nonexistent
	_, ok = c.Get("nonexistent")
	if ok {
		t.Errorf("Expected no value for nonexistent key")
	}
}

func TestCacheExpiration(t *testing.T) {
	c := NewCache()
	defer c.Close()

	// Set with very short TTL
	c.Set("expire", "data", 10*time.Millisecond)

	// Immediately retrieve
	_, ok := c.Get("expire")
	if !ok {
		t.Errorf("Expected value to exist immediately after set")
	}

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Manual cleanup
	c.cleanup()

	// Should be gone
	_, ok = c.Get("expire")
	if ok {
		t.Errorf("Expected value to be expired")
	}
}

func TestCacheDelete(t *testing.T) {
	c := NewCache()
	defer c.Close()

	c.Set("key1", "value1", 10*time.Minute)
	c.Delete("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Errorf("Expected value to be deleted")
	}
}

func TestCacheClear(t *testing.T) {
	c := NewCache()
	defer c.Close()

	c.Set("key1", "value1", 10*time.Minute)
	c.Set("key2", "value2", 10*time.Minute)

	if c.Size() != 2 {
		t.Errorf("Expected size 2, got %d", c.Size())
	}

	c.Clear()

	if c.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", c.Size())
	}
}

func TestCacheRaceCondition(t *testing.T) {
	c := NewCache()
	defer c.Close()

	// Spawn multiple goroutines accessing cache concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				c.Set("key", "value", 10*time.Minute)
				_, _ = c.Get("key")
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMovieSearchKey(t *testing.T) {
	svc := NewService(nil, nil)

	// Same inputs should produce same key
	key1 := svc.movieSearchKey("Inception", 2010)
	key2 := svc.movieSearchKey("Inception", 2010)
	if key1 != key2 {
		t.Errorf("Expected same key for identical inputs")
	}

	// Different inputs should produce different keys
	key3 := svc.movieSearchKey("Inception", 2011)
	if key1 == key3 {
		t.Errorf("Expected different keys for different inputs")
	}
}

func TestSeriesSearchKey(t *testing.T) {
	svc := NewService(nil, nil)

	// Same input should produce same key
	key1 := svc.seriesSearchKey("Breaking Bad")
	key2 := svc.seriesSearchKey("Breaking Bad")
	if key1 != key2 {
		t.Errorf("Expected same key for identical inputs")
	}

	// Different inputs should produce different keys
	key3 := svc.seriesSearchKey("Breaking Good")
	if key1 == key3 {
		t.Errorf("Expected different keys for different inputs")
	}
}

// MockRepository implements Repository for testing
type MockRepository struct {
	mu       sync.Mutex
	movies   map[string]*MovieMetadata
	series   map[string]*SeriesMetadata
	episodes map[string]*EpisodeMetadata
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		movies:   make(map[string]*MovieMetadata),
		series:   make(map[string]*SeriesMetadata),
		episodes: make(map[string]*EpisodeMetadata),
	}
}

func (m *MockRepository) GetMovie(ctx context.Context, id string) (*MovieMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.movies[id], nil
}

func (m *MockRepository) PutMovie(ctx context.Context, id string, movie *MovieMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.movies[id] = movie
	return nil
}

func (m *MockRepository) GetMovieByExternalID(ctx context.Context, idType, idValue string) (*MovieMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, movie := range m.movies {
		switch idType {
		case "tmdb":
			if movie.TMDBID != nil && *movie.TMDBID == idValue {
				return movie, nil
			}
		case "imdb":
			if movie.IMDBID != nil && *movie.IMDBID == idValue {
				return movie, nil
			}
		case "tvdb":
			if movie.TVDBID != nil && *movie.TVDBID == idValue {
				return movie, nil
			}
		}
	}
	return nil, nil
}

func (m *MockRepository) GetSeries(ctx context.Context, id string) (*SeriesMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.series[id], nil
}

func (m *MockRepository) PutSeries(ctx context.Context, id string, series *SeriesMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.series[id] = series
	return nil
}

func (m *MockRepository) GetSeriesByExternalID(ctx context.Context, idType, idValue string) (*SeriesMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, series := range m.series {
		switch idType {
		case "tmdb":
			if series.TMDBID != nil && *series.TMDBID == idValue {
				return series, nil
			}
		case "imdb":
			if series.IMDBID != nil && *series.IMDBID == idValue {
				return series, nil
			}
		case "tvdb":
			if series.TVDBID != nil && *series.TVDBID == idValue {
				return series, nil
			}
		}
	}
	return nil, nil
}

func (m *MockRepository) GetEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := episodeKey(seriesID, season, episode)
	return m.episodes[key], nil
}

func (m *MockRepository) PutEpisode(ctx context.Context, id string, seriesID string, season, episode int, ep *EpisodeMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := episodeKey(seriesID, season, episode)
	m.episodes[key] = ep
	return nil
}

func (m *MockRepository) DeleteExpiredMovies(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *MockRepository) DeleteExpiredSeries(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *MockRepository) DeleteExpiredEpisodes(ctx context.Context) (int64, error) {
	return 0, nil
}

func episodeKey(seriesID string, season, episode int) string {
	return seriesID + ":" + string(rune(season)) + ":" + string(rune(episode))
}

// MockProvider implements MetadataProvider for testing
type MockProvider struct {
	Name_        string
	MovieError   error
	SeriesError  error
	MovieResult  []*MovieMetadata
	SeriesResult []*SeriesMetadata
}

func (m *MockProvider) Name() string {
	return m.Name_
}

func (m *MockProvider) FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*MovieMetadata, error) {
	if m.MovieError != nil {
		return nil, m.MovieError
	}
	return m.MovieResult, nil
}

func (m *MockProvider) FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*SeriesMetadata, error) {
	if m.SeriesError != nil {
		return nil, m.SeriesError
	}
	return m.SeriesResult, nil
}

func (m *MockProvider) FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*EpisodeMetadata, error) {
	return nil, nil
}

func TestServiceCacheHit(t *testing.T) {
	repo := NewMockRepository()
	provider := &MockProvider{Name_: "mock"}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	// Pre-populate cache
	tmdbID := "550"
	movie := &MovieMetadata{
		TMDBID: &tmdbID,
		Title:  "Fight Club",
		Year:   1999,
	}
	svc.cache.Set("movie:tmdb:550", movie, TTLFullDetails)

	// FindMovie should hit cache
	result, err := svc.FindMovie(context.Background(), SearchMovieParams{
		TMDBID: tmdbID,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Errorf("Expected movie result from cache")
	}
	if result.Title != "Fight Club" {
		t.Errorf("Expected title Fight Club, got %s", result.Title)
	}
}

func TestServiceDBHit(t *testing.T) {
	tmdbID := "550"
	movie := &MovieMetadata{
		TMDBID: &tmdbID,
		Title:  "Fight Club",
		Year:   1999,
	}

	repo := NewMockRepository()
	repo.movies["tmdb:550"] = movie

	provider := &MockProvider{Name_: "mock"}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	// FindMovie should hit database
	result, err := svc.FindMovie(context.Background(), SearchMovieParams{
		TMDBID: tmdbID,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Errorf("Expected movie result from database")
	}
	if result.Title != "Fight Club" {
		t.Errorf("Expected title Fight Club, got %s", result.Title)
	}

	// Verify cache was populated
	if _, ok := svc.cache.Get("movie:tmdb:550"); !ok {
		t.Errorf("Expected cache to be populated after DB hit")
	}
}

func TestServiceProviderHit(t *testing.T) {
	tmdbID := "550"
	movie := &MovieMetadata{
		TMDBID: &tmdbID,
		Title:  "Fight Club",
		Year:   1999,
	}

	repo := NewMockRepository()
	provider := &MockProvider{
		Name_:       "mock",
		MovieResult: []*MovieMetadata{movie},
	}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	result, err := svc.FindMovie(context.Background(), SearchMovieParams{
		Title: "Fight Club",
		Year:  1999,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Errorf("Expected movie result from provider")
	}
	if result.Title != "Fight Club" {
		t.Errorf("Expected title Fight Club, got %s", result.Title)
	}

	// Verify it was written to database
	if len(repo.movies) == 0 {
		t.Errorf("Expected movie to be written to database")
	}

	// Verify it was cached
	if _, ok := svc.cache.Get("movie:tmdb:550"); !ok {
		t.Errorf("Expected cache to be populated after provider hit")
	}
}

func TestServiceNoResult(t *testing.T) {
	repo := NewMockRepository()
	provider := &MockProvider{
		Name_: "mock",
		// No results
	}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	result, err := svc.FindMovie(context.Background(), SearchMovieParams{
		Title: "Nonexistent Movie",
		Year:  9999,
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result when no providers return data")
	}
}

func TestServiceRaceCondition(t *testing.T) {
	repo := NewMockRepository()

	// Create a provider that returns fresh metadata on each call
	// to avoid shared state between concurrent goroutines
	provider := &raceTestProvider{name: "mock"}

	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	// Spawn concurrent lookups
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = svc.FindMovie(context.Background(), SearchMovieParams{
				Title: "Fight Club",
				Year:  1999,
			})
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// raceTestProvider returns fresh metadata on each call to avoid shared state
type raceTestProvider struct {
	name string
}

func (p *raceTestProvider) Name() string {
	return p.name
}

func (p *raceTestProvider) FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*MovieMetadata, error) {
	id := "550"
	return []*MovieMetadata{
		{
			TMDBID: &id,
			Title:  "Fight Club",
			Year:   1999,
		},
	}, nil
}

func (p *raceTestProvider) FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*SeriesMetadata, error) {
	return nil, nil
}

func (p *raceTestProvider) FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*EpisodeMetadata, error) {
	return nil, nil
}
