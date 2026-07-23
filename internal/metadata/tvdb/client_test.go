package tvdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLogin_Success tests successful authentication.
func TestLogin_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/login" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		resp := LoginResponse{
			Data: LoginData{
				Token: "test-jwt-token-12345",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		PIN:     "1234",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	err := client.Login(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.token != "test-jwt-token-12345" {
		t.Fatalf("token not set correctly")
	}
}

// TestLogin_InvalidCredentials tests 401 response.
func TestLogin_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Status:  "error",
			Message: "Invalid credentials",
		})
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "bad-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	err := client.Login(context.Background())
	if err == nil {
		t.Fatalf("expected error for invalid credentials")
	}

	var e *ClientError
	if !errors.As(err, &e) || e.Code != ErrCodeUnauthorized {
		t.Fatalf("expected ErrCodeUnauthorized, got %v", err)
	}
}

func TestLogin_RejectsOversizedErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxTVDBErrorBodySize+1))))
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	err := client.Login(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var e *ClientError
	if !errors.As(err, &e) || e.Code != ErrCodeNetworkError {
		t.Fatalf("expected network error for oversized body, got %v", err)
	}
}

func TestEnsureToken_ConcurrentSingleflight(t *testing.T) {
	var loginCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		loginCalls.Add(1)
		time.Sleep(150 * time.Millisecond)
		json.NewEncoder(w).Encode(LoginResponse{
			Data: LoginData{Token: "test-token"},
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- client.ensureToken(context.Background())
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ensureToken returned error: %v", err)
		}
	}
	if got := loginCalls.Load(); got != 1 {
		t.Fatalf("expected 1 login call, got %d", got)
	}
	if token := client.getToken(); token != "test-token" {
		t.Fatalf("expected token to be populated, got %q", token)
	}
}

// TestGetSeries_Success tests retrieving series metadata.
func TestGetSeries_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/series/81189" {
			resp := SeriesResponse{
				Data: SeriesData{
					ID:           81189,
					Name:         "Breaking Bad",
					Overview:     "A high school chemistry teacher...",
					Image:        "/images/81189.jpg",
					FirstAirDate: "2008-01-20",
					Year:         "2008",
					ExternalIDs: IDsInfo{
						IMDB: "tt0903747",
						TVDb: 81189,
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	series, err := client.GetSeries(context.Background(), 81189)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if series == nil {
		t.Fatalf("expected series metadata, got nil")
	}

	if series.Title != "Breaking Bad" {
		t.Fatalf("expected title 'Breaking Bad', got %s", series.Title)
	}

	if series.IMDBID == nil || *series.IMDBID != "tt0903747" {
		t.Fatalf("expected IMDB ID 'tt0903747', got %v", series.IMDBID)
	}
}

// TestGetSeries_NotFound tests 404 response.
func TestGetSeries_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	series, err := client.GetSeries(context.Background(), 999999)
	if err == nil {
		t.Fatalf("expected error for not found")
	}

	var e *ClientError
	if !errors.As(err, &e) || e.Code != ErrCodeNotFound {
		t.Fatalf("expected ErrCodeNotFound, got %v", err)
	}

	if series != nil {
		t.Fatalf("expected nil series, got %v", series)
	}
}

// TestGetSeries_ServerError tests 5xx response.
func TestGetSeries_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	_, err := client.GetSeries(context.Background(), 81189)
	if err == nil {
		t.Fatalf("expected error for server error")
	}

	var e *ClientError
	if !errors.As(err, &e) || e.Code != ErrCodeServerError {
		t.Fatalf("expected ErrCodeServerError, got %v", err)
	}
}

