package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds MusicBrainz client configuration.
type Config struct {
	BaseURL string        // MusicBrainz API base URL (default: https://musicbrainz.org/ws/2)
	Timeout time.Duration // HTTP request timeout (default: 10s)
}

// DefaultConfig returns the default MusicBrainz configuration.
func DefaultConfig() *Config {
	return &Config{
		BaseURL: "https://musicbrainz.org/ws/2",
		Timeout: 10 * time.Second,
	}
}

// Client is the MusicBrainz HTTP client.
// It manages rate limiting, retries, and request throttling.
type Client struct {
	config     *Config
	httpClient *http.Client

	// Throttler enforces 1s minimum delay between requests.
	throttler *Throttler
}

// Throttler ensures at most one request per second (MusicBrainz rate limit).
// It uses a mutex to serialize requests and a timer for the 1s delay.
type Throttler struct {
	mu       sync.Mutex
	lastReq  time.Time
	interval time.Duration
}

// NewThrottler creates a new throttler with the specified interval.
func NewThrottler(interval time.Duration) *Throttler {
	return &Throttler{
		interval: interval,
		lastReq:  time.Now().Add(-interval), // Allow first request immediately
	}
}

// Wait blocks until the throttler permits a new request.
// It ensures at least 'interval' has elapsed since the last request.
func (t *Throttler) Wait() {
	t.mu.Lock()
	defer t.mu.Unlock()

	elapsed := time.Since(t.lastReq)
	if elapsed < t.interval {
		time.Sleep(t.interval - elapsed)
	}
	t.lastReq = time.Now()
}

// NewClient creates a new MusicBrainz client with the given config.
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	return &Client{
		config:     cfg,
		httpClient: httpClient,
		throttler:  NewThrottler(1 * time.Second),
	}
}

// GetArtist fetches an artist by MBID.
func (c *Client) GetArtist(ctx context.Context, mbid string) (*ArtistMetadata, error) {
	url := fmt.Sprintf("%s/artist/%s?fmt=json&inc=area+genres", c.config.BaseURL, url.QueryEscape(mbid))
	resp := &ArtistResponse{}

	if err := c.doRequest(ctx, url, resp); err != nil {
		return nil, err
	}

	return MapArtist(resp), nil
}

// GetRelease fetches a release/album by MBID.
func (c *Client) GetRelease(ctx context.Context, mbid string) (*ReleaseMetadata, error) {
	url := fmt.Sprintf("%s/release/%s?fmt=json&inc=artist-credits+media+release-groups", c.config.BaseURL, url.QueryEscape(mbid))
	resp := &ReleaseResponse{}

	if err := c.doRequest(ctx, url, resp); err != nil {
		return nil, err
	}

	return MapRelease(resp), nil
}

// GetRecording fetches a recording/track by MBID.
func (c *Client) GetRecording(ctx context.Context, mbid string) (*RecordingMetadata, error) {
	url := fmt.Sprintf("%s/recording/%s?fmt=json&inc=artist-credits", c.config.BaseURL, url.QueryEscape(mbid))
	resp := &RecordingResponse{}

	if err := c.doRequest(ctx, url, resp); err != nil {
		return nil, err
	}

	return MapRecording(resp), nil
}

// SearchArtist searches for artists by query string.
// Returns a list of matching artists with pagination support.
func (c *Client) SearchArtist(ctx context.Context, query string, offset, limit int) ([]*ArtistMetadata, error) {
	params := url.Values{
		"fmt":    {"json"},
		"query":  {query},
		"offset": {strconv.Itoa(offset)},
		"limit":  {strconv.Itoa(limit)},
	}
	url := fmt.Sprintf("%s/artist?%s", c.config.BaseURL, params.Encode())

	resp := &SearchResponse{}
	if err := c.doRequest(ctx, url, resp); err != nil {
		return nil, err
	}

	var results []*ArtistMetadata
	for _, artist := range resp.Artists {
		if mapped := MapArtist(&artist); mapped != nil {
			results = append(results, mapped)
		}
	}

	return results, nil
}

