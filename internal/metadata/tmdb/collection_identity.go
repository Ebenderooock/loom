package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// FindTitleID resolves only exact external identifiers, never names. TVDB
// movie identifiers are deliberately unsupported by TMDB's find endpoint.
func (c *Client) FindTitleID(ctx context.Context, media, provider, id string) (int, error) {
	if provider != "imdb" && (provider != "tvdb" || media != "series") {
		return 0, fmt.Errorf("unsupported external identity lookup")
	}
	path := "movie_results"
	if media == "series" {
		path = "tv_results"
	} else if media != "movie" {
		return 0, fmt.Errorf("invalid media type")
	}
	var result map[string][]struct {
		ID int `json:"id"`
	}
	endpoint := c.config.BaseURL + "/find/" + url.PathEscape(id) + "?" + url.Values{"external_source": {provider + "_id"}}.Encode()
	body, err := c.doRequest(ctx, endpoint)
	if err != nil {
		return 0, err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}
	matches := result[path]
	if _, ok := result[path]; !ok {
		return 0, fmt.Errorf("incomplete external identity response")
	}
	if len(matches) == 0 {
		return 0, nil
	}
	if len(matches) != 1 || matches[0].ID <= 0 {
		return 0, fmt.Errorf("ambiguous external identity")
	}
	return matches[0].ID, nil
}
