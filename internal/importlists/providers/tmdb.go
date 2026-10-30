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
	Popular  bool // if true, fetches popular instead of a specific list
	Version4 bool // explicit mixed movie/TV lists; requires a v4 read token
	client   *http.Client
}

// NewTMDbMixedList reuses the list adapter for paginated TMDB v4 mixed lists.
func NewTMDbMixedList() *TMDbProvider {
	return &TMDbProvider{Version4: true, client: &http.Client{Timeout: 30 * time.Second}}
}

// NewTMDbList returns a provider for a specific TMDb list.
func NewTMDbList() *TMDbProvider {
	return &TMDbProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

// NewTMDbPopular returns a provider for TMDb popular movies.
func NewTMDbPopular() *TMDbProvider {
	return &TMDbProvider{Popular: true, client: &http.Client{Timeout: 30 * time.Second}}
}

// Fetch reads the configured TMDB source without inferring identity from names.
func (p *TMDbProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	if p.Version4 {
		return p.fetchMixed(ctx, cfg)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("tmdb: API key required")
	}

	var url string
	if p.Popular {
		url = "https://api.themoviedb.org/3/movie/popular"
	} else {
		if cfg.URL == "" {
			return nil, fmt.Errorf("tmdb: list URL or ID required")
		}
		url = fmt.Sprintf("https://api.themoviedb.org/3/list/%s", cfg.URL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tmdb: build request: %w", err)
	}

	// Use Bearer auth for JWT tokens (len>100), query param for v3 keys.
	if len(cfg.APIKey) > 100 {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	} else {
		q := req.URL.Query()
		q.Add("api_key", cfg.APIKey)
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
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
			_, _ = fmt.Sscanf(e.ReleaseDate[:4], "%d", &year)
		}
		if year == 0 && len(e.FirstAirDate) >= 4 {
			_, _ = fmt.Sscanf(e.FirstAirDate[:4], "%d", &year)
		}

		items = append(items, Item{
			ExternalID: fmt.Sprintf("tmdb-%d", e.ID),
			Title:      title,
			Year:       year,
			TMDbID:     fmt.Sprintf("%d", e.ID),
			MediaType:  tmdbMedia(e),
			PosterPath: e.PosterPath,
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
	MediaType    string `json:"media_type"`
	PosterPath   string `json:"poster_path"`
}

func tmdbMedia(e tmdbEntry) string {
	if e.MediaType == "tv" || e.MediaType == "series" || (e.MediaType == "" && e.Name != "" && e.Title == "") {
		return "series"
	}
	if e.MediaType == "" || e.MediaType == "movie" {
		return "movie"
	}
	return ""
}

func (p *TMDbProvider) fetchMixed(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	// Only numeric IDs can reach the fixed provider host.
	for _, ch := range cfg.URL {
		if ch < '0' || ch > '9' {
			return nil, fmt.Errorf("tmdb: numeric list ID required")
		}
	}
	if cfg.URL == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("tmdb: list ID and v4 read access token required")
	}
	items := []Item{}
	for page := 1; page <= 500; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.themoviedb.org/4/list/%s?page=%d", cfg.URL, page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("tmdb: list request failed")
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, (10<<20)+1))
		_ = resp.Body.Close()
		if err != nil || len(body) > 10<<20 {
			return nil, fmt.Errorf("tmdb: list response exceeds limit or cannot be read")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("tmdb: list access failed (%d); v4 read token required", resp.StatusCode)
		}
		var result struct {
			Results    []tmdbEntry `json:"results"`
			TotalPages int         `json:"total_pages"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("tmdb: invalid list response")
		}
		if result.Results == nil || result.TotalPages < 0 {
			return nil, fmt.Errorf("tmdb: incomplete list response")
		}
		for _, e := range result.Results {
			media := tmdbMedia(e)
			if media == "" {
				continue
			}
			if e.ID <= 0 {
				return nil, fmt.Errorf("tmdb: invalid title ID")
			}
			title, date := e.Title, e.ReleaseDate
			if media == "series" {
				title, date = e.Name, e.FirstAirDate
			}
			var year int
			if len(date) >= 4 {
				_, _ = fmt.Sscanf(date[:4], "%d", &year)
			}
			items = append(items, Item{TMDbID: fmt.Sprint(e.ID), Title: title, Year: year, MediaType: media, PosterPath: e.PosterPath})
		}
		if page >= result.TotalPages {
			return items, nil
		}
	}
	return nil, fmt.Errorf("tmdb: list exceeds 500 pages")
}
