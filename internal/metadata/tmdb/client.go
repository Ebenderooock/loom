package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/metadata"
)

// Config holds TMDb client configuration.
type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

// Client implements a TMDb HTTP client with exponential backoff for rate limiting.
type Client struct {
	config        Config
	httpClient    *http.Client
	requestCache  sync.Map // simple per-request deduplication
	backoffConfig BackoffConfig
}

// BackoffConfig controls exponential backoff behavior.
type BackoffConfig struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DefaultBackoffConfig returns sensible defaults for exponential backoff.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
	}
}

// NewClient builds a new TMDb HTTP client.
func NewClient(config Config) *Client {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.themoviedb.org/3"
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		backoffConfig: DefaultBackoffConfig(),
	}
}

// GetMovie fetches movie metadata by TMDb ID.
func (c *Client) GetMovie(ctx context.Context, tmdbID int) (*metadata.MovieMetadata, error) {
	url := fmt.Sprintf("%s/movie/%d?append_to_response=external_ids", c.config.BaseURL, tmdbID)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp MovieResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal movie response: %w", err)
	}

	return mapMovieResponse(&resp), nil
}

// GetTV fetches TV series metadata by TMDb ID.
func (c *Client) GetTV(ctx context.Context, tvID int) (*metadata.SeriesMetadata, error) {
	url := fmt.Sprintf("%s/tv/%d?append_to_response=external_ids", c.config.BaseURL, tvID)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp TVResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal tv response: %w", err)
	}

	return mapTVResponse(&resp), nil
}

// SearchMovie searches for movies by title and optional year.
func (c *Client) SearchMovie(ctx context.Context, query string, year int) ([]*metadata.MovieMetadata, error) {
	url := fmt.Sprintf("%s/search/movie", c.config.BaseURL)

	// Build request with query params
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, NewNetworkError(err)
	}

	q := req.URL.Query()
	q.Add("query", query)
	if year > 0 {
		q.Add("year", strconv.Itoa(year))
	}
	req.URL.RawQuery = q.Encode()

	body, err := c.doHTTPRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var resp SearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal search response: %w", err)
	}

	results := make([]*metadata.MovieMetadata, 0, len(resp.Results))
	for _, result := range resp.Results {
		if result.MediaType == "movie" || result.Title != "" {
			m := &metadata.MovieMetadata{
				Title:      result.Title,
				Overview:   cropOverview(result.Overview),
				PosterPath: buildPosterURL(result.PosterPath),
				Rating:     result.VoteAverage,
			}
			// Parse year from release date
			if result.ReleaseDate != "" && len(result.ReleaseDate) >= 4 {
				if year, err := strconv.Atoi(result.ReleaseDate[:4]); err == nil {
					m.Year = year
				}
			}
			// Set TMDB ID
			tmdbIDStr := strconv.Itoa(result.ID)
			m.TMDBID = &tmdbIDStr
			results = append(results, m)
		}
	}

	return results, nil
}

// SearchTV searches for TV series by title and optional year.
func (c *Client) SearchTV(ctx context.Context, query string, year int) ([]*metadata.SeriesMetadata, error) {
	url := fmt.Sprintf("%s/search/tv", c.config.BaseURL)

	// Build request with query params
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, NewNetworkError(err)
	}

	q := req.URL.Query()
	q.Add("query", query)
	if year > 0 {
		q.Add("first_air_date_year", strconv.Itoa(year))
	}
	req.URL.RawQuery = q.Encode()

	body, err := c.doHTTPRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var resp SearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal search response: %w", err)
	}

	results := make([]*metadata.SeriesMetadata, 0, len(resp.Results))
	for _, result := range resp.Results {
		if result.MediaType == "tv" || result.Name != "" {
			s := &metadata.SeriesMetadata{
				Title:      result.Name,
				Overview:   cropOverview(result.Overview),
				PosterPath: buildPosterURL(result.PosterPath),
				Rating:     result.VoteAverage,
			}
			// Set TMDB ID
			tmdbIDStr := strconv.Itoa(result.ID)
			s.TMDBID = &tmdbIDStr
			results = append(results, s)
		}
	}

	return results, nil
}

// GetMovieCredits fetches cast and crew for a movie by TMDb ID.
func (c *Client) GetMovieCredits(ctx context.Context, tmdbID int) (*metadata.Credits, error) {
	url := fmt.Sprintf("%s/movie/%d/credits", c.config.BaseURL, tmdbID)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp CreditsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal credits response: %w", err)
	}

	return mapCreditsResponse(&resp), nil
}

// GetEpisode fetches episode metadata by TV ID, season, and episode number.
func (c *Client) GetEpisode(ctx context.Context, tvID, season, episode int) (*metadata.EpisodeMetadata, error) {
	url := fmt.Sprintf("%s/tv/%d/season/%d/episode/%d", c.config.BaseURL, tvID, season, episode)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp EpisodeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal episode response: %w", err)
	}

	return mapEpisodeResponse(&resp), nil
}

