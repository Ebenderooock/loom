package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/metadata"
)

func TestGetMovieSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/movie/550") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(MovieResponse{
				ID:          550,
				IMDBID:      "tt0137523",
				Title:       "Fight Club",
				ReleaseDate: "1999-10-15",
				Overview:    "An insomniac office worker and a devil-may-care soap maker form an underground fight club.",
				PosterPath:  "/a28my1q3o1MjID6a8ynT2Yemzj.jpg",
				Runtime:     139,
				VoteAverage: 8.8,
				Genres: []GenreResponse{
					{ID: 18, Name: "Drama"},
					{ID: 28, Name: "Action"},
				},
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	movie, err := client.GetMovie(ctx, 550)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if movie == nil {
		t.Fatal("expected movie, got nil")
	}
	if movie.Title != "Fight Club" {
		t.Errorf("expected title 'Fight Club', got %q", movie.Title)
	}
	if movie.Year != 1999 {
		t.Errorf("expected year 1999, got %d", movie.Year)
	}
	if movie.Runtime != 139 {
		t.Errorf("expected runtime 139, got %d", movie.Runtime)
	}
	if movie.IMDBID == nil || *movie.IMDBID != "tt0137523" {
		t.Errorf("expected IMDB ID 'tt0137523', got %v", movie.IMDBID)
	}
}

func TestGetMovieNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			StatusCode:    404,
			StatusMessage: "The resource you requested could not be found.",
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	_, err := client.GetMovie(ctx, 999999999)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *ClientError
	if !errors.As(err, &ce) || ce.Code != ErrCodeNotFound {
		t.Errorf("expected ErrCodeNotFound, got %T: %v", err, err)
	}
}

func TestGetMovieUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			StatusCode:    401,
			StatusMessage: "Invalid API key: You must be granted a valid key.",
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "invalid-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	_, err := client.GetMovie(ctx, 550)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *ClientError
	if !errors.As(err, &ce) || ce.Code != ErrCodeUnauthorized {
		t.Errorf("expected ErrCodeUnauthorized, got %T: %v", err, err)
	}
}

