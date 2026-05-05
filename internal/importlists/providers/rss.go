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

// RSSProvider parses generic RSS/Atom feeds for media items.
type RSSProvider struct {
	client *http.Client
}

// NewRSS returns a generic RSS feed provider.
func NewRSS() *RSSProvider {
	return &RSSProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *RSSProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("rss: URL required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("rss: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rss: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("rss: read body: %w", err)
	}

	// Try RSS first, then Atom
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		return parseRSSItems(rss.Channel.Items), nil
	}

	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err == nil && len(atom.Entries) > 0 {
		return parseAtomItems(atom.Entries), nil
	}

	return nil, fmt.Errorf("rss: could not parse feed as RSS or Atom")
}

func parseRSSItems(entries []rssItem) []Item {
	yearRe := regexp.MustCompile(`\((\d{4})\)`)
	imdbRe := regexp.MustCompile(`tt\d+`)

	var items []Item
	for _, e := range entries {
		title := e.Title
		year := 0
		if m := yearRe.FindStringSubmatch(title); len(m) == 2 {
			year, _ = strconv.Atoi(m[1])
			title = strings.TrimSpace(yearRe.ReplaceAllString(title, ""))
		}
		imdbID := ""
		if m := imdbRe.FindString(e.Link); m != "" {
			imdbID = m
		}

		items = append(items, Item{
			ExternalID: e.GUID,
			Title:      title,
			Year:       year,
			IMDbID:     imdbID,
		})
	}
	return items
}

func parseAtomItems(entries []atomEntry) []Item {
	yearRe := regexp.MustCompile(`\((\d{4})\)`)
	var items []Item
	for _, e := range entries {
		title := e.Title
		year := 0
		if m := yearRe.FindStringSubmatch(title); len(m) == 2 {
			year, _ = strconv.Atoi(m[1])
			title = strings.TrimSpace(yearRe.ReplaceAllString(title, ""))
		}
		items = append(items, Item{
			ExternalID: e.ID,
			Title:      title,
			Year:       year,
		})
	}
	return items
}

type rssFeed struct {
	XMLName xml.Name      `xml:"rss"`
	Channel rssChannelDef `xml:"channel"`
}

type rssChannelDef struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
	GUID  string `xml:"guid"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID    string `xml:"id"`
	Title string `xml:"title"`
}
