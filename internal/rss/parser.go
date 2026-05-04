package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NewznabItem represents a Newznab/Torznab RSS item.
type NewznabItem struct {
	XMLName     xml.Name `xml:"item"`
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	PubDate     string   `xml:"pubDate"`
	GUID        string   `xml:"guid"`
	Comments    string   `xml:"comments"`
	Attributes  []struct {
		XMLName xml.Name `xml:"attr"`
		Name    string   `xml:"name,attr"`
		Value   string   `xml:"value,attr"`
	} `xml:"attr"`
}

// NewznabChannel represents the RSS channel.
type NewznabChannel struct {
	XMLName xml.Name       `xml:"channel"`
	Title   string         `xml:"title"`
	Link    string         `xml:"link"`
	Items   []NewznabItem  `xml:"item"`
}

// NewznabRSS represents the root RSS document.
type NewznabRSS struct {
	XMLName xml.Name       `xml:"rss"`
	Channel NewznabChannel `xml:"channel"`
}

// NewznabFeedSource fetches and parses Newznab-compatible RSS feeds.
type NewznabFeedSource struct {
	id              string
	name            string
	url             string
	interval        time.Duration
	httpClient      *http.Client
	logger          *slog.Logger
	apiKey          string
	lastModified    time.Time
	lastETag        string
	pageSize        int
	offset          int
}

// NewNewznabFeedSource creates a new Newznab feed source.
func NewNewznabFeedSource(id, name, url, apiKey string, interval time.Duration, logger *slog.Logger) *NewznabFeedSource {
	return &NewznabFeedSource{
		id:         id,
		name:       name,
		url:        url,
		interval:   interval,
		apiKey:     apiKey,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		pageSize:   100,
	}
}

// ID returns the source ID.
func (n *NewznabFeedSource) ID() string {
	return n.id
}

// Name returns the source name.
func (n *NewznabFeedSource) Name() string {
	return n.name
}

// RefreshInterval returns how often this source should be synced.
func (n *NewznabFeedSource) RefreshInterval() time.Duration {
	return n.interval
}

// Fetch retrieves and parses Newznab RSS items.
func (n *NewznabFeedSource) Fetch(ctx interface{}) ([]*Item, error) {
	goCtx, ok := ctx.(context.Context)
	if !ok {
		goCtx = context.Background()
	}

	req, err := http.NewRequestWithContext(goCtx, "GET", n.buildURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add conditional headers for efficient polling
	if !n.lastModified.IsZero() {
		req.Header.Set("If-Modified-Since", n.lastModified.UTC().Format(http.TimeFormat))
	}
	if n.lastETag != "" {
		req.Header.Set("If-None-Match", n.lastETag)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	// 304 Not Modified means no new items
	if resp.StatusCode == http.StatusNotModified {
		n.logger.Debug("feed not modified", slog.String("source", n.id))
		return []*Item{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: HTTP %d", resp.StatusCode)
	}

	// Update conditional headers for next request
	if etag := resp.Header.Get("ETag"); etag != "" {
		n.lastETag = etag
	}
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		if t, err := time.Parse(http.TimeFormat, lastMod); err == nil {
			n.lastModified = t
		}
	}

	// Parse RSS body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	rss := &NewznabRSS{}
	if err := xml.Unmarshal(body, rss); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	return n.parseItems(rss), nil
}

// buildURL constructs the feed URL with appropriate query parameters.
func (n *NewznabFeedSource) buildURL() string {
	u := n.url
	if !strings.Contains(u, "?") {
		u += "?"
	} else {
		u += "&"
	}

	params := fmt.Sprintf("limit=%d&offset=%d", n.pageSize, n.offset)
	if n.apiKey != "" {
		params += fmt.Sprintf("&apikey=%s", n.apiKey)
	}

	return u + params
}

// parseItems converts NewznabItem entries to normalized Item entries.
func (n *NewznabFeedSource) parseItems(rss *NewznabRSS) []*Item {
	var items []*Item

	for _, nzbItem := range rss.Channel.Items {
		item := &Item{
			Title:    nzbItem.Title,
			Link:     nzbItem.Link,
			SourceID: n.id,
			GUID:     nzbItem.GUID,
			Raw:      nzbItem.Description,
		}

		// Parse publication date (Newznab uses RFC822)
		if nzbItem.PubDate != "" {
			if t, err := time.Parse(time.RFC822, nzbItem.PubDate); err == nil {
				item.PublishedAt = t.UTC()
			}
		}

		// Extract indexer attributes if present (e.g., category, files, size)
		for _, attr := range nzbItem.Attributes {
			switch strings.ToLower(attr.Name) {
			case "category":
				// Store category in item metadata if needed
			case "files":
				// Store file count if relevant
			case "size":
				// Store size if relevant
			}
		}

		items = append(items, item)
	}

	return items
}

// GenericRSSFeedSource handles generic RSS feeds (non-Newznab).
type GenericRSSFeedSource struct {
	id              string
	name            string
	url             string
	interval        time.Duration
	httpClient      *http.Client
	logger          *slog.Logger
	lastModified    time.Time
	lastETag        string
}

// NewGenericRSSFeedSource creates a new generic RSS feed source.
func NewGenericRSSFeedSource(id, name, url string, interval time.Duration, logger *slog.Logger) *GenericRSSFeedSource {
	return &GenericRSSFeedSource{
		id:         id,
		name:       name,
		url:        url,
		interval:   interval,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ID returns the source ID.
func (g *GenericRSSFeedSource) ID() string {
	return g.id
}

// Name returns the source name.
func (g *GenericRSSFeedSource) Name() string {
	return g.name
}

// RefreshInterval returns how often this source should be synced.
func (g *GenericRSSFeedSource) RefreshInterval() time.Duration {
	return g.interval
}

// Fetch retrieves and parses generic RSS items.
func (g *GenericRSSFeedSource) Fetch(ctx interface{}) ([]*Item, error) {
	goCtx, ok := ctx.(context.Context)
	if !ok {
		goCtx = context.Background()
	}

	req, err := http.NewRequestWithContext(goCtx, "GET", g.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if !g.lastModified.IsZero() {
		req.Header.Set("If-Modified-Since", g.lastModified.UTC().Format(http.TimeFormat))
	}
	if g.lastETag != "" {
		req.Header.Set("If-None-Match", g.lastETag)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return []*Item{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: HTTP %d", resp.StatusCode)
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		g.lastETag = etag
	}
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		if t, err := time.Parse(http.TimeFormat, lastMod); err == nil {
			g.lastModified = t
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	rss := &NewznabRSS{}
	if err := xml.Unmarshal(body, rss); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	var items []*Item
	for _, nzbItem := range rss.Channel.Items {
		item := &Item{
			Title:    nzbItem.Title,
			Link:     nzbItem.Link,
			SourceID: g.id,
			GUID:     nzbItem.GUID,
			Raw:      nzbItem.Description,
		}

		if nzbItem.PubDate != "" {
			if t, err := time.Parse(time.RFC822, nzbItem.PubDate); err == nil {
				item.PublishedAt = t.UTC()
			} else if t, err := time.Parse(time.RFC3339, nzbItem.PubDate); err == nil {
				item.PublishedAt = t.UTC()
			}
		}

		items = append(items, item)
	}

	return items, nil
}
