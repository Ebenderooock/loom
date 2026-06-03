package tvdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/metadata"
)

// Config holds TVDB API configuration.
type Config struct {
	APIKey  string
	UserKey string
	PIN     string
	BaseURL string
}

// Client is the TVDB HTTP client.
type Client struct {
	config     Config
	httpClient *http.Client
	token      string
	tokenMutex sync.Mutex
	logger     interface{} // placeholder for logger
}

// NewClient creates a new TVDB client with the given config.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api4.thetvdb.com/v4"
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Name implements MetadataProvider interface.
func (c *Client) Name() string {
	return "tvdb"
}

// Login obtains a JWT token from TVDB API.
func (c *Client) Login(ctx context.Context) error {
	c.tokenMutex.Lock()
	defer c.tokenMutex.Unlock()

	loginReq := LoginRequest{
		APIKey: c.config.APIKey,
		PIN:    c.config.PIN,
	}

	body, err := json.Marshal(loginReq)
	if err != nil {
		return NewNetworkError(err)
	}

	url := fmt.Sprintf("%s/login", c.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return NewNetworkError(err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NewNetworkError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return NewUnauthorizedError("invalid TVDB credentials (invalid API key or PIN)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return NewClientError(resp.StatusCode, string(body))
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return NewNetworkError(err)
	}

	c.token = loginResp.Data.Token
	return nil
}

// GetSeries retrieves series metadata by TVDB ID.
func (c *Client) GetSeries(ctx context.Context, tvdbID int) (*metadata.SeriesMetadata, error) {
	data, err := c.getSeriesData(ctx, tvdbID)
	if err != nil {
		return nil, err
	}

	return MapSeriesToMetadata(data), nil
}

// GetSeriesEpisodes retrieves all episodes for a series in the given season type
// (e.g. "official" for aired order, "absolute", "dvd"). It paginates the TVDB
// endpoint until no further episodes are returned.
func (c *Client) GetSeriesEpisodes(ctx context.Context, tvdbID int, seasonType string) ([]EpisodeBaseRecord, error) {
	if seasonType == "" {
		seasonType = "official"
	}
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	const maxPages = 200
	var episodes []EpisodeBaseRecord

	for page := 0; page < maxPages; page++ {
		u, _ := url.Parse(fmt.Sprintf("%s/series/%d/episodes/%s", c.config.BaseURL, tvdbID, url.PathEscape(seasonType)))
		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()
		reqURL := u.String()

		resp, err := c.doRequest(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			c.tokenMutex.Lock()
			c.token = ""
			c.tokenMutex.Unlock()
			if err := c.ensureToken(ctx); err != nil {
				return nil, err
			}
			resp, err = c.doRequest(ctx, "GET", reqURL, nil)
			if err != nil {
				return nil, err
			}
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil, NewNotFoundError(fmt.Sprintf("series %d not found", tvdbID))
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				return nil, NewServerError(resp.StatusCode, string(body))
			}
			return nil, NewClientError(resp.StatusCode, string(body))
		}

		var er SeriesEpisodesResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			resp.Body.Close()
			return nil, NewNetworkError(err)
		}
		resp.Body.Close()

		if len(er.Data.Episodes) == 0 {
			break
		}
		episodes = append(episodes, er.Data.Episodes...)

		// Stop when the API reports no further page.
		if er.Links.Next == "" {
			break
		}
	}

	return episodes, nil
}

// GetEpisode retrieves episode metadata by series TVDB ID and season/episode numbers.
func (c *Client) GetEpisode(ctx context.Context, seriesTVDBID, season, episode int) (*metadata.EpisodeMetadata, error) {
	// TVDB v4 requires episode ID, not season/episode numbers directly.
	// This is a limitation; we'd need a separate call to get episode list first.
	// For now, return a placeholder implementation that would need the episode TVDB ID.
	return nil, NewClientError(501, "GetEpisode requires episode TVDB ID; use SearchEpisodes first")
}

// SearchSeries searches for series by title and optional year.
func (c *Client) SearchSeries(ctx context.Context, query string, year int) ([]*metadata.SeriesMetadata, error) {
	results, err := c.searchSeries(ctx, query, year)
	if err != nil {
		return nil, err
	}

	var metadataResults []*metadata.SeriesMetadata
	for _, result := range results {
		if result.Type == "series" {
			m := MapSearchResultToMetadata(&result)
			if m != nil {
				metadataResults = append(metadataResults, m)
			}
		}
	}

	return metadataResults, nil
}

