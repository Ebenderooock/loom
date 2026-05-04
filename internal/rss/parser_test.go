package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewznabFeedSource tests parsing Newznab RSS feeds.
func TestNewznabFeedSource(t *testing.T) {
	// Mock Newznab feed
	newznabXML := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
		<channel>
			<title>Test Indexer</title>
			<link>http://example.com</link>
			<item>
				<title>Test.Movie.2024.1080p.BluRay.x264</title>
				<link>http://example.com/download/1</link>
				<guid>123e4567-e89b-12d3-a456-426614174000</guid>
				<pubDate>Mon, 06 Jan 2024 15:04:05 -0700</pubDate>
				<description>A test release</description>
				<attr name="category" value="2000" />
				<attr name="size" value="5368709120" />
			</item>
			<item>
				<title>Test.Series.S01E01.1080p.WebDL.x264</title>
				<link>http://example.com/download/2</link>
				<guid>223e4567-e89b-12d3-a456-426614174001</guid>
				<pubDate>Tue, 07 Jan 2024 15:04:05 -0700</pubDate>
				<description>Another test release</description>
			</item>
		</channel>
	</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(newznabXML))
	}))
	defer server.Close()

	logger := setupTestLogger(t)
	source := NewNewznabFeedSource("test-indexer", "Test Indexer", server.URL+"?t=search&q=test", "", 1*time.Hour, logger)

	ctx := context.Background()
	items, err := source.Fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("items: %d, want 2", len(items))
	}

	// Check first item
	if items[0].Title != "Test.Movie.2024.1080p.BluRay.x264" {
		t.Errorf("title: %s", items[0].Title)
	}
	if items[0].SourceID != "test-indexer" {
		t.Errorf("source_id: %s", items[0].SourceID)
	}
	if items[0].GUID != "123e4567-e89b-12d3-a456-426614174000" {
		t.Errorf("guid: %s", items[0].GUID)
	}

	// Check second item
	if items[1].Title != "Test.Series.S01E01.1080p.WebDL.x264" {
		t.Errorf("title: %s", items[1].Title)
	}
}

// TestNewznabFeedSourceNotModified tests 304 Not Modified responses.
func TestNewznabFeedSourceNotModified(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Header().Set("ETag", "test-etag")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0"?>
			<rss version="2.0">
				<channel>
					<item>
						<title>Test</title>
						<link>http://example.com</link>
						<guid>guid-1</guid>
						<pubDate>Mon, 06 Jan 2024 15:04:05 -0700</pubDate>
					</item>
				</channel>
			</rss>`))
		} else {
			// Second request should get 304
			if r.Header.Get("If-None-Match") == "test-etag" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	logger := setupTestLogger(t)
	source := NewNewznabFeedSource("test", "Test", server.URL, "", 1*time.Hour, logger)

	ctx := context.Background()

	// First fetch
	items, err := source.Fetch(ctx)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("first fetch items: %d, want 1", len(items))
	}

	// Second fetch (should get 304)
	items, err = source.Fetch(ctx)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("second fetch items: %d, want 0 (304 not modified)", len(items))
	}
}

// TestNewznabFeedSourceWithAPIKey tests URL construction with API key.
func TestNewznabFeedSourceWithAPIKey(t *testing.T) {
	requestURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURL = r.RequestURI
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?>
		<rss version="2.0"><channel></channel></rss>`))
	}))
	defer server.Close()

	logger := setupTestLogger(t)
	source := NewNewznabFeedSource("test", "Test", server.URL+"?t=search", "secret-api-key", 1*time.Hour, logger)

	ctx := context.Background()
	source.Fetch(ctx)

	if requestURL == "" {
		t.Fatal("no request made")
	}

	if !contains(requestURL, "apikey=secret-api-key") {
		t.Errorf("API key not in URL: %s", requestURL)
	}
}

// TestGenericRSSFeedSource tests parsing generic RSS feeds.
func TestGenericRSSFeedSource(t *testing.T) {
	genericXML := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
		<channel>
			<title>Test RSS</title>
			<link>http://example.com</link>
			<item>
				<title>Release 1</title>
				<link>http://example.com/1</link>
				<guid>guid-1</guid>
				<pubDate>2024-01-06T15:04:05Z</pubDate>
				<description>Test item 1</description>
			</item>
		</channel>
	</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(genericXML))
	}))
	defer server.Close()

	logger := setupTestLogger(t)
	source := NewGenericRSSFeedSource("test-rss", "Test RSS", server.URL, 1*time.Hour, logger)

	ctx := context.Background()
	items, err := source.Fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("items: %d, want 1", len(items))
	}

	if items[0].Title != "Release 1" {
		t.Errorf("title: %s", items[0].Title)
	}
}

// TestFetchHTTPError tests handling of HTTP errors.
func TestFetchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := setupTestLogger(t)
	source := NewNewznabFeedSource("test", "Test", server.URL, "", 1*time.Hour, logger)

	ctx := context.Background()
	_, err := source.Fetch(ctx)
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
