package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TMDbProvider fetches items from TMDb lists or popular/trending endpoints.
type TMDbProvider struct {
	Popular bool // if true, fetches popular instead of a specific list
	client  *http.Client
}

// NewTMDbList returns a provider for a specific TMDb list.
func NewTMDbList() *TMDbProvider {
	return &TMDbProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

// NewTMDbPopular returns a provider for TMDb popular movies.
func NewTMDbPopular() *TMDbProvider {
	return &TMDbProvider{Popular: true, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *TMDbProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("tmdb: API key required")
	}

	var url string
	if p.Popular {
		url = fmt.Sprintf("https://api.themoviedb.org/3/movie/popular?api_key=%s", cfg.APIKey)
	} else {
		if cfg.URL == "" {
			return nil, fmt.Errorf("tmdb: list URL or ID required")
		}
		url = fmt.Sprintf("https://api.themoviedb.org/3/list/%s?api_key=%s", cfg.URL, cfg.APIKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tmdb: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("tmdb: read body: %w", err)
	}

	var result tmdbResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("tmdb: parse: %w", err)
	}

	entries := result.Results
	if entries == nil {
		entries = result.Items
	}

	var items []Item
	for _, e := range entries {
		title := e.Title
		if title == "" {
			title = e.Name
		}
		if title == "" {
			continue
		}
		year := 0
		if len(e.ReleaseDate) >= 4 {
			fmt.Sscanf(e.ReleaseDate[:4], "%d", &year)
		}
		if year == 0 && len(e.FirstAirDate) >= 4 {
			fmt.Sscanf(e.FirstAirDate[:4], "%d", &year)
		}

		items = append(items, Item{
			ExternalID: fmt.Sprintf("tmdb-%d", e.ID),
			Title:      title,
			Year:       year,
			TMDbID:     fmt.Sprintf("%d", e.ID),
		})
	}
	return items, nil
}

type tmdbResponse struct {
	Results []tmdbEntry `json:"results"`
	Items   []tmdbEntry `json:"items"`
}

type tmdbEntry struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
}
