package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Configured reports whether catalog requests can authenticate. It never
// exposes the configured credential.
func (c *Client) Configured() bool {
	return c != nil && strings.TrimSpace(c.config.APIKey) != ""
}

// GetMovieDetails includes explicit collection membership, genres and credits.
func (c *Client) GetMovieDetails(ctx context.Context, id int) (*MovieResponse, error) {
	var out MovieResponse
	err := c.catalog(ctx, fmt.Sprintf("/movie/%d", id), url.Values{"append_to_response": {"credits"}}, &out)
	if err == nil && (id <= 0 || out.ID != id) {
		err = NewServerError(502, "invalid movie catalog response")
	}
	return &out, err
}

// GetTVDetails returns series details with genres and credits.
func (c *Client) GetTVDetails(ctx context.Context, id int) (*TVResponse, error) {
	var out TVResponse
	err := c.catalog(ctx, fmt.Sprintf("/tv/%d", id), url.Values{"append_to_response": {"credits"}}, &out)
	if err == nil && (id <= 0 || out.ID != id) {
		err = NewServerError(502, "invalid TV catalog response")
	}
	return &out, err
}

// GetCollection returns a TMDB movie set and its members.
func (c *Client) GetCollection(ctx context.Context, id int) (*CollectionResponse, error) {
	var out CollectionResponse
	err := c.catalog(ctx, fmt.Sprintf("/collection/%d", id), nil, &out)
	if err == nil && (id <= 0 || out.ID != id) {
		err = NewServerError(502, "invalid collection catalog response")
	}
	return &out, err
}

func catalogMediaPath(mediaType string) (string, error) {
	switch mediaType {
	case "movie":
		return "movie", nil
	case "series":
		return "tv", nil
	default:
		return "", fmt.Errorf("tmdb: invalid catalog media type")
	}
}

// GetCatalogCredits returns cast and crew for a movie or series.
func (c *Client) GetCatalogCredits(ctx context.Context, mediaType string, id int) (*CreditsResponse, error) {
	path, err := catalogMediaPath(mediaType)
	if err != nil {
		return nil, err
	}
	var out CreditsResponse
	err = c.catalog(ctx, fmt.Sprintf("/%s/%d/credits", path, id), nil, &out)
	return &out, err
}

// GetRecommendations returns TMDB titles related to a movie or series.
func (c *Client) GetRecommendations(ctx context.Context, mediaType string, id int) (*SearchResponse, error) {
	path, err := catalogMediaPath(mediaType)
	if err != nil {
		return nil, err
	}
	var out SearchResponse
	err = c.catalog(ctx, fmt.Sprintf("/%s/%d/recommendations", path, id), url.Values{"page": {"1"}}, &out)
	if err == nil && out.Results == nil {
		err = NewServerError(502, "invalid recommendations catalog response")
	}
	return &out, err
}

// DiscoverOptions intentionally exposes only the bounded first-page genre
// catalog needed by library growth. Multiple genres use TMDB's AND semantics.
type DiscoverOptions struct {
	GenreIDs   []int
	ReleasedBy string // YYYY-MM-DD; empty permits future results.
}

// Discover queries TMDB discover for a media type using opts filters.
func (c *Client) Discover(ctx context.Context, mediaType string, opts DiscoverOptions) (*SearchResponse, error) {
	path, err := catalogMediaPath(mediaType)
	if err != nil {
		return nil, err
	}
	genres := make([]string, 0, len(opts.GenreIDs))
	for _, id := range opts.GenreIDs {
		if id <= 0 {
			return nil, fmt.Errorf("tmdb: invalid genre ID")
		}
		genres = append(genres, strconv.Itoa(id))
	}
	q := url.Values{
		"page": {"1"}, "include_adult": {"false"},
		"sort_by": {"popularity.desc"}, "with_genres": {strings.Join(genres, ",")},
	}
	if opts.ReleasedBy != "" {
		dateKey := "primary_release_date.lte"
		if mediaType == "series" {
			dateKey = "first_air_date.lte"
		}
		q.Set(dateKey, opts.ReleasedBy)
	}
	var out SearchResponse
	err = c.catalog(ctx, "/discover/"+path, q, &out)
	if err == nil && out.Results == nil {
		err = NewServerError(502, "invalid discovery catalog response")
	}
	return &out, err
}

func (c *Client) catalog(ctx context.Context, path string, q url.Values, out any) error {
	if !c.Configured() {
		return NewUnauthorizedError("TMDB is not configured")
	}
	endpoint := strings.TrimRight(c.config.BaseURL, "/") + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	body, err := c.doRequest(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return NewServerError(502, "invalid catalog response")
	}
	return nil
}
