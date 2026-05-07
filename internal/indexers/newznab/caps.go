package newznab

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/indexers"
)

// capsXML is the subset of the `t=caps` document we read. The full
// upstream schema covers server, limits, retention and registration
// blocks; we only project the fields Loom uses today.
type capsXML struct {
	XMLName    xml.Name           `xml:"caps"`
	Searching  capsSearching      `xml:"searching"`
	Categories capsCategoriesNode `xml:"categories"`
}

type capsSearching struct {
	Search   capsSearchMode `xml:"search"`
	TVSearch capsSearchMode `xml:"tv-search"`
	Movie    capsSearchMode `xml:"movie-search"`
	Music    capsSearchMode `xml:"audio-search"`
	Book     capsSearchMode `xml:"book-search"`
}

type capsSearchMode struct {
	Available    string `xml:"available,attr"`
	SupportedIDs string `xml:"supportedParams,attr"`
}

type capsCategoriesNode struct {
	Categories []capsCategory `xml:"category"`
}

type capsCategory struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

// parseCapsResponse maps the upstream caps XML onto the neutral
// indexers.Caps shape used by the rest of the system.
func parseCapsResponse(body []byte) (indexers.Caps, error) {
	if !looksLikeXML(body) {
		return indexers.Caps{}, fmt.Errorf("%w: not xml", ErrMalformedXML)
	}
	var c capsXML
	if err := xml.Unmarshal(body, &c); err != nil {
		return indexers.Caps{}, fmt.Errorf("%w: %s", ErrCapsParse, err.Error())
	}
	out := indexers.Caps{
		SearchTypes:  collectSearchTypes(c.Searching),
		Categories:   collectCategories(c.Categories.Categories),
		SupportedIDs: collectSupportedIDs(c.Searching),
	}
	return out, nil
}

// collectSearchTypes returns the modes flagged available="yes".
func collectSearchTypes(s capsSearching) []string {
	out := []string{}
	add := func(mode string, m capsSearchMode) {
		if strings.EqualFold(strings.TrimSpace(m.Available), "yes") {
			out = append(out, mode)
		}
	}
	add("search", s.Search)
	add("tvsearch", s.TVSearch)
	add("movie", s.Movie)
	add("music", s.Music)
	add("book", s.Book)
	return out
}

// collectCategories projects upstream IDs onto indexers.Category.
// Sub-categories pass through as raw integers; the indexers package
// already documents this for the API surface.
func collectCategories(in []capsCategory) []indexers.Category {
	out := make([]indexers.Category, 0, len(in))
	seen := make(map[indexers.Category]bool, len(in))
	for _, c := range in {
		id := indexers.Category(parseInt(c.ID))
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// collectSupportedIDs gathers the union of `supportedParams` across
// search modes, deduped. Newznab caps documents typically advertise
// imdbid/tvdbid/tmdbid here; we keep the strings as upstream emits
// them so wire-compat layers can pass them through unchanged.
func collectSupportedIDs(s capsSearching) []string {
	seen := map[string]bool{}
	out := []string{}
	push := func(raw string) {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" || p == "q" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	push(s.Search.SupportedIDs)
	push(s.TVSearch.SupportedIDs)
	push(s.Movie.SupportedIDs)
	push(s.Music.SupportedIDs)
	push(s.Book.SupportedIDs)
	return out
}

// looksLikeXML lets us reject HTML error pages early so the caller
// gets ErrMalformedXML rather than a confusing parse error.
func looksLikeXML(body []byte) bool {
	t := strings.TrimSpace(string(body))
	return strings.HasPrefix(t, "<?xml") || strings.HasPrefix(t, "<")
}