// SearchRelease searches for releases/albums by query string.
// Returns a list of matching releases with pagination support.
func (c *Client) SearchRelease(ctx context.Context, query string, offset, limit int) ([]*ReleaseMetadata, error) {
	params := url.Values{
		"fmt":    {"json"},
		"query":  {query},
		"offset": {strconv.Itoa(offset)},
		"limit":  {strconv.Itoa(limit)},
	}
	url := fmt.Sprintf("%s/release?%s", c.config.BaseURL, params.Encode())

	resp := &SearchResponse{}
	if err := c.doRequest(ctx, url, resp); err != nil {
		return nil, err
	}

	var results []*ReleaseMetadata
	for _, release := range resp.Releases {
		if mapped := MapRelease(&release); mapped != nil {
			results = append(results, mapped)
		}
	}

	return results, nil
}

// doRequest performs an HTTP request with error handling, retries, and throttling.
// It implements exponential backoff for 429 responses and enforces the 1s throttle.
func (c *Client) doRequest(ctx context.Context, endpoint string, result interface{}) error {
	// Enforce 1s minimum delay between requests
	c.throttler.Wait()

	// Exponential backoff parameters
	const (
		maxRetries = 5
		initialBackoff = 1 * time.Second
		maxBackoff = 60 * time.Second
	)

	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return NewContextError(ctx.Err())
		}

		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return NewNetworkError(err)
		}

		// Set User-Agent header (required by MusicBrainz)
		// MusicBrainz requires a descriptive User-Agent to identify clients
		req.Header.Set("User-Agent", "Loom/1.0 (metadata_service)")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Network error; retry with backoff
			if attempt < maxRetries {
				time.Sleep(backoff)
				backoff = c.nextBackoff(backoff, maxBackoff)
				continue
			}
			return NewNetworkError(err)
		}
		defer resp.Body.Close()

		// Handle HTTP status codes
		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated:
			// Success; decode and return
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				return NewNetworkError(err)
			}
			return nil

		case http.StatusNotFound:
			// 404: Entity not found (no retry)
			body, _ := io.ReadAll(resp.Body)
			return NewNotFoundError(fmt.Sprintf("entity not found: %s", string(body)))

		case http.StatusTooManyRequests:
			// 429: Rate limited; parse Retry-After header and backoff
			retryAfter := c.parseRetryAfter(resp, backoff)
			if attempt < maxRetries {
				time.Sleep(retryAfter)
				backoff = c.nextBackoff(backoff, maxBackoff)
				continue
			}
			return NewRateLimitError("rate limit exceeded, max retries reached", int(retryAfter.Seconds()))

		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
			// 4xx client errors (not retryable)
			body, _ := io.ReadAll(resp.Body)
			return NewClientError(resp.StatusCode, fmt.Sprintf("client error: %s", string(body)))

		case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			// 5xx server errors; retry with backoff
			if attempt < maxRetries {
				time.Sleep(backoff)
				backoff = c.nextBackoff(backoff, maxBackoff)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			return NewServerError(resp.StatusCode, fmt.Sprintf("server error: %s", string(body)))

		default:
			if resp.StatusCode >= 500 {
				// Server error; retry with backoff
				if attempt < maxRetries {
					time.Sleep(backoff)
					backoff = c.nextBackoff(backoff, maxBackoff)
					continue
				}
				body, _ := io.ReadAll(resp.Body)
				return NewServerError(resp.StatusCode, fmt.Sprintf("server error: %s", string(body)))
			}

			// Other 4xx errors
			body, _ := io.ReadAll(resp.Body)
			return NewClientError(resp.StatusCode, fmt.Sprintf("client error: %s", string(body)))
		}
	}

	// Exhausted retries (should not reach here)
	return NewServerError(http.StatusServiceUnavailable, "max retries exhausted")
}

// parseRetryAfter extracts the Retry-After duration from response headers.
// Falls back to exponential backoff if header is not present.
func (c *Client) parseRetryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return fallback
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		if duration := time.Until(t); duration > 0 {
			return duration
		}
	}

	return fallback
}

// nextBackoff calculates the next backoff duration (exponential, capped at max).
func (c *Client) nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