func TestGetMovieRateLimit(t *testing.T) {
	callCount := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if callCount.Load() == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(ErrorResponse{
				StatusCode:    429,
				StatusMessage: "Your request count (1) is over the allowed limit of 0.",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(MovieResponse{
				ID:          550,
				Title:       "Fight Club",
				ReleaseDate: "1999-10-15",
				Overview:    "An insomniac office worker and a devil-may-care soap maker form an underground fight club.",
				PosterPath:  "/a28my1q3o1MjID6a8ynT2Yemzj.jpg",
				Runtime:     139,
				VoteAverage: 8.8,
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	movie, err := client.GetMovie(ctx, 550)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if movie == nil {
		t.Fatal("expected movie after retry, got nil")
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (1 429 + 1 retry), got %d", callCount.Load())
	}
}

func TestGetMovieServerError(t *testing.T) {
	callCount := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if callCount.Load() < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{
				StatusCode:    500,
				StatusMessage: "Internal Server Error",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(MovieResponse{
				ID:    550,
				Title: "Fight Club",
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	movie, err := client.GetMovie(ctx, 550)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if movie == nil {
		t.Fatal("expected movie after retries, got nil")
	}
	if callCount.Load() < 3 {
		t.Errorf("expected at least 3 calls with backoff, got %d", callCount.Load())
	}
}

func TestSearchMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{
			Results: []SearchResult{
				{
					ID:          550,
					Title:       "Fight Club",
					ReleaseDate: "1999-10-15",
					Overview:    "An insomniac office worker and a devil-may-care soap maker form an underground fight club.",
					PosterPath:  "/a28my1q3o1MjID6a8ynT2Yemzj.jpg",
					VoteAverage: 8.8,
					MediaType:   "movie",
				},
			},
			TotalResults: 1,
			TotalPages:   1,
			Page:         1,
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	results, err := client.SearchMovie(ctx, "Fight Club", 1999)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Fight Club" {
		t.Errorf("expected title 'Fight Club', got %q", results[0].Title)
	}
}

func TestSearchMovieNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{
			Results:      []SearchResult{},
			TotalResults: 0,
			TotalPages:   0,
			Page:         1,
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	results, err := client.SearchMovie(ctx, "NonexistentMovie123", 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestGetTVSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/1399") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TVResponse{
				ID:              1399,
				IMDBID:          "tt0944947",
				Name:            "Game of Thrones",
				FirstAirDate:    "2011-04-18",
				Overview:        "Seven noble families fight for control of the mythical land of Westeros.",
				PosterPath:      "/u3bZgnrm11QwQ5kCD7nau7Sw1qb.jpg",
				VoteAverage:     9.2,
				NumberOfSeasons: 8,
				Genres: []GenreResponse{
					{ID: 18, Name: "Drama"},
					{ID: 10759, Name: "Action & Adventure"},
				},
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	series, err := client.GetTV(ctx, 1399)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if series == nil {
		t.Fatal("expected series, got nil")
	}
	if series.Title != "Game of Thrones" {
		t.Errorf("expected title 'Game of Thrones', got %q", series.Title)
	}
	if series.Seasons != 8 {
		t.Errorf("expected 8 seasons, got %d", series.Seasons)
	}
}

func TestSearchTV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{
			Results: []SearchResult{
				{
					ID:           1399,
					Name:         "Game of Thrones",
					FirstAirDate: "2011-04-18",
					Overview:     "Seven noble families fight for control of the mythical land of Westeros.",
					PosterPath:   "/u3bZgnrm11QwQ5kCD7nau7Sw1qb.jpg",
					VoteAverage:  9.2,
					MediaType:    "tv",
				},
			},
			TotalResults: 1,
			TotalPages:   1,
			Page:         1,
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	results, err := client.SearchTV(ctx, "Game of Thrones", 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Game of Thrones" {
		t.Errorf("expected title 'Game of Thrones', got %q", results[0].Title)
	}
}

func TestGetEpisodeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/1399/season/1/episode/1") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(EpisodeResponse{
				ID:            349232,
				Name:          "Winter is Coming",
				Overview:      "Bran Stark and his father escort a deserter from the Wall.",
				AirDate:       "2011-04-18",
				EpisodeNumber: 1,
				SeasonNumber:  1,
				Runtime:       56,
				VoteAverage:   7.8,
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	episode, err := client.GetEpisode(ctx, 1399, 1, 1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if episode == nil {
		t.Fatal("expected episode, got nil")
	}
	if episode.Title != "Winter is Coming" {
		t.Errorf("expected title 'Winter is Coming', got %q", episode.Title)
	}
	if episode.Season != 1 || episode.Episode != 1 {
		t.Errorf("expected S01E01, got S%02dE%02d", episode.Season, episode.Episode)
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MovieResponse{ID: 550, Title: "Fight Club"})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.GetMovie(ctx, 550)

	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	var ce *ClientError
	if !errors.As(err, &ce) || ce.Code != ErrCodeContextError {
		t.Errorf("expected ErrCodeContextError, got %T: %v", err, err)
	}
}

func TestConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Extract ID from path
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-1]
		movieID, _ := strconv.Atoi(id)
		json.NewEncoder(w).Encode(MovieResponse{
			ID:    movieID,
			Title: fmt.Sprintf("Movie %d", movieID),
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan *metadata.MovieMetadata, 10)
	errors := make(chan error, 10)

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			movie, err := client.GetMovie(ctx, id)
			if err != nil {
				errors <- err
			} else {
				results <- movie
			}
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	if len(errors) > 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}
}

func TestMapperOverviewCropping(t *testing.T) {
	longOverview := strings.Repeat("x", 2000)
	resp := &MovieResponse{
		ID:       1,
		Title:    "Test",
		Overview: longOverview,
	}

	m := mapMovieResponse(resp)
	if len(m.Overview) != 1000 {
		t.Errorf("expected overview length 1000, got %d", len(m.Overview))
	}
}

func TestMapperPosterURL(t *testing.T) {
	tests := []struct {
		name        string
		posterPath  string
		expectedURL string
	}{
		{
			name:        "relative path",
			posterPath:  "/a28my1q3o1MjID6a8ynT2Yemzj.jpg",
			expectedURL: "https://image.tmdb.org/t/p/w342/a28my1q3o1MjID6a8ynT2Yemzj.jpg",
		},
		{
			name:        "empty path",
			posterPath:  "",
			expectedURL: "",
		},
		{
			name:        "full URL",
			posterPath:  "https://example.com/poster.jpg",
			expectedURL: "https://example.com/poster.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := buildPosterURL(tt.posterPath)
			if url != tt.expectedURL {
				t.Errorf("expected %q, got %q", tt.expectedURL, url)
			}
		})
	}
}

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      *ClientError
		expected ErrorCode
	}{
		{
			name:     "not found",
			err:      NewNotFoundError("not found"),
			expected: ErrCodeNotFound,
		},
		{
			name:     "unauthorized",
			err:      NewUnauthorizedError("invalid key"),
			expected: ErrCodeUnauthorized,
		},
		{
			name:     "rate limit",
			err:      NewRateLimitError("too many requests", 60),
			expected: ErrCodeRateLimit,
		},
		{
			name:     "server error",
			err:      NewServerError(500, "internal error"),
			expected: ErrCodeServerError,
		},
		{
			name:     "client error",
			err:      NewClientError(400, "bad request"),
			expected: ErrCodeClientError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.expected {
				t.Errorf("expected code %v, got %v", tt.expected, tt.err.Code)
			}
		})
	}
}

func TestJSONUnmarshalErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{invalid json}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx := context.Background()
	_, err := client.GetMovie(ctx, 550)

	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRetryAfterHeader(t *testing.T) {
	callCount := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if callCount.Load() == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(ErrorResponse{
				StatusCode:    429,
				StatusMessage: "Rate limit exceeded",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(MovieResponse{
				ID:    550,
				Title: "Fight Club",
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	start := time.Now()
	movie, err := client.GetMovie(ctx, 550)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if movie == nil {
		t.Fatal("expected movie, got nil")
	}

	elapsed := time.Since(start)
	if elapsed < 2*time.Second {
		t.Errorf("expected at least 2 second delay (Retry-After), but elapsed %.2f seconds", elapsed.Seconds())
	}
}