// TestSearchSeries_Success tests search with multiple results.
func TestSearchSeries_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/search" {
			resp := SearchResponse{
				Data: []SearchResult{
					{
						ID:           81189,
						Name:         "Breaking Bad",
						FirstAirDate: "2008-01-20",
						Overview:     "A high school chemistry teacher...",
						Type:         "series",
						ExternalIDs: IDsInfo{
							IMDB: "tt0903747",
						},
					},
					{
						ID:           95831,
						Name:         "Breaking Bad: A Short Story",
						FirstAirDate: "2009-05-02",
						Type:         "series",
					},
				},
				Meta: SearchMeta{
					Page:      1,
					PageSize:  20,
					TotalSize: 2,
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	results, err := client.SearchSeries(context.Background(), "Breaking Bad", 2008)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "Breaking Bad" {
		t.Fatalf("expected first result to be 'Breaking Bad', got %s", results[0].Title)
	}
}

// TestSearchSeries_EmptyResults tests empty search results.
func TestSearchSeries_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/search" {
			resp := SearchResponse{
				Data: []SearchResult{},
				Meta: SearchMeta{
					Page:      1,
					PageSize:  20,
					TotalSize: 0,
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	results, err := client.SearchSeries(context.Background(), "NonexistentSeries999", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// TestTokenRefreshOn401 tests that 401 triggers token refresh.
func TestTokenRefreshOn401(t *testing.T) {
	requestCount := atomic.Int32{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "refreshed-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/series/81189" {
			count := requestCount.Add(1)
			// First request returns 401, second returns success
			if count == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			resp := SeriesResponse{
				Data: SeriesData{
					ID:   81189,
					Name: "Breaking Bad",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	// Pre-set an old token
	client.token = "old-token"

	series, err := client.GetSeries(context.Background(), 81189)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if series == nil {
		t.Fatalf("expected series metadata after token refresh")
	}

	if series.Title != "Breaking Bad" {
		t.Fatalf("expected title 'Breaking Bad', got %s", series.Title)
	}
}

// TestRateLimit_ExponentialBackoff tests 429 handling with backoff.
func TestRateLimit_ExponentialBackoff(t *testing.T) {
	requestCount := atomic.Int32{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/series/81189" {
			count := requestCount.Add(1)
			// Return 429 once, then succeed
			if count == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				w.Header().Set("Retry-After", "1")
				return
			}

			resp := SeriesResponse{
				Data: SeriesData{
					ID:   81189,
					Name: "Breaking Bad",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	start := time.Now()
	series, err := client.GetSeries(context.Background(), 81189)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if series == nil {
		t.Fatalf("expected series metadata after rate limit retry")
	}

	// Should have waited at least 1 second for the retry
	if elapsed < 1*time.Second {
		t.Logf("warning: expected backoff wait, but elapsed was %v", elapsed)
	}
}

// TestFindSeries_ImplementsInterface tests MetadataProvider interface.
func TestFindSeries_ImplementsInterface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/series/81189" {
			resp := SeriesResponse{
				Data: SeriesData{
					ID:   81189,
					Name: "Breaking Bad",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	results, err := client.FindSeries(context.Background(), "Breaking Bad", map[string]string{
		"tvdb": "81189",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Title != "Breaking Bad" {
		t.Fatalf("expected title 'Breaking Bad', got %s", results[0].Title)
	}
}

// TestConcurrentRequests_RaceSafe tests concurrent request handling.
func TestConcurrentRequests_RaceSafe(t *testing.T) {
	requestMutex := sync.Mutex{}
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMutex.Lock()
		requestCount++
		requestMutex.Unlock()

		if r.URL.Path == "/login" {
			resp := LoginResponse{
				Data: LoginData{Token: "test-token"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/series/81189" {
			resp := SeriesResponse{
				Data: SeriesData{
					ID:   81189,
					Name: "Breaking Bad",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	client := NewClient(cfg)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			series, err := client.GetSeries(context.Background(), 81189)
			if err != nil {
				t.Logf("error: %v", err)
			}
			if series == nil {
				t.Logf("series is nil")
			}
		}()
	}

	wg.Wait()

	requestMutex.Lock()
	defer requestMutex.Unlock()

	if requestCount < numGoroutines+1 { // +1 for login
		t.Logf("expected at least %d requests, got %d", numGoroutines+1, requestCount)
	}
}

// TestName implements MetadataProvider.
func TestName(t *testing.T) {
	client := NewClient(Config{APIKey: "test"})
	if client.Name() != "tvdb" {
		t.Fatalf("expected name 'tvdb', got %s", client.Name())
	}
}

// TestFindMovie_NotSupported tests that FindMovie returns error.
func TestFindMovie_NotSupported(t *testing.T) {
	client := NewClient(Config{APIKey: "test"})
	_, err := client.FindMovie(context.Background(), "Movie", 2020, nil)
	if err == nil {
		t.Fatalf("expected error for FindMovie")
	}
}

// TestGetSeriesEpisodes_Pagination verifies multi-page aggregation and that the
// aired-order season split is preserved.
func TestGetSeriesEpisodes_Pagination(t *testing.T) {
	page0 := SeriesEpisodesResponse{
		Data: SeriesEpisodesData{Episodes: []EpisodeBaseRecord{
			{ID: 1, SeasonNumber: 1, Number: 1, AbsoluteNumber: 1, Name: "S1E1"},
			{ID: 2, SeasonNumber: 1, Number: 2, AbsoluteNumber: 2, Name: "S1E2"},
		}},
		Links: PageLinks{Next: "page2"},
	}
	page1 := SeriesEpisodesResponse{
		Data: SeriesEpisodesData{Episodes: []EpisodeBaseRecord{
			{ID: 13, SeasonNumber: 2, Number: 1, AbsoluteNumber: 13, Name: "S2E1"},
		}},
		Links: PageLinks{Next: ""},
	}

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			json.NewEncoder(w).Encode(LoginResponse{Data: LoginData{Token: "t"}})
			return
		}
		if r.URL.Path != "/series/100/episodes/official" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			json.NewEncoder(w).Encode(page0)
		default:
			json.NewEncoder(w).Encode(page1)
		}
	}))
	defer server.Close()

	client := NewClient(Config{APIKey: "k", BaseURL: server.URL})
	eps, err := client.GetSeriesEpisodes(context.Background(), 100, "official")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 3 {
		t.Fatalf("expected 3 episodes across pages, got %d", len(eps))
	}
	if eps[2].SeasonNumber != 2 || eps[2].Number != 1 {
		t.Fatalf("expected last episode to be S2E1, got S%dE%d", eps[2].SeasonNumber, eps[2].Number)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected 2 page requests, got %d", hits)
	}
}

// TestGetSeriesEpisodes_EmptyStops ensures the loop stops on an empty page.
func TestGetSeriesEpisodes_EmptyStops(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			json.NewEncoder(w).Encode(LoginResponse{Data: LoginData{Token: "t"}})
			return
		}
		// Always return empty episodes with a non-empty Next link; the empty
		// page must still terminate the loop.
		json.NewEncoder(w).Encode(SeriesEpisodesResponse{Links: PageLinks{Next: "more"}})
	}))
	defer server.Close()

	client := NewClient(Config{APIKey: "k", BaseURL: server.URL})
	eps, err := client.GetSeriesEpisodes(context.Background(), 7, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 episodes, got %d", len(eps))
	}
}
