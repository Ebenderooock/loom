// Package tvmaze provides optional actual broadcast times from TVmaze.
package tvmaze

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Episode contains the identity and schedule fields needed for safe enrichment.
type Episode struct {
	Season   int    `json:"season"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	AirDate  string `json:"airdate"`
	AirTime  string `json:"airtime"`
	AirStamp string `json:"airstamp"`
}

// Client reads optional broadcast timestamps from the public TVmaze API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a client with bounded request timeouts.
func NewClient() *Client {
	return &Client{baseURL: "https://api.tvmaze.com", http: &http.Client{Timeout: 10 * time.Second}}
}

// Episodes performs two API operations (external-ID lookup, which may redirect,
// and episode list) under a single deadline. It never searches by title.
func (c *Client) Episodes(ctx context.Context, tvdbID, imdbID string) ([]Episode, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	q := url.Values{}
	if id, err := strconv.Atoi(tvdbID); err == nil && id > 0 {
		q.Set("thetvdb", tvdbID)
	} else if imdbID != "" {
		q.Set("imdb", imdbID)
	} else {
		return nil, nil
	}
	var show struct {
		ID int `json:"id"`
	}
	found, err := c.get(ctx, "/lookup/shows?"+q.Encode(), &show)
	if err != nil || !found {
		return nil, err
	}
	if show.ID <= 0 {
		return nil, fmt.Errorf("tvmaze: invalid show ID")
	}
	var episodes []Episode
	_, err = c.get(ctx, fmt.Sprintf("/shows/%d/episodes", show.ID), &episodes)
	return episodes, err
}

func (c *Client) get(ctx context.Context, path string, dest any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "Loom/episode-airing-times")
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("tvmaze: HTTP %d", resp.StatusCode)
	}
	// Bound even unusually large shows. No retry loop on 429: next refresh
	// retries enrichment without blocking the primary metadata provider.
	err = json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(dest)
	return err == nil, err
}
