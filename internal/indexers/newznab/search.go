package newznab

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/indexers"
)

// Search implements indexers.Indexer. The mode (`search`, `tvsearch`,
// `movie`) is chosen from the Query: presence of imdb/tmdb routes us
// to `movie`, presence of tvdb/season to `tvsearch`, otherwise plain
// `search`.
func (c *Client) Search(ctx context.Context, q indexers.Query) (*indexers.Results, error) {
	caps := c.Caps()
	mode, params := buildQuery(q, c.cfg, caps)
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
// When caps are known (non-empty), the mode and ID params are
// restricted to what the indexer actually supports. Empty caps are
// treated as "unknown — allow everything" to avoid regressing when
// caps fetch failed.
func buildQuery(q indexers.Query, cfg Config, caps indexers.Caps) (mode string, params url.Values) {
	params = url.Values{}
	mode = chooseMode(q, caps)
	if t := strings.TrimSpace(q.Term); t != "" {
		hasSEP := mode == "tvsearch" || mode == "movie"
		params.Set("q", indexers.SanitizeTerm(t, hasSEP))
	}
	if cats := pickCategories(q, cfg); len(cats) > 0 {
		params.Set("cat", strings.Join(cats, ","))
	}
	// Only include ID params the indexer supports (or all if caps unknown).
	if id := strings.TrimPrefix(strings.TrimSpace(q.IMDBID), "tt"); id != "" {
		if capsSupportsID(caps, "imdb") {
			params.Set("imdbid", id)
		}
	}
	if id := strings.TrimSpace(q.TVDBID); id != "" {
		if capsSupportsID(caps, "tvdb") {
			params.Set("tvdbid", id)
		}
	}
	if id := strings.TrimSpace(q.TMDBID); id != "" {
		if capsSupportsID(caps, "tmdb") {
			params.Set("tmdbid", id)
		}
	}
	if q.Season > 0 {
		params.Set("season", strconv.Itoa(q.Season))
	}
	if q.Episode > 0 {
		params.Set("ep", strconv.Itoa(q.Episode))
	}
	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
		params.Set("offset", "0")
	}
	return mode, params
}

// chooseMode reads the Query and routes to the most specific mode the
// upstream supports. When caps are known, we verify the chosen mode
// is actually supported; if not, we fall back to "search".
func chooseMode(q indexers.Query, caps indexers.Caps) string {
	preferred := inferPreferredMode(q)
	if capsSupportsMode(caps, preferred) {
		return preferred
	}
	return "search"
}

// inferPreferredMode picks the ideal mode based on Query fields,
// ignoring caps.
func inferPreferredMode(q indexers.Query) string {
	switch {
	case q.IMDBID != "" || q.TMDBID != "":
		return "movie"
	case q.TVDBID != "" || q.Season > 0 || q.Episode > 0:
		return "tvsearch"
	}

	// Infer mode from categories when no IDs are present.
	if len(q.Categories) > 0 {
		allMovie, allTV := true, true
		for _, c := range q.Categories {
			family := (int(c) / 1000) * 1000
			if family != int(indexers.CategoryMovies) {
				allMovie = false
			}
			if family != int(indexers.CategoryTV) {
				allTV = false
			}
		}
		if allMovie {
			return "movie"
		}
		if allTV {
			return "tvsearch"
		}
	}
	return "search"
}

// capsSupportsMode checks if the indexer caps allow the given mode.
// Empty caps (unknown) means "allow everything".
func capsSupportsMode(caps indexers.Caps, mode string) bool {
	if len(caps.SearchTypes) == 0 {
		return true // Unknown caps — allow all modes.
	}
	// Normalise names: Newznab caps XML uses "tv-search" / "movie-search"
	// but the API param is "tvsearch" / "movie".
	aliases := map[string][]string{
		"tvsearch": {"tvsearch", "tv-search"},
		"movie":    {"movie", "movie-search", "moviesearch"},
		"search":   {"search"},
	}
	names, ok := aliases[mode]
	if !ok {
		names = []string{mode}
	}
	for _, st := range caps.SearchTypes {
		lower := strings.ToLower(st)
		for _, n := range names {
			if lower == n {
				return true
			}
		}
	}
	return false
}

// capsSupportsID checks if the indexer caps include a given ID scheme.
// Empty SupportedIDs (unknown) means "allow all".
// idType is the short form (e.g. "tvdb") and we match against both
// the short form and the suffixed form (e.g. "tvdbid") since upstream
// caps XML uses the suffixed form in supportedParams.
func capsSupportsID(caps indexers.Caps, idType string) bool {
	if len(caps.SupportedIDs) == 0 {
		return true // Unknown — allow all.
	}
	lower := strings.ToLower(idType)
	suffixed := lower + "id"
	for _, s := range caps.SupportedIDs {
		sl := strings.ToLower(s)
		if sl == lower || sl == suffixed {
			return true
		}
	}
	return false
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
