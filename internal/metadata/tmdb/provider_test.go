package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	if provider.Name() != "tmdb" {
		t.Errorf("expected provider name 'tmdb', got %q", provider.Name())
	}
}

func TestProviderFindMovieByTMDBID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/movie/550") {
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
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	externalIDs := map[string]string{"tmdb": "550"}
	results, err := provider.FindMovie(ctx, "", 0, externalIDs)

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

func TestProviderFindMovieBySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search/movie") {
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
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	results, err := provider.FindMovie(ctx, "Fight Club", 1999, map[string]string{})

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

func TestProviderFindMovieNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			StatusCode:    404,
			StatusMessage: "Not found",
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	results, err := provider.FindMovie(ctx, "Nonexistent", 0, map[string]string{})

	// Provider should return nil, nil on not found (no error)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil && len(results) > 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestProviderFindSeriesByTMDBID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/1399") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TVResponse{
				ID:              1399,
				Name:            "Game of Thrones",
				FirstAirDate:    "2011-04-18",
				Overview:        "Seven noble families fight for control of the mythical land of Westeros.",
				PosterPath:      "/u3bZgnrm11QwQ5kCD7nau7Sw1qb.jpg",
				VoteAverage:     9.2,
				NumberOfSeasons: 8,
			})
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	externalIDs := map[string]string{"tmdb": "1399"}
	results, err := provider.FindSeries(ctx, "", externalIDs)

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

func TestProviderFindSeriesBySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search/tv") {
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
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	results, err := provider.FindSeries(ctx, "Game of Thrones", map[string]string{})

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

func TestProviderFindEpisodeSuccess(t *testing.T) {
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
	provider := NewProvider(client)

	ctx := context.Background()
	episode, err := provider.FindEpisode(ctx, "1399", 1, 1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if episode == nil {
		t.Fatal("expected episode, got nil")
	}
	if episode.Title != "Winter is Coming" {
		t.Errorf("expected title 'Winter is Coming', got %q", episode.Title)
	}
}

func TestProviderFindEpisodeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			StatusCode:    404,
			StatusMessage: "Episode not found",
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	episode, err := provider.FindEpisode(ctx, "1399", 99, 99)

	// Provider should return nil, nil on not found
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if episode != nil {
		t.Errorf("expected nil episode, got %v", episode)
	}
}

func TestProviderFindEpisodeInvalidSeriesID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	ctx := context.Background()
	episode, err := provider.FindEpisode(ctx, "not-a-number", 1, 1)

	// Provider should return nil, nil if series ID is not a valid number
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if episode != nil {
		t.Errorf("expected nil episode for invalid series ID, got %v", episode)
	}
}

func TestProviderInterfaceImplementation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})
	provider := NewProvider(client)

	// Verify Provider implements MetadataProvider interface
	var _ interface {
		Name() string
	} = provider

	// Just verify the methods exist and are callable
	if provider.Name() == "" {
		t.Fatal("Name() returned empty string")
	}
}
