package rss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/PuerkitoBio/goquery"
)

// RateLimiter implements a token bucket rate limiter per domain.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*TokenBucket
	refillHz float64 // tokens per second (default: 1 = 1 request/sec)
}

// TokenBucket is a simple token bucket for rate limiting.
type TokenBucket struct {
	tokens    float64
	maxTokens float64
	lastTime  time.Time
	refillHz  float64
}

// NewRateLimiter creates a new rate limiter with specified refill rate (tokens/sec).
func NewRateLimiter(refillHz float64) *RateLimiter {
	if refillHz <= 0 {
		refillHz = 1.0 // Default: 1 request/sec
	}
	return &RateLimiter{
		buckets:  make(map[string]*TokenBucket),
		refillHz: refillHz,
	}
}

// Allow checks if a domain is allowed to make a request.
// Returns (allowed, waitDuration).
func (rl *RateLimiter) Allow(domain string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[domain]
	if !ok {
		bucket = &TokenBucket{
			tokens:    1.0,
			maxTokens: 1.0,
			lastTime:  time.Now(),
			refillHz:  rl.refillHz,
		}
		rl.buckets[domain] = bucket
	}

	now := time.Now()
	elapsed := now.Sub(bucket.lastTime).Seconds()
	bucket.tokens = min(bucket.maxTokens, bucket.tokens+elapsed*bucket.refillHz)
	bucket.lastTime = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, 0
	}

	// Calculate wait time
	tokensNeeded := 1.0 - bucket.tokens
	waitSec := tokensNeeded / bucket.refillHz
	return false, time.Duration(waitSec*1000) * time.Millisecond
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// WebScraper implements FeedSource for HTML web scraping.
type WebScraper struct {
	id          string
	name        string
	config      ScraperConfig
	logger      *slog.Logger
	rateLimiter *RateLimiter
	httpClient  *http.Client
}

// NewWebScraper creates a new web scraper from config.
func NewWebScraper(logger *slog.Logger, id, name string, cfg ScraperConfig) (*WebScraper, error) {
	if cfg.URL == "" {
		return nil, errors.New("scraper URL required")
	}
	if cfg.SelectorType != "css" && cfg.SelectorType != "xpath" {
		return nil, errors.New("selector_type must be 'css' or 'xpath'")
	}
	if cfg.ItemSelector == "" {
		return nil, errors.New("item_selector required")
	}
	if cfg.TitleSelector == "" {
		return nil, errors.New("title_selector required")
	}

	// Set defaults for timeouts
	connectTimeout := 10
	readTimeout := 30

	if logger == nil {
		logger = slog.Default()
	}

	// Create HTTP client with timeouts
	httpClient := &http.Client{
		Timeout: time.Duration(readTimeout) * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: time.Duration(connectTimeout) * time.Second,
			}).DialContext,
		},
	}

	return &WebScraper{
		id:          id,
		name:        name,
		config:      cfg,
		logger:      logger,
		rateLimiter: NewRateLimiter(1.0), // 1 request/sec per domain
		httpClient:  httpClient,
	}, nil
}

// ID returns the scraper ID.
func (ws *WebScraper) ID() string {
	return ws.id
}

// Name returns the scraper name.
func (ws *WebScraper) Name() string {
	return ws.name
}

// RefreshInterval returns a default refresh interval (1 hour for web scrapers).
func (ws *WebScraper) RefreshInterval() time.Duration {
	return time.Hour
}

