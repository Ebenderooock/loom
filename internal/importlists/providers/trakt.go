package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TraktProvider fetches items from Trakt lists, watchlists, or built-in
// discovery endpoints (popular, trending, anticipated).
type TraktProvider struct {
	// endpoint is the fixed API URL for built-in types; empty for custom lists.
	endpoint string
	// needsAuth indicates the endpoint requires an access_token (e.g. watchlist, user lists).
	needsAuth bool
	client    *http.Client
}

// NewTraktList returns a provider for a user's custom Trakt list.
// The list slug is stored in ProviderConfig.URL and the provider
// constructs the full API URL internally.
func NewTraktList() *TraktProvider {
	return &TraktProvider{needsAuth: true, client: &http.Client{Timeout: 30 * time.Second}}
}

// NewTraktWatchlist returns a provider for a Trakt user watchlist.
func NewTraktWatchlist() *TraktProvider {
	return &TraktProvider{
		endpoint:  "https://api.trakt.tv/users/me/watchlist",
		needsAuth: true,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// NewTraktPopular returns a provider for Trakt popular movies/shows.
func NewTraktPopular() *TraktProvider {
	return &TraktProvider{
		endpoint: "https://api.trakt.tv/movies/popular",
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// NewTraktTrending returns a provider for Trakt trending movies/shows.
func NewTraktTrending() *TraktProvider {
	return &TraktProvider{
		endpoint: "https://api.trakt.tv/movies/trending",
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// NewTraktAnticipated returns a provider for Trakt anticipated movies/shows.
func NewTraktAnticipated() *TraktProvider {
	return &TraktProvider{
		endpoint: "https://api.trakt.tv/movies/anticipated",
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *TraktProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	apiURL := p.endpoint

	// For custom user lists, cfg.URL holds the list slug.
	// Construct: /users/me/lists/{slug}/items
	if apiURL == "" && cfg.URL != "" {
		apiURL = "https://api.trakt.tv/users/me/lists/" + cfg.URL + "/items"
	}

	if apiURL == "" {
		return nil, fmt.Errorf("trakt: no endpoint or list slug configured")
	}

	if p.needsAuth && cfg.AccessToken == "" {
		return nil, fmt.Errorf("trakt: access token required for this list type")
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

// FetchUserLists retrieves the authenticated user's custom Trakt lists.
func FetchTraktUserLists(ctx context.Context, clientID, accessToken string) ([]TraktUserList, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.trakt.tv/users/me/lists", nil)
	if err != nil {
		return nil, fmt.Errorf("trakt: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-key", clientID)
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trakt: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trakt: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("trakt: read body: %w", err)
	}

	var raw []struct {
		Name    string `json:"name"`
		IDs     struct{ Slug string `json:"slug"` } `json:"ids"`
		Privacy string `json:"privacy"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("trakt: parse lists: %w", err)
	}

	lists := make([]TraktUserList, len(raw))
	for i, r := range raw {
		lists[i] = TraktUserList{Name: r.Name, Slug: r.IDs.Slug}
	}
	return lists, nil
}

// TraktUserList is a Trakt custom list name+slug pair.
type TraktUserList struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
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
