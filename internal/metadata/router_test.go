package metadata

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider is a test implementation of MetadataProvider.
type mockProvider struct {
	name                 string
	findMovieResults     []*MovieMetadata
	findMovieErr         error
	findMovieDelay       time.Duration
	findMovieCalls       atomic.Int32
	findSeriesResults    []*SeriesMetadata
	findSeriesErr        error
	findSeriesDelay      time.Duration
	findSeriesCalls      atomic.Int32
	findEpisodeResult    *EpisodeMetadata
	findEpisodeErr       error
	findEpisodeDelay     time.Duration
	findEpisodeCalls     atomic.Int32
	findEpisodeDelayOnce bool // Only delay first call
	callCount            int32
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*MovieMetadata, error) {
	m.findMovieCalls.Add(1)
	if m.findMovieDelay > 0 {
		select {
		case <-time.After(m.findMovieDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.findMovieErr != nil {
		return nil, m.findMovieErr
	}
	return m.findMovieResults, nil
}

func (m *mockProvider) FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*SeriesMetadata, error) {
	m.findSeriesCalls.Add(1)
	if m.findSeriesDelay > 0 {
		select {
		case <-time.After(m.findSeriesDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.findSeriesErr != nil {
		return nil, m.findSeriesErr
	}
	return m.findSeriesResults, nil
}

func (m *mockProvider) FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*EpisodeMetadata, error) {
	m.findEpisodeCalls.Add(1)
	if m.findEpisodeDelay > 0 && (!m.findEpisodeDelayOnce || m.findEpisodeCalls.Load() == 1) {
		select {
		case <-time.After(m.findEpisodeDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.findEpisodeErr != nil {
		return nil, m.findEpisodeErr
	}
	return m.findEpisodeResult, nil
}

// Test: ResolveMovie with movie found by external ID
func TestResolveMovie_ByID(t *testing.T) {
	movieID := "12345"
	expected := &MovieMetadata{
		TMDBID: &movieID,
		Title:  "Inception",
		Year:   2010,
	}

	provider := &mockProvider{
		name:             "test",
		findMovieResults: []*MovieMetadata{expected},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveMovie(ctx, "Inception", 2010, map[string]string{"tmdb": movieID})
	if err != nil {
		t.Fatalf("ResolveMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result, got nil")
	}
	if result.Title != expected.Title {
		t.Errorf("expected title %q, got %q", expected.Title, result.Title)
	}
}

// Test: ResolveMovie fallback to search
func TestResolveMovie_SearchFallback(t *testing.T) {
	expected := &MovieMetadata{
		Title: "The Matrix",
		Year:  1999,
	}

	provider := &mockProvider{
		name:             "test",
		findMovieResults: []*MovieMetadata{expected},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveMovie(ctx, "The Matrix", 1999, map[string]string{})
	if err != nil {
		t.Fatalf("ResolveMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result, got nil")
	}
	if result.Title != expected.Title {
		t.Errorf("expected title %q, got %q", expected.Title, result.Title)
	}
}

// Test: ResolveMovie returns nil when no match
func TestResolveMovie_NoMatch(t *testing.T) {
	provider := &mockProvider{
		name:             "test",
		findMovieResults: []*MovieMetadata{},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveMovie(ctx, "NonexistentMovie", 2099, map[string]string{})
	if err != nil {
		t.Logf("error (expected): %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// Test: ResolveSeries happy path
func TestResolveSeries_ByID(t *testing.T) {
	seriesID := "67890"
	expected := &SeriesMetadata{
		TVDBID: &seriesID,
		Title:  "Breaking Bad",
	}

	provider := &mockProvider{
		name:             "test",
		findSeriesResults: []*SeriesMetadata{expected},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveSeries(ctx, "Breaking Bad", map[string]string{"tvdb": seriesID})
	if err != nil {
		t.Fatalf("ResolveSeries failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result, got nil")
	}
	if result.Title != expected.Title {
		t.Errorf("expected title %q, got %q", expected.Title, result.Title)
	}
}

// Test: ResolveSeries search fallback
func TestResolveSeries_SearchFallback(t *testing.T) {
	expected := &SeriesMetadata{
		Title: "Game of Thrones",
	}

	provider := &mockProvider{
		name:             "test",
		findSeriesResults: []*SeriesMetadata{expected},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveSeries(ctx, "Game of Thrones", map[string]string{})
	if err != nil {
		t.Fatalf("ResolveSeries failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result, got nil")
	}
	if result.Title != expected.Title {
		t.Errorf("expected title %q, got %q", expected.Title, result.Title)
	}
}

// Test: ResolveSeries returns nil when no match
func TestResolveSeries_NoMatch(t *testing.T) {
	provider := &mockProvider{
		name:              "test",
		findSeriesResults: []*SeriesMetadata{},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveSeries(ctx, "NonexistentSeries", map[string]string{})
	if err != nil {
		t.Logf("error (expected): %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// Test: ResolveEpisode happy path
func TestResolveEpisode_Found(t *testing.T) {
	expected := &EpisodeMetadata{
		Season:  5,
		Episode: 14,
		Title:   "Ozymandias",
	}

	provider := &mockProvider{
		name:              "test",
		findEpisodeResult: expected,
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveEpisode(ctx, "tvdb:123", 5, 14)
	if err != nil {
		t.Fatalf("ResolveEpisode failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result, got nil")
	}
	if result.Title != expected.Title {
		t.Errorf("expected title %q, got %q", expected.Title, result.Title)
	}
}

// Test: ResolveEpisode timeout (all providers slow)
func TestResolveEpisode_Timeout(t *testing.T) {
	slowProvider := &mockProvider{
		name:              "test",
		findEpisodeDelay:  5 * time.Second, // Slower than individual timeout
		findEpisodeResult: &EpisodeMetadata{Title: "Test"},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{slowProvider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Shorter timeout
	defer cancel()

	result, err := router.ResolveEpisode(ctx, "tvdb:123", 5, 14)
	if err == nil && result == nil {
		// Expected: timeout with no result
		return
	}
	if result != nil {
		t.Errorf("expected nil result on timeout, got %v", result)
	}
}

// Test: ResolveEpisode with partial results (first succeeds, second slow)
func TestResolveEpisode_PartialResults(t *testing.T) {
	expected := &EpisodeMetadata{
		Season:  5,
		Episode: 14,
		Title:   "Ozymandias",
	}

	fastProvider := &mockProvider{
		name:              "fast",
		findEpisodeResult: expected,
		findEpisodeDelay:  100 * time.Millisecond,
	}

	slowProvider := &mockProvider{
		name:             "slow",
		findEpisodeDelay: 5 * time.Second,
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{fastProvider, slowProvider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := router.ResolveEpisode(ctx, "tvdb:123", 5, 14)
	if err != nil {
		t.Logf("error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from fast provider")
	}
	if result.Title != expected.Title {
		t.Errorf("expected title %q, got %q", expected.Title, result.Title)
	}
}

// Test: Concurrent resolve calls are race-safe
func TestConcurrentResolves_RaceSafe(t *testing.T) {
	provider := &mockProvider{
		name: "test",
		findMovieResults: []*MovieMetadata{{
			Title: "Concurrent Test",
			Year:  2024,
		}},
		findEpisodeResult: &EpisodeMetadata{
			Title:   "Test Episode",
			Season:  1,
			Episode: 1,
		},
	}

	repo := &mockRepository{}
	svc := NewService(repo, []MetadataProvider{provider})
	defer svc.Close()

	router := NewRouter(svc)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Launch 10 concurrent resolve calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Mix different resolve types
			if idx%3 == 0 {
				_, err := router.ResolveMovie(ctx, "Test Movie", 2024, map[string]string{})
				if err != nil {
					errors <- fmt.Errorf("ResolveMovie error: %w", err)
				}
			} else if idx%3 == 1 {
				_, err := router.ResolveSeries(ctx, "Test Series", map[string]string{})
				if err != nil {
					errors <- fmt.Errorf("ResolveSeries error: %w", err)
				}
			} else {
				_, err := router.ResolveEpisode(ctx, "tvdb:123", 1, 1)
				if err != nil {
					errors <- fmt.Errorf("ResolveEpisode error: %w", err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		if err != nil {
			t.Errorf("concurrent call failed: %v", err)
		}
	}
}

// Test: Config loading from environment
func TestConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := DefaultConfig()
		if len(cfg.Providers) == 0 {
			t.Error("expected default providers to be set")
		}
		if cfg.Timeout <= 0 {
			t.Error("expected timeout to be positive")
		}
		if !cfg.CacheEnabled {
			t.Error("expected cache to be enabled by default")
		}
	})

	t.Run("env_overrides", func(t *testing.T) {
		t.Setenv("LOOM_METADATA_PROVIDERS", "custom1,custom2")
		t.Setenv("LOOM_METADATA_TIMEOUT", "5s")

		cfg := ConfigFromEnv()
		if len(cfg.Providers) != 2 || cfg.Providers[0] != "custom1" {
			t.Errorf("expected custom providers, got %v", cfg.Providers)
		}
		if cfg.Timeout != 5*time.Second {
			t.Errorf("expected 5s timeout, got %v", cfg.Timeout)
		}
	})
}

// mockRepository is a test implementation of Repository.
type mockRepository struct {
	movies   map[string]*MovieMetadata
	series   map[string]*SeriesMetadata
	episodes map[string]*EpisodeMetadata
	mu       sync.RWMutex
}

func (m *mockRepository) GetMovie(ctx context.Context, id string) (*MovieMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.movies[id], nil
}

func (m *mockRepository) GetMovieByExternalID(ctx context.Context, provider, id string) (*MovieMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.movies[provider+":"+id], nil
}

func (m *mockRepository) GetSeries(ctx context.Context, id string) (*SeriesMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.series[id], nil
}

func (m *mockRepository) GetSeriesByExternalID(ctx context.Context, provider, id string) (*SeriesMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.series[provider+":"+id], nil
}

func (m *mockRepository) GetEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:S%dE%d", seriesID, season, episode)
	return m.episodes[key], nil
}

func (m *mockRepository) PutMovie(ctx context.Context, id string, movie *MovieMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.movies == nil {
		m.movies = make(map[string]*MovieMetadata)
	}
	m.movies[id] = movie
	return nil
}

func (m *mockRepository) PutSeries(ctx context.Context, id string, series *SeriesMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.series == nil {
		m.series = make(map[string]*SeriesMetadata)
	}
	m.series[id] = series
	return nil
}

func (m *mockRepository) PutEpisode(ctx context.Context, id, seriesID string, season, episode int, ep *EpisodeMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.episodes == nil {
		m.episodes = make(map[string]*EpisodeMetadata)
	}
	key := fmt.Sprintf("%s:S%dE%d", seriesID, season, episode)
	m.episodes[key] = ep
	return nil
}

func (m *mockRepository) DeleteExpiredMovies(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockRepository) DeleteExpiredSeries(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockRepository) DeleteExpiredEpisodes(ctx context.Context) (int64, error) {
	return 0, nil
}
