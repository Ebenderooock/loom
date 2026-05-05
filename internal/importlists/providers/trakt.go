package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TraktProvider fetches items from Trakt lists or watchlists.
type TraktProvider struct {
	// Watchlist mode fetches the user's watchlist; otherwise a list URL.
	Watchlist bool
	client    *http.Client
}

// NewTraktList returns a provider for a Trakt public list.
func NewTraktList() *TraktProvider {
	return &TraktProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

// NewTraktWatchlist returns a provider for a Trakt user watchlist.
func NewTraktWatchlist() *TraktProvider {
	return &TraktProvider{Watchlist: true, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *TraktProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	if cfg.URL == "" && cfg.AccessToken == "" {
		return nil, fmt.Errorf("trakt: URL or access token required")
	}

	url := cfg.URL
	if url == "" && p.Watchlist {
		url = "https://api.trakt.tv/users/me/watchlist/movies"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("trakt: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("trakt-api-key", cfg.APIKey)
		req.Header.Set("trakt-api-version", "2")
	}
	if cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trakt: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trakt: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("trakt: read body: %w", err)
	}

	var entries []traktEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("trakt: parse: %w", err)
	}

	var items []Item
	for _, e := range entries {
		m := e.Movie
		if m.Title == "" {
			m = e.Show
		}
		if m.Title == "" {
			continue
		}
		items = append(items, Item{
			ExternalID: fmt.Sprintf("trakt-%d", m.IDs.Trakt),
			Title:      m.Title,
			Year:       m.Year,
			IMDbID:     m.IDs.IMDB,
			TMDbID:     fmt.Sprintf("%d", m.IDs.TMDB),
			TVDbID:     fmt.Sprintf("%d", m.IDs.TVDB),
		})
	}
	return items, nil
}

type traktEntry struct {
	Movie traktMedia `json:"movie"`
	Show  traktMedia `json:"show"`
}

type traktMedia struct {
	Title string   `json:"title"`
	Year  int      `json:"year"`
	IDs   traktIDs `json:"ids"`
}

type traktIDs struct {
	Trakt int    `json:"trakt"`
	Slug  string `json:"slug"`
	IMDB  string `json:"imdb"`
	TMDB  int    `json:"tmdb"`
	TVDB  int    `json:"tvdb"`
}