// FindMovie implements MetadataProvider interface (not implemented for TVDB).
func (c *Client) FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*metadata.MovieMetadata, error) {
	return nil, NewClientError(400, "TVDB is TV-only; use FindSeries instead")
}

// FindSeries implements MetadataProvider interface.
func (c *Client) FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*metadata.SeriesMetadata, error) {
	// Try TVDB ID first if available
	if tvdbIDStr, ok := externalIDs["tvdb"]; ok {
		tvdbID, err := strconv.Atoi(tvdbIDStr)
		if err == nil {
			series, err := c.GetSeries(ctx, tvdbID)
			if err == nil && series != nil {
				return []*metadata.SeriesMetadata{series}, nil
			}
		}
	}

	// Fall back to search by title
	results, err := c.SearchSeries(ctx, title, 0)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// FindEpisode implements MetadataProvider interface.
func (c *Client) FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*metadata.EpisodeMetadata, error) {
	// TVDB requires episode TVDB ID, not just season/episode numbers
	// This would require a two-step lookup: get series episodes, then get specific episode
	// For now, return placeholder
	return nil, NewClientError(501, "TVDB episode lookup requires episode TVDB ID")
}

// --- Private methods ---

// getSeriesData retrieves raw series data from TVDB.
func (c *Client) getSeriesData(ctx context.Context, tvdbID int) (*SeriesData, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/series/%d", c.config.BaseURL, tvdbID)
	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, NewNotFoundError(fmt.Sprintf("series %d not found", tvdbID))
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token and retry
		c.tokenMutex.Lock()
		c.token = ""
		c.tokenMutex.Unlock()

		if err := c.ensureToken(ctx); err != nil {
			return nil, err
		}

		resp, err = c.doRequest(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, NewUnauthorizedError("token refresh failed")
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 500 {
			return nil, NewServerError(resp.StatusCode, string(body))
		}
		return nil, NewClientError(resp.StatusCode, string(body))
	}

	var seriesResp SeriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&seriesResp); err != nil {
		return nil, NewNetworkError(err)
	}

	return &seriesResp.Data, nil
}

// searchSeries performs a series search on TVDB.
func (c *Client) searchSeries(ctx context.Context, query string, year int) ([]SearchResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	// Use net/url for proper query encoding
	u, _ := url.Parse(fmt.Sprintf("%s/search", c.config.BaseURL))
	q := u.Query()
	q.Set("query", query)
	q.Set("type", "series")
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	u.RawQuery = q.Encode()
	searchURL := u.String()

	resp, err := c.doRequest(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.tokenMutex.Lock()
		c.token = ""
		c.tokenMutex.Unlock()

		if err := c.ensureToken(ctx); err != nil {
			return nil, err
		}

		resp, err = c.doRequest(ctx, "GET", searchURL, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 500 {
			return nil, NewServerError(resp.StatusCode, string(body))
		}
		return nil, NewClientError(resp.StatusCode, string(body))
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, NewNetworkError(err)
	}

	return searchResp.Data, nil
}

// ensureToken ensures a valid JWT token is available.
func (c *Client) ensureToken(ctx context.Context) error {
	c.tokenMutex.Lock()
	if c.token != "" {
		c.tokenMutex.Unlock()
		return nil
	}
	c.tokenMutex.Unlock()

	return c.Login(ctx)
}

// doRequest performs an HTTP request with exponential backoff for rate limits.
func (c *Client) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	maxRetries := 5
	backoff := 1.0
	maxBackoff := 60.0

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, NewNetworkError(err)
		}

		req.Header.Set("Content-Type", "application/json")
		if c.token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, NewContextError(ctx.Err())
			}
			return nil, NewNetworkError(err)
		}

		// Handle rate limiting (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			if attempt < maxRetries {
				// Extract Retry-After header if present
				retryAfter := int(backoff)
				if header := resp.Header.Get("Retry-After"); header != "" {
					if seconds, err := strconv.Atoi(header); err == nil {
						retryAfter = seconds
					}
				}

				// Add jitter to prevent thundering herd
				jitter := time.Duration((rand.Float64() * 0.2 * float64(retryAfter)) * float64(time.Second))
				waitTime := time.Duration(retryAfter)*time.Second + jitter

				select {
				case <-time.After(waitTime):
					backoff = math.Min(backoff*2, maxBackoff)
					continue
				case <-ctx.Done():
					return nil, NewContextError(ctx.Err())
				}
			}

			return nil, NewRateLimitError("rate limited by TVDB", int(backoff))
		}

		// Success or non-retryable error
		return resp, nil
	}

	return nil, NewRateLimitError("max retries exceeded for rate limit", int(maxBackoff))
}
