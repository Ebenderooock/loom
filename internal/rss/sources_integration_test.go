package rss

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMakeFeedSourceRSS verifies that MakeFeedSource creates a GenericRSSFeedSource.
func TestMakeFeedSourceRSS(t *testing.T) {
	svc := &SourcesService{logger: setupTestLogger(t)}

	us := &UserSource{
		ID:   "test-rss-1",
		Name: "Test RSS",
		Type: SourceTypeRSS,
		Config: json.RawMessage(`{
			"url": "http://example.com/feed.xml",
			"refresh_interval_minutes": 60
		}`),
	}

	source, err := svc.MakeFeedSource(us)
	if err != nil {
		t.Fatalf("make feed source: %v", err)
	}

	if source.ID() != "test-rss-1" {
		t.Errorf("source ID: %s, want test-rss-1", source.ID())
	}
	if source.Name() != "Test RSS" {
		t.Errorf("source name: %s, want Test RSS", source.Name())
	}
	if source.RefreshInterval() != time.Hour {
		t.Errorf("refresh interval: %v, want 1h", source.RefreshInterval())
	}
}

// TestMakeFeedSourceScraper verifies that MakeFeedSource creates a WebScraper.
func TestMakeFeedSourceScraper(t *testing.T) {
	svc := &SourcesService{logger: setupTestLogger(t)}

	us := &UserSource{
		ID:   "test-scraper-1",
		Name: "Test Scraper",
		Type: SourceTypeScraper,
		Config: json.RawMessage(`{
			"url": "http://example.com/releases",
			"selector_type": "css",
			"item_selector": "div.release",
			"title_selector": "h2",
			"link_selector": "a",
			"refresh_interval_minutes": 30
		}`),
	}

	source, err := svc.MakeFeedSource(us)
	if err != nil {
		t.Fatalf("make feed source: %v", err)
	}

	if source.ID() != "test-scraper-1" {
		t.Errorf("source ID: %s, want test-scraper-1", source.ID())
	}
	if source.Name() != "Test Scraper" {
		t.Errorf("source name: %s, want Test Scraper", source.Name())
	}
	// Scrapers default to 1h refresh interval; refresh_interval_minutes in config is not yet used
	if source.RefreshInterval() != time.Hour {
		t.Errorf("refresh interval: %v, want 1h", source.RefreshInterval())
	}
}

// TestSyncManagerWithRealRSSSource verifies syncing a real RSS feed with mock HTTP.
func TestSyncManagerWithRealRSSSource(t *testing.T) {
	// Create mock RSS server
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>http://example.com</link>
    <item>
      <title>First Release</title>
      <link>http://example.com/release1</link>
      <guid>release1</guid>
      <pubDate>Mon, 04 May 2026 12:00:00 GMT</pubDate>
    </item>
    <item>
      <title>Second Release</title>
      <link>http://example.com/release2</link>
      <guid>release2</guid>
      <pubDate>Mon, 04 May 2026 11:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feed.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, rssContent)
	}))
	defer server.Close()

	// Create sources service and sync manager
	db := setupTestDB(t)
	defer db.Close()
	logger := setupTestLogger(t)
	svc := &SourcesService{logger: logger}
	storage := NewStorage(db)
	manager := NewSyncManager(storage, logger)

	// Create RSS source pointing to mock server
	us := &UserSource{
		ID:   "test-rss-source",
		Name: "Test RSS",
		Type: SourceTypeRSS,
		Config: json.RawMessage(fmt.Sprintf(`{
			"url": "%s/feed.xml",
			"refresh_interval_minutes": 60
		}`, server.URL)),
	}

	source, err := svc.MakeFeedSource(us)
	if err != nil {
		t.Fatalf("make feed source: %v", err)
	}

	manager.RegisterSource(source)

	// Sync feeds
	ctx := context.Background()
	err = manager.SyncFeeds(ctx)
	if err != nil {
		t.Fatalf("sync feeds: %v", err)
	}

	// Verify items were stored
	stats := manager.GetStats()
	if stats.ItemsStored != 2 {
		t.Errorf("items stored: %d, want 2", stats.ItemsStored)
	}

	// Retrieve items
	items, err := manager.GetRecentItems(ctx, 100)
	if err != nil {
		t.Fatalf("get items: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("items count: %d, want 2", len(items))
	}

	// Verify titles
	titles := map[string]bool{"First Release": false, "Second Release": false}
	for _, item := range items {
		if _, ok := titles[item.Title]; ok {
			titles[item.Title] = true
		}
	}
	if !titles["First Release"] || !titles["Second Release"] {
		t.Errorf("unexpected items: %v", titles)
	}

	// Sync again and verify deduplication
	err = manager.SyncFeeds(ctx)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	stats = manager.GetStats()
	if stats.ItemsDeduped != 2 {
		t.Errorf("deduped items: %d, want 2", stats.ItemsDeduped)
	}
}

