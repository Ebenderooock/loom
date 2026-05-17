package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// transformTraktURL converts web-facing Trakt URLs into API URLs.
func transformTraktURL(raw string) string {
	// Already an API URL — leave as-is.
	if strings.HasPrefix(raw, "https://api.trakt.tv/") {
		// For user lists, ensure /items suffix is present.
		if strings.Contains(raw, "/users/") && strings.Contains(raw, "/lists/") && !strings.HasSuffix(raw, "/items") {
			raw = strings.TrimRight(raw, "/") + "/items"
		}
		return raw
	}

	// Parse the URL to extract path and query params.
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	host := strings.ToLower(u.Hostname())
	if host != "trakt.tv" && host != "app.trakt.tv" && host != "www.trakt.tv" {
		return raw
	}

	path := strings.TrimPrefix(u.Path, "/")
	mode := u.Query().Get("mode")

	// Handle /discover/{type}?mode={movie|show} pages.
	if strings.HasPrefix(path, "discover/") {
		category := strings.TrimPrefix(path, "discover/")
		category = strings.Split(category, "/")[0] // e.g. "anticipated", "popular", "trending"

		mediaPath := "movies"
		if mode == "show" || mode == "shows" {
			mediaPath = "shows"
		}
		return fmt.Sprintf("https://api.trakt.tv/%s/%s", mediaPath, category)
	}

	// Handle /movies/{category} and /shows/{category} pages.
	if strings.HasPrefix(path, "movies/") || strings.HasPrefix(path, "shows/") {
		return "https://api.trakt.tv/" + path
	}

	// Handle /users/{user}/lists/{list} pages.
	if strings.HasPrefix(path, "users/") && strings.Contains(path, "/lists/") {
		apiURL := "https://api.trakt.tv/" + strings.TrimRight(path, "/")
		if !strings.HasSuffix(apiURL, "/items") {
			apiURL += "/items"
		}
		return apiURL
	}

	// Fallback: reconstruct with API host.
	return "https://api.trakt.tv/" + path
}

func (p *TraktProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	if cfg.URL == "" && cfg.AccessToken == "" {
		return nil, fmt.Errorf("trakt: URL or access token required")
	}

	apiURL := cfg.URL
	if apiURL == "" && p.Watchlist {
		apiURL = "https://api.trakt.tv/users/me/watchlist/movies"
	}

	// Transform web URLs to API URLs.
	if apiURL != "" {
		apiURL = transformTraktURL(apiURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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
		if e.Movie.Title != "" {
			items = append(items, Item{
				ExternalID: fmt.Sprintf("trakt-%d", e.Movie.IDs.Trakt),
				Title:      e.Movie.Title,
				Year:       e.Movie.Year,
				IMDbID:     e.Movie.IDs.IMDB,
				TMDbID:     fmt.Sprintf("%d", e.Movie.IDs.TMDB),
				TVDbID:     fmt.Sprintf("%d", e.Movie.IDs.TVDB),
				MediaType:  "movie",
			})
		} else if e.Show.Title != "" {
			items = append(items, Item{
				ExternalID: fmt.Sprintf("trakt-%d", e.Show.IDs.Trakt),
				Title:      e.Show.Title,
				Year:       e.Show.Year,
				IMDbID:     e.Show.IDs.IMDB,
				TMDbID:     fmt.Sprintf("%d", e.Show.IDs.TMDB),
				TVDbID:     fmt.Sprintf("%d", e.Show.IDs.TVDB),
				MediaType:  "series",
			})
		}
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
