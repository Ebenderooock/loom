package musicbrainz

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// ReleaseGroupFull is the full release-group representation returned by the
// /release-group lookup and browse endpoints (richer than ReleaseGroupResponse).
type ReleaseGroupFull struct {
	ID               string             `json:"id"`
	Title            string             `json:"title"`
	PrimaryType      string             `json:"primary-type,omitempty"`
	SecondaryTypes   []string           `json:"secondary-types,omitempty"`
	FirstReleaseDate string             `json:"first-release-date,omitempty"`
	Disambiguation   string             `json:"disambiguation,omitempty"`
	Artists          []ArtistResponse   `json:"artist-credit,omitempty"`
	Genres           []GenreResponse    `json:"genres,omitempty"`
	Releases         []ReleaseResponse  `json:"releases,omitempty"`
	Relations        []RelationResponse `json:"relations,omitempty"`
}

// releaseGroupBrowseResponse wraps a browse-by-artist response.
type releaseGroupBrowseResponse struct {
	Count         int                `json:"release-group-count"`
	Offset        int                `json:"release-group-offset"`
	ReleaseGroups []ReleaseGroupFull `json:"release-groups"`
}

// GetArtistReleaseGroups browses the release-groups (albums) for an artist.
// Returns the page of release-groups and the total count.
func (c *Client) GetArtistReleaseGroups(ctx context.Context, artistMBID string, offset, limit int) ([]ReleaseGroupFull, int, error) {
	if limit <= 0 {
		limit = 100
	}
	params := url.Values{
		"fmt":    {"json"},
		"artist": {artistMBID},
		"inc":    {"genres"},
		"offset": {strconv.Itoa(offset)},
		"limit":  {strconv.Itoa(limit)},
	}
	endpoint := fmt.Sprintf("%s/release-group?%s", c.config.BaseURL, params.Encode())

	resp := &releaseGroupBrowseResponse{}
	if err := c.doRequest(ctx, endpoint, resp); err != nil {
		return nil, 0, err
	}
	return resp.ReleaseGroups, resp.Count, nil
}

// GetReleaseGroup fetches a single release-group (album) including its releases
// (editions). Track lists are NOT included; use GetReleaseRaw for tracks.
func (c *Client) GetReleaseGroup(ctx context.Context, mbid string) (*ReleaseGroupFull, error) {
	endpoint := fmt.Sprintf("%s/release-group/%s?fmt=json&inc=releases+artist-credits+genres",
		c.config.BaseURL, url.QueryEscape(mbid))

	resp := &ReleaseGroupFull{}
	if err := c.doRequest(ctx, endpoint, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetReleaseRaw fetches a single release (edition) with its media and tracks,
// returning the raw response so callers can map disc/track positions and
// recording IDs precisely.
func (c *Client) GetReleaseRaw(ctx context.Context, mbid string) (*ReleaseResponse, error) {
	endpoint := fmt.Sprintf("%s/release/%s?fmt=json&inc=artist-credits+recordings+labels",
		c.config.BaseURL, url.QueryEscape(mbid))

	resp := &ReleaseResponse{}
	if err := c.doRequest(ctx, endpoint, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