// TestSyncManagerWithRealScraperSource verifies syncing with a real web scraper.
func TestSyncManagerWithRealScraperSource(t *testing.T) {
	// Create mock web server with structured content
	htmlContent := `<!DOCTYPE html>
<html>
<head><title>Releases</title></head>
<body>
  <div class="release">
    <h2>v1.0.0</h2>
    <a href="https://releases.example.com/v1.0.0">Download</a>
  </div>
  <div class="release">
    <h2>v0.9.0</h2>
    <a href="https://releases.example.com/v0.9.0">Download</a>
  </div>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, htmlContent)
	}))
	defer server.Close()

	// Create sources service and sync manager
	db := setupTestDB(t)
	defer db.Close()
	logger := setupTestLogger(t)
	svc := &SourcesService{logger: logger}
	storage := NewStorage(db)
	manager := NewSyncManager(storage, logger)

	// Create scraper source pointing to mock server
	us := &UserSource{
		ID:   "test-scraper-source",
		Name: "Test Scraper",
		Type: SourceTypeScraper,
		Config: json.RawMessage(fmt.Sprintf(`{
			"url": "%s/releases",
			"selector_type": "css",
			"item_selector": "div.release",
			"title_selector": "h2",
			"link_selector": "a",
			"refresh_interval_minutes": 30
		}`, server.URL)),
	}

	source, err := svc.MakeFeedSource(us)
	if err != nil {
		t.Fatalf("make feed source: %v", err)
	}

	manager.RegisterSource(source)

	// Sync feeds
	ctx := context.Background()
	err = manager.SyncFeeds(ctx)
	if err != nil {
		t.Fatalf("sync feeds: %v", err)
	}

	// Verify items were stored (should extract the h2 titles)
	stats := manager.GetStats()
	if stats.ItemsStored != 2 {
		t.Errorf("items stored: %d, want 2", stats.ItemsStored)
	}

	// Retrieve items
	items, err := manager.GetRecentItems(ctx, 100)
	if err != nil {
		t.Fatalf("get items: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("items count: %d, want 2", len(items))
	}

	// Verify that items contain expected text
	foundVersions := map[string]bool{}
	for _, item := range items {
		if strings.Contains(item.Title, "v1.0.0") {
			foundVersions["v1.0.0"] = true
		}
		if strings.Contains(item.Title, "v0.9.0") {
			foundVersions["v0.9.0"] = true
		}
	}
	if !foundVersions["v1.0.0"] || !foundVersions["v0.9.0"] {
		t.Errorf("did not find expected versions in items")
	}
}

// TestSourcesServiceMakeFeedSourceValidation verifies that MakeFeedSource
// validates required config fields.
func TestMakeFeedSourceValidation(t *testing.T) {
	svc := &SourcesService{logger: setupTestLogger(t)}

	tests := []struct {
		name    string
		us      *UserSource
		wantErr bool
	}{
		{
			name: "valid_rss",
			us: &UserSource{
				ID:     "test1",
				Type:   SourceTypeRSS,
				Config: json.RawMessage(`{"url": "http://example.com/feed", "refresh_interval_minutes": 60}`),
			},
			wantErr: false,
		},
		{
			name: "rss_missing_url",
			us: &UserSource{
				ID:     "test2",
				Type:   SourceTypeRSS,
				Config: json.RawMessage(`{"refresh_interval_minutes": 60}`),
			},
			wantErr: true,
		},
		{
			name: "valid_scraper",
			us: &UserSource{
				ID:     "test3",
				Type:   SourceTypeScraper,
				Config: json.RawMessage(`{"url": "http://example.com", "selector_type": "css", "item_selector": "div", "title_selector": "h2", "refresh_interval_minutes": 30}`),
			},
			wantErr: false,
		},
		{
			name: "scraper_missing_selector",
			us: &UserSource{
				ID:     "test4",
				Type:   SourceTypeScraper,
				Config: json.RawMessage(`{"url": "http://example.com", "selector_type": "css", "refresh_interval_minutes": 30}`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.MakeFeedSource(tt.us)
			if (err != nil) != tt.wantErr {
				t.Errorf("make feed source error: %v, wantErr: %v", err, tt.wantErr)
			}
		})
	}
}

// TestMakeFeedSourceInvalidType verifies that MakeFeedSource errors on unknown type.
func TestMakeFeedSourceInvalidType(t *testing.T) {
	svc := &SourcesService{logger: setupTestLogger(t)}

	us := &UserSource{
		ID:     "test",
		Type:   SourceType("unknown"),
		Config: json.RawMessage(`{"url": "http://example.com"}`),
	}

	_, err := svc.MakeFeedSource(us)
	if err == nil {
		t.Errorf("make feed source should error on unknown type")
	}
}
