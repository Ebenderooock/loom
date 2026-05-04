package rss

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScraperNewValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ScraperConfig
		wantErr string
	}{
		{
			name:    "missing URL",
			cfg:     ScraperConfig{SelectorType: "css", ItemSelector: "a", TitleSelector: "h1"},
			wantErr: "URL required",
		},
		{
			name:    "invalid selector type",
			cfg:     ScraperConfig{URL: "http://example.com", SelectorType: "regex", ItemSelector: "a", TitleSelector: "h1"},
			wantErr: "must be 'css' or 'xpath'",
		},
		{
			name:    "missing item selector",
			cfg:     ScraperConfig{URL: "http://example.com", SelectorType: "css", TitleSelector: "h1"},
			wantErr: "item_selector required",
		},
		{
			name:    "missing title selector",
			cfg:     ScraperConfig{URL: "http://example.com", SelectorType: "css", ItemSelector: "a"},
			wantErr: "title_selector required",
		},
		{
			name: "valid config",
			cfg: ScraperConfig{
				URL:            "http://example.com",
				SelectorType:   "css",
				ItemSelector:   ".item",
				TitleSelector:  "h2",
				LinkSelector:   "a",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWebScraper(quietLogger(), "test", "test", tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestScraperCSSExtraction(t *testing.T) {
	html := `
	<html>
	<body>
		<div class="item">
			<h2 class="title">Movie One</h2>
			<a class="link" href="/movie/1">Download</a>
			<span class="date">2026-05-01T12:00:00Z</span>
		</div>
		<div class="item">
			<h2 class="title">Movie Two</h2>
			<a class="link" href="/movie/2">Download</a>
		</div>
	</body>
	</html>
	`

	cfg := ScraperConfig{
		URL:              "http://example.com",
		SelectorType:     "css",
		ItemSelector:     ".item",
		TitleSelector:    ".title",
		LinkSelector:     ".link",
		PublishedSelector: ".date",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	items, err := scraper.extractWithCSS(html)
	if err != nil {
		t.Fatalf("extraction failed: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	if items[0].Title != "Movie One" {
		t.Errorf("expected title 'Movie One', got %q", items[0].Title)
	}
	if items[0].Link != "/movie/1" {
		t.Errorf("expected link '/movie/1', got %q", items[0].Link)
	}

	if items[1].Title != "Movie Two" {
		t.Errorf("expected title 'Movie Two', got %q", items[1].Title)
	}
}

func TestScraperXPathExtraction(t *testing.T) {
	html := `
	<html>
	<body>
		<item>
			<title>Movie One</title>
			<link>/movie/1</link>
		</item>
		<item>
			<title>Movie Two</title>
			<link>/movie/2</link>
		</item>
	</body>
	</html>
	`

	cfg := ScraperConfig{
		URL:              "http://example.com",
		SelectorType:     "xpath",
		ItemSelector:     "//item",
		TitleSelector:    "./title",
		LinkSelector:     "./link",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	items, err := scraper.extractWithXPath(html)
	if err != nil {
		t.Fatalf("extraction failed: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	if items[0].Title != "Movie One" {
		t.Errorf("expected title 'Movie One', got %q", items[0].Title)
	}

	if items[1].Title != "Movie Two" {
		t.Errorf("expected title 'Movie Two', got %q", items[1].Title)
	}
}

func TestScraperDuplicateDetection(t *testing.T) {
	html := `
	<html>
	<body>
		<div class="item">
			<h2 class="title">Movie One</h2>
			<a class="link" href="/movie/1">Download</a>
		</div>
		<div class="item">
			<h2 class="title">Movie One</h2>
			<a class="link" href="/movie/1">Download</a>
		</div>
	</body>
	</html>
	`

	cfg := ScraperConfig{
		URL:           "http://example.com",
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".title",
		LinkSelector:  ".link",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	items, err := scraper.extractWithCSS(html)
	if err != nil {
		t.Fatalf("extraction failed: %v", err)
	}

	// Should only extract 1 item (duplicate filtered)
	if len(items) != 1 {
		t.Errorf("expected 1 unique item, got %d", len(items))
	}
}

func TestScraperHTTPFetch(t *testing.T) {
	html := `
	<html>
	<body>
		<div class="item">
			<h2 class="title">Fetched Movie</h2>
			<a class="link" href="/movie/1">Download</a>
		</div>
	</body>
	</html>
	`

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	cfg := ScraperConfig{
		URL:           server.URL,
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".title",
		LinkSelector:  ".link",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	items, err := scraper.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Fetched Movie" {
		t.Errorf("expected title 'Fetched Movie', got %q", items[0].Title)
	}
}

func TestScraperRateLimiting(t *testing.T) {
	rl := NewRateLimiter(10.0) // 10 requests per second

	// First request should succeed immediately
	allowed, dur := rl.Allow("example.com")
	if !allowed || dur != 0 {
		t.Error("first request should be allowed immediately")
	}

	// Subsequent requests should be throttled
	for i := 0; i < 5; i++ {
		allowed, dur := rl.Allow("example.com")
		if !allowed {
			if dur == 0 {
				t.Error("expected wait duration for throttled request")
			}
		}
	}

	// Different domain should not be throttled
	allowed, dur = rl.Allow("other.com")
	if !allowed || dur != 0 {
		t.Error("different domain should not be throttled")
	}
}

func TestScraperURLBuilding(t *testing.T) {
	tests := []struct {
		name     string
		cfg      ScraperConfig
		pageNum  int
		expected string
	}{
		{
			name: "no pagination",
			cfg: func() ScraperConfig {
				c := ScraperConfig{
					URL:           "http://example.com/list",
					SelectorType:  "css",
					ItemSelector:  ".item",
					TitleSelector: ".t",
				}
				c.Pagination.Type = "none"
				return c
			}(),
			pageNum:  1,
			expected: "http://example.com/list",
		},
		{
			name: "page number pagination",
			cfg: func() ScraperConfig {
				c := ScraperConfig{
					URL:           "http://example.com/list",
					SelectorType:  "css",
					ItemSelector:  ".item",
					TitleSelector: ".t",
				}
				c.Pagination.Type = "page_number"
				c.Pagination.PageParam = "page"
				c.Pagination.PageSize = 20
				return c
			}(),
			pageNum:  2,
			expected: "http://example.com/list?page=2",
		},
		{
			name: "offset pagination",
			cfg: func() ScraperConfig {
				c := ScraperConfig{
					URL:           "http://example.com/list",
					SelectorType:  "css",
					ItemSelector:  ".item",
					TitleSelector: ".t",
				}
				c.Pagination.Type = "offset"
				c.Pagination.OffsetParam = "skip"
				c.Pagination.PageSize = 20
				return c
			}(),
			pageNum:  2,
			expected: "http://example.com/list?skip=20",
		},
		{
			name: "existing query params",
			cfg: func() ScraperConfig {
				c := ScraperConfig{
					URL:           "http://example.com/list?sort=title",
					SelectorType:  "css",
					ItemSelector:  ".item",
					TitleSelector: ".t",
				}
				c.Pagination.Type = "page_number"
				c.Pagination.PageParam = "page"
				c.Pagination.PageSize = 20
				return c
			}(),
			pageNum:  2,
			expected: "http://example.com/list?sort=title&page=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scraper, err := NewWebScraper(quietLogger(), "test", "test", tt.cfg)
			if err != nil {
				t.Fatalf("failed to create scraper: %v", err)
			}

			url := scraper.buildURL(tt.pageNum)
			if url != tt.expected {
				t.Errorf("expected URL %q, got %q", tt.expected, url)
			}
		})
	}
}

func TestScraperAuthHeaders(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		xkey := r.Header.Get("X-API-Key")
		if auth != "" {
			capturedAuth = auth
		}
		if xkey != "" {
			capturedAuth = xkey
		}
		fmt.Fprint(w, `<div class="item"><h1 class="t">T</h1></div>`)
	}))
	defer server.Close()

	tests := []struct {
		name       string
		authType   string
		username   string
		password   string
		apiKey     string
		wantAuth   string
	}{
		{
			name:     "basic auth",
			authType: "basic",
			username: "user",
			password: "pass",
			wantAuth: "Basic",
		},
		{
			name:     "api key auth",
			authType: "apikey",
			apiKey:   "secret123",
			wantAuth: "secret123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedAuth = ""
			cfg := ScraperConfig{
				URL:            server.URL,
				SelectorType:   "css",
				ItemSelector:   ".item",
				TitleSelector:  ".t",
				AuthType:       tt.authType,
				Username:       tt.username,
				Password:       tt.password,
				APIKey:         tt.apiKey,
			}

			scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
			if err != nil {
				t.Fatalf("failed to create scraper: %v", err)
			}

			_, _ = scraper.Fetch(context.Background())

			if !strings.Contains(capturedAuth, tt.wantAuth) {
				t.Errorf("expected auth header containing %q, got %q", tt.wantAuth, capturedAuth)
			}
		})
	}
}

func TestScraperHTTPRetry(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			// Simulate server error on first attempt
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `<div class="item"><h1 class="t">Retried</h1></div>`)
	}))
	defer server.Close()

	cfg := ScraperConfig{
		URL:           server.URL,
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".t",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	items, err := scraper.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if len(items) != 1 || items[0].Title != "Retried" {
		t.Error("retry should have succeeded on second attempt")
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestScraperContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `<div class="item"><h1 class="t">T</h1></div>`)
	}))
	defer server.Close()

	cfg := ScraperConfig{
		URL:           server.URL,
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".t",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = scraper.Fetch(ctx)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestScraperErrorHandling(t *testing.T) {
	cfg := ScraperConfig{
		URL:           "http://invalid-domain-that-does-not-exist-12345.com",
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".t",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	_, err = scraper.Fetch(context.Background())
	if err == nil {
		t.Error("expected fetch error for invalid domain")
	}
}

func TestScraperInterfaceCompliance(t *testing.T) {
	cfg := ScraperConfig{
		URL:           "http://example.com",
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".t",
	}

	scraper, err := NewWebScraper(quietLogger(), "id123", "test-scraper", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	// Verify FeedSource interface implementation
	if scraper.ID() != "id123" {
		t.Errorf("expected ID 'id123', got %q", scraper.ID())
	}
	if scraper.Name() != "test-scraper" {
		t.Errorf("expected name 'test-scraper', got %q", scraper.Name())
	}

	interval := scraper.RefreshInterval()
	if interval != time.Hour {
		t.Errorf("expected 1 hour refresh interval, got %v", interval)
	}

	// Verify Fetch() signature matches FeedSource interface
	// (it should accept interface{} and return []*Item, error)
	var _ FeedSource = scraper
}

func TestScraperEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body></body></html>`)
	}))
	defer server.Close()

	cfg := ScraperConfig{
		URL:           server.URL,
		SelectorType:  "css",
		ItemSelector:  ".nonexistent",
		TitleSelector: ".t",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	_, err = scraper.Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no items extracted") {
		t.Error("expected 'no items extracted' error")
	}
}

func TestScraperHTTP404Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := ScraperConfig{
		URL:           server.URL,
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".t",
	}

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	_, err = scraper.Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Error("expected HTTP 404 error")
	}
}

func TestScraperExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://example.com/path", "example.com"},
		{"https://sub.example.com:8080/path", "sub.example.com:8080"},
		{"http://localhost/path", "localhost"},
		{"http://192.168.1.1:3000/path", "192.168.1.1:3000"},
	}

	for _, tt := range tests {
		domain := extractDomain(tt.url)
		if domain != tt.expected {
			t.Errorf("expected domain %q, got %q", tt.expected, domain)
		}
	}
}

func TestScraperBuildURLFirstPage(t *testing.T) {
	// First page should not include pagination params
	cfg := func() ScraperConfig {
		c := ScraperConfig{
			URL:           "http://example.com/list",
			SelectorType:  "css",
			ItemSelector:  ".item",
			TitleSelector: ".t",
		}
		c.Pagination.Type = "page_number"
		c.Pagination.PageParam = "p"
		return c
	}()

	scraper, err := NewWebScraper(quietLogger(), "test", "test", cfg)
	if err != nil {
		t.Fatalf("failed to create scraper: %v", err)
	}

	// Page 1 should not add params
	url := scraper.buildURL(1)
	if url != "http://example.com/list" {
		t.Errorf("page 1 should not add pagination params, got %q", url)
	}
}

func BenchmarkScraperCSSExtraction(b *testing.B) {
	html := strings.Repeat(
		`<div class="item"><h2 class="title">Test</h2><a class="link" href="/test">Link</a></div>`,
		100,
	)

	cfg := ScraperConfig{
		URL:           "http://example.com",
		SelectorType:  "css",
		ItemSelector:  ".item",
		TitleSelector: ".title",
		LinkSelector:  ".link",
	}

	scraper, _ := NewWebScraper(quietLogger(), "test", "test", cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = scraper.extractWithCSS(html)
	}
}

func BenchmarkScraperRateLimiter(b *testing.B) {
	rl := NewRateLimiter(1000.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("example.com")
	}
}
