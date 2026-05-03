package newznab

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/loomctl/loom/internal/indexers"
)

// Search implements indexers.Indexer. The mode (`search`, `tvsearch`,
// `movie`) is chosen from the Query: presence of imdb/tmdb routes us
// to `movie`, presence of tvdb/season to `tvsearch`, otherwise plain
// `search`.
func (c *Client) Search(ctx context.Context, q indexers.Query) (*indexers.Results, error) {
	mode, params := buildQuery(q, c.cfg)
	body, err := c.get(ctx, c.buildURL(mode, params))
	if err != nil {
		return nil, err
	}
	res, err := parseSearchResponse(body, c.id, c.cfg.attrFlavour)
	if err != nil {
		return nil, fmt.Errorf("newznab search %q: %w", c.cfg.URL, err)
	}
	return res, nil
}

// buildQuery picks the search mode and packs Query into URL values.
// Pure function so search_test.go can assert the wire shape.
func buildQuery(q indexers.Query, cfg Config) (mode string, params url.Values) {
	params = url.Values{}
	mode = chooseMode(q)
	if t := strings.TrimSpace(q.Term); t != "" {
		params.Set("q", t)
	}
	if cats := pickCategories(q, cfg); len(cats) > 0 {
		params.Set("cat", strings.Join(cats, ","))
	}
	if id := strings.TrimPrefix(strings.TrimSpace(q.IMDBID), "tt"); id != "" {
		params.Set("imdbid", id)
	}
	if id := strings.TrimSpace(q.TVDBID); id != "" {
		params.Set("tvdbid", id)
	}
	if id := strings.TrimSpace(q.TMDBID); id != "" {
		params.Set("tmdbid", id)
	}
	if q.Season > 0 {
		params.Set("season", strconv.Itoa(q.Season))
	}
	if q.Episode > 0 {
		params.Set("ep", strconv.Itoa(q.Episode))
	}
	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
		// Pagination semantics: Newznab uses offset+limit. We don't
		// expose offset on Query yet (Phase 2c does single-page
		// fan-out), but the param shape is wired so a later phase
		// only needs to add the field.
		params.Set("offset", "0")
	}
	return mode, params
}

// chooseMode reads the Query and routes to the most specific mode the
// upstream supports. Some indexers reject `tvsearch` without tvdbid;
// to avoid surprising operators we only switch away from plain
// `search` when an explicit ID/season is present.
func chooseMode(q indexers.Query) string {
	switch {
	case q.IMDBID != "" || q.TMDBID != "":
		return "movie"
	case q.TVDBID != "" || q.Season > 0 || q.Episode > 0:
		return "tvsearch"
	default:
		return "search"
	}
}

// pickCategories blends Query.Categories with cfg.CategoryMap. The map
// is opt-in: when present, an alias key in q.Categories (treated as
// raw int alias-or-id) gets expanded into upstream sub-category IDs.
// Today we keep it simple and just stringify the integers.
func pickCategories(q indexers.Query, _ Config) []string {
	if len(q.Categories) == 0 {
		return nil
	}
	out := make([]string, 0, len(q.Categories))
	for _, c := range q.Categories {
		out = append(out, strconv.Itoa(int(c)))
	}
	return out
}