// Fetch fetches content from the configured URL(s) and extracts items.
func (ws *WebScraper) Fetch(ctx interface{}) ([]*Item, error) {
	goCtx, ok := ctx.(context.Context)
	if !ok {
		goCtx = context.Background()
	}

	var items []*Item
	var lastErr error

	// Determine pagination range
	startPage := 1
	endPage := 1
	if ws.config.Pagination.PageSize > 0 {
		// Pagination is configured; try a few pages
		endPage = 3 // Fetch up to 3 pages by default
	}

	// Fetch and parse each page
	for pageNum := startPage; pageNum <= endPage; pageNum++ {
		pageItems, err := ws.fetchPage(goCtx, pageNum)
		if err != nil {
			ws.logger.Warn("scraper: error fetching page",
				"scraper", ws.name,
				"page", pageNum,
				"error", err)
			lastErr = err
			// Stop on error; don't continue to next page
			break
		}
		if len(pageItems) == 0 {
			// No more items
			break
		}
		items = append(items, pageItems...)
	}

	// Return error if we got one on the first page
	if len(items) == 0 && lastErr != nil {
		return nil, lastErr
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items extracted")
	}

	return items, nil
}

// fetchPage fetches and parses a single page.
func (ws *WebScraper) fetchPage(ctx context.Context, pageNum int) ([]*Item, error) {
	// Build URL
	url := ws.buildURL(pageNum)

	// Rate limit
	domain := extractDomain(url)
	allowed, waitDur := ws.rateLimiter.Allow(domain)
	if !allowed {
		select {
		case <-time.After(waitDur):
			// Proceed after wait
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Fetch with exponential backoff
	body, err := ws.fetchWithRetry(ctx, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	// Parse and extract
	items, err := ws.parseAndExtract(body)
	if err != nil {
		return nil, err
	}

	return items, nil
}

// buildURL constructs the URL for a given page number.
func (ws *WebScraper) buildURL(pageNum int) string {
	url := ws.config.URL

	// Apply pagination parameters if configured
	if ws.config.Pagination.PageParam != "" && pageNum > 1 {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += fmt.Sprintf("%s%s=%d", sep, ws.config.Pagination.PageParam, pageNum)
	} else if ws.config.Pagination.OffsetParam != "" && pageNum > 1 {
		offset := (pageNum - 1) * ws.config.Pagination.PageSize
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += fmt.Sprintf("%s%s=%d", sep, ws.config.Pagination.OffsetParam, offset)
	}

	return url
}

// fetchWithRetry fetches the URL with exponential backoff retry.
func (ws *WebScraper) fetchWithRetry(ctx context.Context, url string) (io.ReadCloser, error) {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: delay = baseDelay * 2^(attempt-1)
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			select {
			case <-time.After(delay):
				// Proceed
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		// Add auth headers
		ws.addAuthHeaders(req)

		// Add User-Agent
		req.Header.Set("User-Agent", "Loom/1.0 (+https://github.com/loomctl/loom)")

		resp, err := ws.httpClient.Do(req)
		if err != nil {
			lastErr = err
			// Retry on network errors
			if isRetryableError(err) {
				continue
			}
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			return resp.Body, nil
		}

		resp.Body.Close()

		// Retry on 5xx errors
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		// Don't retry 4xx errors
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return nil, errors.New("max retries exceeded")
}

// isRetryableError checks if an error is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Retry on network errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connection reset")
}

// addAuthHeaders adds authentication headers to the request.
func (ws *WebScraper) addAuthHeaders(req *http.Request) {
	switch ws.config.AuthType {
	case "basic":
		req.SetBasicAuth(ws.config.Username, ws.config.Password)
	case "apikey":
		req.Header.Set("X-API-Key", ws.config.APIKey)
	}
}

// parseAndExtract parses HTML and extracts items using configured selectors.
func (ws *WebScraper) parseAndExtract(body io.Reader) ([]*Item, error) {
	// Read all content
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var items []*Item

	if ws.config.SelectorType == "css" {
		items, err = ws.extractWithCSS(string(content))
	} else {
		items, err = ws.extractWithXPath(string(content))
	}

	return items, err
}

// extractWithCSS extracts items using CSS selectors.
func (ws *WebScraper) extractWithCSS(html string) ([]*Item, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	var items []*Item
	seen := make(map[string]bool)

	// Find all matching item containers
	doc.Find(ws.config.ItemSelector).Each(func(i int, itemSel *goquery.Selection) {
		// Get title
		title := strings.TrimSpace(itemSel.Find(ws.config.TitleSelector).Text())
		if title == "" {
			return // Skip empty titles
		}

		// Get link
		link := ""
		if ws.config.LinkSelector != "" {
			link, _ = itemSel.Find(ws.config.LinkSelector).Attr("href")
		}
		if link == "" {
			// Try to find any link
			link, _ = itemSel.Find("a").Attr("href")
		}

		// Skip duplicates
		guid := fmt.Sprintf("%s_%s", title, link)
		if seen[guid] {
			return
		}
		seen[guid] = true

		// Get optional publication date
		pubDate := time.Now()
		if ws.config.PublishedSelector != "" {
			dateText := strings.TrimSpace(itemSel.Find(ws.config.PublishedSelector).Text())
			if dateText != "" {
				// Try common date formats
				if parsed, err := time.Parse(time.RFC3339, dateText); err == nil {
					pubDate = parsed
				} else if parsed, err := time.Parse("2006-01-02", dateText); err == nil {
					pubDate = parsed
				} else if parsed, err := time.Parse(time.RFC1123, dateText); err == nil {
					pubDate = parsed
				}
			}
		}

		items = append(items, &Item{
			Title:       title,
			Link:        link,
			PublishedAt: pubDate,
			GUID:        guid,
		})
	})

	return items, nil
}

// extractWithXPath extracts items using XPath selectors.
func (ws *WebScraper) extractWithXPath(html string) ([]*Item, error) {
	doc, err := htmlquery.Parse(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	var items []*Item
	seen := make(map[string]bool)

	// Find all matching item containers
	itemNodes, err := htmlquery.QueryAll(doc, ws.config.ItemSelector)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}

	for _, itemNode := range itemNodes {
		// Get title
		titleNodes, err := htmlquery.QueryAll(itemNode, ws.config.TitleSelector)
		if err != nil || len(titleNodes) == 0 {
			continue
		}
		title := strings.TrimSpace(htmlquery.InnerText(titleNodes[0]))
		if title == "" {
			continue
		}

		// Get link
		link := ""
		if ws.config.LinkSelector != "" {
			linkNodes, _ := htmlquery.QueryAll(itemNode, ws.config.LinkSelector)
			if len(linkNodes) > 0 {
				link = htmlquery.SelectAttr(linkNodes[0], "href")
			}
		}
		if link == "" {
			// Try to find any link
			aNodes, _ := htmlquery.QueryAll(itemNode, ".//a")
			if len(aNodes) > 0 {
				link = htmlquery.SelectAttr(aNodes[0], "href")
			}
		}

		// Skip duplicates
		guid := fmt.Sprintf("%s_%s", title, link)
		if seen[guid] {
			continue
		}
		seen[guid] = true

		// Get optional publication date
		pubDate := time.Now()
		if ws.config.PublishedSelector != "" {
			dateNodes, err := htmlquery.QueryAll(itemNode, ws.config.PublishedSelector)
			if err == nil && len(dateNodes) > 0 {
				dateText := strings.TrimSpace(htmlquery.InnerText(dateNodes[0]))
				if dateText != "" {
					// Try common date formats
					if parsed, err := time.Parse(time.RFC3339, dateText); err == nil {
						pubDate = parsed
					} else if parsed, err := time.Parse("2006-01-02", dateText); err == nil {
						pubDate = parsed
					} else if parsed, err := time.Parse(time.RFC1123, dateText); err == nil {
						pubDate = parsed
					}
				}
			}
		}

		items = append(items, &Item{
			Title:       title,
			Link:        link,
			PublishedAt: pubDate,
			GUID:        guid,
		})
	}

	return items, nil
}

// extractDomain extracts the domain from a URL.
func extractDomain(urlStr string) string {
	// Simple extraction: just get the host part
	if idx := strings.Index(urlStr, "://"); idx >= 0 {
		urlStr = urlStr[idx+3:]
	}
	if idx := strings.Index(urlStr, "/"); idx >= 0 {
		urlStr = urlStr[:idx]
	}
	return urlStr
}