// GetReleaseDates fetches release dates for a movie from TMDB.
// It returns the best theatrical and digital dates, preferring US releases.
func (c *Client) GetReleaseDates(ctx context.Context, tmdbID int) (theatrical, digital string, err error) {
	url := fmt.Sprintf("%s/movie/%d/release_dates", c.config.BaseURL, tmdbID)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return "", "", err
	}

	var resp ReleaseDatesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("tmdb: failed to unmarshal release_dates response: %w", err)
	}

	// Priority: US > GB > first available
	preferredCountries := []string{"US", "GB"}

	var bestTheatrical, bestDigital string

	for _, country := range preferredCountries {
		for _, r := range resp.Results {
			if r.ISO31661 != country {
				continue
			}
			for _, rd := range r.ReleaseDates {
				date := rd.ReleaseDate
				if len(date) >= 10 {
					date = date[:10] // trim to YYYY-MM-DD
				}
				if (rd.Type == 2 || rd.Type == 3) && bestTheatrical == "" {
					bestTheatrical = date
				}
				if rd.Type == 4 && bestDigital == "" {
					bestDigital = date
				}
			}
		}
		if bestTheatrical != "" || bestDigital != "" {
			break
		}
	}

	// Fallback: scan all countries if preferred ones had nothing
	if bestTheatrical == "" && bestDigital == "" {
		for _, r := range resp.Results {
			for _, rd := range r.ReleaseDates {
				date := rd.ReleaseDate
				if len(date) >= 10 {
					date = date[:10]
				}
				if (rd.Type == 2 || rd.Type == 3) && bestTheatrical == "" {
					bestTheatrical = date
				}
				if rd.Type == 4 && bestDigital == "" {
					bestDigital = date
				}
			}
			if bestTheatrical != "" && bestDigital != "" {
				break
			}
		}
	}

	return bestTheatrical, bestDigital, nil
}

// GetPerson fetches person details by TMDb person ID.
func (c *Client) GetPerson(ctx context.Context, personID int) (*PersonResponse, error) {
	url := fmt.Sprintf("%s/person/%d", c.config.BaseURL, personID)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp PersonResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal person response: %w", err)
	}

	return &resp, nil
}

// GetPersonCredits fetches the combined movie + TV credits for a person.
func (c *Client) GetPersonCredits(ctx context.Context, personID int) (*CombinedCreditsResponse, error) {
	url := fmt.Sprintf("%s/person/%d/combined_credits", c.config.BaseURL, personID)
	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp CombinedCreditsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tmdb: failed to unmarshal combined credits response: %w", err)
	}

	return &resp, nil
}

// doRequest is a helper that adds API key and calls doHTTPRequest.
func (c *Client) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, NewNetworkError(err)
	}

	return c.doHTTPRequest(ctx, req)
}

// doHTTPRequest executes a request with exponential backoff on 429 (rate limit).
func (c *Client) doHTTPRequest(ctx context.Context, req *http.Request) ([]byte, error) {
	// Set auth: Bearer token for v4 JWT, or api_key param for v3 key
	if len(c.config.APIKey) > 100 {
		// JWT bearer token (v4 read access token)
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	} else {
		// v3 API key
		q := req.URL.Query()
		q.Add("api_key", c.config.APIKey)
		req.URL.RawQuery = q.Encode()
	}

	backoffDelay := c.backoffConfig.InitialDelay

	for {
		// Check context before attempting request
		select {
		case <-ctx.Done():
			return nil, NewContextError(ctx.Err())
		default:
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Network error; don't retry, but propagate context errors specially
			if ctx.Err() != nil {
				return nil, NewContextError(ctx.Err())
			}
			return nil, NewNetworkError(err)
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, NewNetworkError(err)
		}

		// Handle HTTP status codes
		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil

		case http.StatusNotFound:
			return nil, NewNotFoundError("resource not found")

		case http.StatusUnauthorized:
			return nil, NewUnauthorizedError("invalid API key")

		case http.StatusTooManyRequests:
			// Parse Retry-After header if present
			retryAfter := int(c.backoffConfig.MaxDelay.Seconds())
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if seconds, err := strconv.Atoi(ra); err == nil {
					retryAfter = seconds
				}
			}

			// Wait and retry (unless context is done)
			delay := time.Duration(retryAfter) * time.Second
			select {
			case <-ctx.Done():
				return nil, NewContextError(ctx.Err())
			case <-time.After(delay):
				// Retry with fresh request
				req2, _ := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), nil)
				*req = *req2
				continue
			}

		case http.StatusBadRequest, http.StatusForbidden, http.StatusMethodNotAllowed:
			// 4xx client error (not retryable)
			var errResp ErrorResponse
			_ = json.Unmarshal(body, &errResp)
			msg := errResp.StatusMessage
			if msg == "" {
				msg = http.StatusText(resp.StatusCode)
			}
			return nil, NewClientError(resp.StatusCode, msg)

		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			// 5xx server error; apply exponential backoff
			select {
			case <-ctx.Done():
				return nil, NewContextError(ctx.Err())
			case <-time.After(backoffDelay):
				// Update backoff for next retry
				backoffDelay = time.Duration(
					math.Min(
						float64(backoffDelay)*c.backoffConfig.Multiplier,
						float64(c.backoffConfig.MaxDelay),
					),
				)
				// Retry with fresh request
				req2, _ := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), nil)
				*req = *req2
				continue
			}

		default:
			// Other HTTP error
			var errResp ErrorResponse
			_ = json.Unmarshal(body, &errResp)
			msg := errResp.StatusMessage
			if msg == "" {
				msg = http.StatusText(resp.StatusCode)
			}
			return nil, NewClientError(resp.StatusCode, msg)
		}
	}
}
