package tvdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RemoteID is TVDB's explicitly declared cross-provider title identifier.
type RemoteID struct {
	ID         string `json:"id"`
	SourceName string `json:"sourceName"`
}

// ListRecord describes a TVDB movie/show list and its exact entities.
type ListRecord struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Overview   string     `json:"overview"`
	IsOfficial bool       `json:"isOfficial"`
	RemoteIDs  []RemoteID `json:"remoteIds"`
	Entities   []struct {
		MovieID  int `json:"movieId"`
		SeriesID int `json:"seriesId"`
		Order    int `json:"order"`
	} `json:"entities"`
}

// ExtendedTitle contains source IDs and candidate lists declared for a title.
type ExtendedTitle struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Overview  string       `json:"overview"`
	Image     string       `json:"image"`
	Year      string       `json:"year"`
	RemoteIDs []RemoteID   `json:"remoteIds"`
	Lists     []ListRecord `json:"lists"`
}

// Configured reports access configuration without exposing credentials.
func (c *Client) Configured() bool { return c != nil && c.config.APIKey != "" }

// extendedJSON shares TVDB login, token refresh and rate-limit handling. Never
// accepts arbitrary URLs or returns provider response bodies to the caller.
func (c *Client) extendedJSON(ctx context.Context, path string, out any) error {
	if !c.Configured() {
		return fmt.Errorf("TVDB credentials are not configured")
	}
	if err := c.ensureToken(ctx); err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.doRequest(ctx, http.MethodGet, c.config.BaseURL+path, nil)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			_ = resp.Body.Close()
			c.setToken("")
			if err := c.ensureToken(ctx); err != nil {
				return err
			}
			continue
		}
		body, err := readLimitedBody(resp.Body, maxTVDBLoginBodySize)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("TVDB status %d", resp.StatusCode)
		}
		var result struct {
			Status string          `json:"status"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return err
		}
		if result.Status != "success" || len(result.Data) == 0 || string(result.Data) == "null" {
			return fmt.Errorf("TVDB incomplete response")
		}
		return json.Unmarshal(result.Data, out)
	}
	return fmt.Errorf("TVDB authorization failed")
}

// GetList reads a complete mixed-media TVDB list through the shared client.
func (c *Client) GetList(ctx context.Context, id int) (*ListRecord, error) {
	var out ListRecord
	if id <= 0 {
		return nil, fmt.Errorf("invalid TVDB list ID")
	}
	err := c.extendedJSON(ctx, fmt.Sprintf("/lists/%d/extended", id), &out)
	if err == nil && (out.ID != id || out.Entities == nil) {
		err = fmt.Errorf("TVDB incomplete list response")
	}
	return &out, err
}

// GetExtendedTitle reads exact movie/show IDs and provider-declared lists.
func (c *Client) GetExtendedTitle(ctx context.Context, media string, id int) (*ExtendedTitle, error) {
	path := "series"
	if media == "movie" {
		path = "movies"
	} else if media != "series" {
		return nil, fmt.Errorf("invalid TVDB media type")
	}
	if id <= 0 {
		return nil, fmt.Errorf("invalid TVDB title ID")
	}
	var out ExtendedTitle
	err := c.extendedJSON(ctx, fmt.Sprintf("/%s/%d/extended", path, id), &out)
	if err == nil && (out.ID != id || out.Name == "") {
		err = fmt.Errorf("TVDB incomplete title response")
	}
	return &out, err
}
