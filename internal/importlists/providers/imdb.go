package providers

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IMDbProvider parses IMDb list/watchlist RSS exports.
type IMDbProvider struct {
	client *http.Client
}

// NewIMDbList returns a provider for IMDb list RSS feeds.
func NewIMDbList() *IMDbProvider {
	return &IMDbProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

// NewIMDbWatchlist returns a provider for IMDb watchlist RSS feeds.
func NewIMDbWatchlist() *IMDbProvider {
	return &IMDbProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *IMDbProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("imdb: URL required")
	}

	url := cfg.URL
	if !strings.Contains(url, "export") && !strings.HasSuffix(url, ".rss") {
		// Try to construct RSS URL from list URL
		if strings.Contains(url, "/list/") {
			parts := strings.Split(url, "/list/")
			if len(parts) == 2 {
				listID := strings.Trim(parts[1], "/")
				url = fmt.Sprintf("https://rss.imdb.com/list/%s", listID)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("imdb: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imdb: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imdb: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("imdb: read body: %w", err)
	}

	var rss imdbRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("imdb: parse RSS: %w", err)
	}

	yearRe := regexp.MustCompile(`\((\d{4})\)`)
	imdbIDRe := regexp.MustCompile(`tt\d+`)

	var items []Item
	for _, entry := range rss.Channel.Items {
		title := entry.Title
		year := 0
		if m := yearRe.FindStringSubmatch(title); len(m) == 2 {
			year, _ = strconv.Atoi(m[1])
			title = strings.TrimSpace(yearRe.ReplaceAllString(title, ""))
		}
		imdbID := ""
		if m := imdbIDRe.FindString(entry.Link); m != "" {
			imdbID = m
		}

		items = append(items, Item{
			ExternalID: imdbID,
			Title:      title,
			Year:       year,
			IMDbID:     imdbID,
		})
	}
	return items, nil
}

type imdbRSS struct {
	XMLName xml.Name       `xml:"rss"`
	Channel imdbRSSChannel `xml:"channel"`
}

type imdbRSSChannel struct {
	Items []imdbRSSItem `xml:"item"`
}

type imdbRSSItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}
