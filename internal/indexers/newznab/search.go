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
// `movie`) is chosen from the Query: explicit Mode takes precedence,
// then presence of imdb/tmdb routes to `movie`, tvdb/season to
// `tvsearch`, otherwise plain `search`.
//
// Returns nil results (not an error) when the query requires a mode or
// ID scheme the indexer doesn't support — the caller should skip to
// the next query in the chain rather than marking the indexer as failed.
func (c *Client) Search(ctx context.Context, q indexers.Query) (*indexers.Results, error) {
	caps := c.capsWithContext(ctx)
	mode, params, ok := buildQuery(q, c.cfg, caps)
	if !ok {
		// Indexer can't service this query — return empty, not an error.
		return &indexers.Results{IndexerID: c.id}, nil
	}
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
//
// The third return value (ok) is false when the indexer cannot service
// the query — e.g. the required mode isn't in caps, or a required ID
// scheme isn't supported. The caller should skip this query rather
// than treating it as an error.
//
// Key behaviors ported from the Arr stack:
//   - Explicit Mode on Query takes precedence over inference.
//   - ID-based queries (TVDB/IMDB/TMDB) are only sent if the indexer
//     supports the corresponding ID scheme; if none of the query's IDs
//     are supported AND the query has no text term, ok=false.
//   - The q= parameter uses the RAW title (not sanitized/normalized).
//     url.Values handles URL-encoding.
//   - For movie text fallback, Year is appended to the term.
//   - For daily shows, DailyDate is encoded as season=YYYY&ep=MM/DD.
func buildQuery(q indexers.Query, cfg Config, caps indexers.Caps) (mode string, params url.Values, ok bool) {
	params = url.Values{}
	mode = resolveMode(q, caps)

	// If the resolved mode isn't supported by caps, bail.
	if !capsSupportsMode(caps, mode) {
		return "", nil, false
	}

	// Build the q= term. Use the raw title — do NOT sanitize.
	term := strings.TrimSpace(q.Term)
	if term != "" {
		qVal := term
		// Radarr appends year to movie text queries.
		if mode == "search" && q.Year > 0 {
			qVal = term + " " + strconv.Itoa(q.Year)
		}
		params.Set("q", qVal)
	}

	if cats := pickCategories(q, cfg); len(cats) > 0 {
		params.Set("cat", strings.Join(cats, ","))
	}

	// External IDs — only include what the indexer supports.
	hasAnyID := false
	if id := strings.TrimPrefix(strings.TrimSpace(q.IMDBID), "tt"); id != "" {
		if capsSupportsID(caps, "imdb") {
			params.Set("imdbid", id)
			hasAnyID = true
		}
	}
	if id := strings.TrimSpace(q.TVDBID); id != "" {
		if capsSupportsID(caps, "tvdb") {
			params.Set("tvdbid", id)
			hasAnyID = true
		}
	}
	if id := strings.TrimSpace(q.TMDBID); id != "" {
		if capsSupportsID(caps, "tmdb") {
			params.Set("tmdbid", id)
			hasAnyID = true
		}
	}

	// If this was an ID-based query (tvsearch/movie with IDs) but no
	// IDs were actually supported, and there's no text term either,
	// the query is unsupported — skip it.
	if !hasAnyID && term == "" && (mode == "tvsearch" || mode == "movie") {
		return "", nil, false
	}

	// Season/Episode encoding.
	if q.DailyDate != "" && len(q.DailyDate) == 10 {
		// Daily show: season=YYYY, ep=MM/DD (Sonarr convention).
		params.Set("season", q.DailyDate[:4])
		params.Set("ep", q.DailyDate[5:7]+"/"+q.DailyDate[8:10])
	} else {
		if q.Season > 0 {
			// Sonarr: season 0 → "00" to work around NNTMux.
			if q.Season == 0 {
				params.Set("season", "00")
			} else {
				params.Set("season", strconv.Itoa(q.Season))
			}
		}
		if q.Episode > 0 {
			params.Set("ep", strconv.Itoa(q.Episode))
		}
	}

	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
		params.Set("offset", "0")
	}

	return mode, params, true
}

// resolveMode determines the Newznab API mode for a query.
// Explicit Mode takes precedence; otherwise mode is inferred.
func resolveMode(q indexers.Query, caps indexers.Caps) string {
	// Explicit mode always wins.
	if q.Mode != "" {
		return string(q.Mode)
	}
	return inferPreferredMode(q, caps)
}

// inferPreferredMode picks the ideal mode based on Query fields,
// respecting caps. Unlike the previous chooseMode, this does NOT
// silently fall back to "search" when the preferred mode is
// unsupported — the caller is responsible for handling that.
func inferPreferredMode(q indexers.Query, caps indexers.Caps) string {
	switch {
	case q.IMDBID != "" || q.TMDBID != "":
		return "movie"
	case q.TVDBID != "" || q.Season > 0 || q.Episode > 0 || q.DailyDate != "":
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
