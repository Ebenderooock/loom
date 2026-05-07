package newznabserver

import (
	"encoding/xml"
	"sort"
	"strconv"

	"github.com/ebenderooock/loom/internal/indexers"
)

// capsXML is the wire shape returned by `t=caps`. It's a strict
// subset of the upstream Newznab spec (server / limits / searching /
// categories blocks). Sub-categories are skipped on purpose: we don't
// know upstream sub-category names well enough to reproduce them and
// the families are what every Sonarr/Radarr template keys on.
type capsXML struct {
	XMLName    xml.Name        `xml:"caps"`
	Server     capsServer      `xml:"server"`
	Limits     capsLimits      `xml:"limits"`
	Searching  capsSearching   `xml:"searching"`
	Categories capsCategoryDoc `xml:"categories"`
}

type capsServer struct {
	Version   string `xml:"version,attr"`
	Title     string `xml:"title,attr"`
	Strapline string `xml:"strapline,attr"`
	Email     string `xml:"email,attr,omitempty"`
	URL       string `xml:"url,attr,omitempty"`
}

type capsLimits struct {
	Max     int `xml:"max,attr"`
	Default int `xml:"default,attr"`
}

type capsSearching struct {
	Search   capsSearchMode `xml:"search"`
	TVSearch capsSearchMode `xml:"tv-search"`
	Movie    capsSearchMode `xml:"movie-search"`
	Audio    capsSearchMode `xml:"audio-search"`
	Book     capsSearchMode `xml:"book-search"`
}

type capsSearchMode struct {
	Available       string `xml:"available,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

type capsCategoryDoc struct {
	Categories []capsCategory `xml:"category"`
}

type capsCategory struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

// categoryFamilyNames is the canonical Newznab name for each top-level
// family. Sub-category IDs that fall through to renderCategories
// (e.g. 5040) are emitted without a known name.
var categoryFamilyNames = map[indexers.Category]string{
	indexers.CategoryConsole: "Console",
	indexers.CategoryMovies:  "Movies",
	indexers.CategoryAudio:   "Audio",
	indexers.CategoryPC:      "PC",
	indexers.CategoryTV:      "TV",
	indexers.CategoryXXX:     "XXX",
	indexers.CategoryBooks:   "Books",
	indexers.CategoryOther:   "Other",
}

// supportedParamsBase is the minimum supportedParams string we promise
// for any mode that any indexer answers. The Newznab spec mandates
// `q` for free-text search; modes that take IDs add them on top.
const supportedParamsBase = "q"

// aggregateCaps merges Caps from a list of indexers into a single
// caps document. The aggregation is lenient on purpose: a search
// mode is "available" if any indexer offers it, the category list is
// the union deduped and sorted, and supportedParams is the union of
// every mode-relevant ID scheme any indexer declared.
//
// title and strapline land in the <server> element so operators can
// tell at a glance which Loom instance they're talking to.
func aggregateCaps(all []indexers.Indexer, title, strapline string) capsXML {
	var (
		hasSearch, hasTV, hasMovie, hasAudio, hasBook bool
		idScheme                                      = map[string]bool{}
		categories                                    = map[indexers.Category]bool{}
	)
	for _, ix := range all {
		c := ix.Caps()
		for _, mode := range c.SearchTypes {
			switch mode {
			case "search":
				hasSearch = true
			case "tvsearch", "tv-search":
				hasTV = true
			case "movie", "movie-search":
				hasMovie = true
			case "music", "audio-search":
				hasAudio = true
			case "book", "book-search":
				hasBook = true
			}
		}
		for _, id := range c.SupportedIDs {
			idScheme[id] = true
		}
		for _, cat := range c.Categories {
			categories[cat] = true
		}
	}
	// If no indexers declared a search mode, default to the basic
	// `t=search` so clients can still hit `t=caps` and discover an
	// otherwise-empty server. Sonarr probes capabilities before
	// every search; an entirely empty caps doc trips its fallbacks.
	if !hasSearch && !hasTV && !hasMovie && !hasAudio && !hasBook {
		hasSearch = true
	}

	doc := capsXML{
		Server: capsServer{
			Version:   "1.0",
			Title:     title,
			Strapline: strapline,
		},
		Limits: capsLimits{Max: 100, Default: 50},
		Searching: capsSearching{
			Search:   modeFor(hasSearch, supportedParamsBase),
			TVSearch: modeFor(hasTV, paramsForTV(idScheme)),
			Movie:    modeFor(hasMovie, paramsForMovie(idScheme)),
			Audio:    modeFor(hasAudio, paramsForAudio(idScheme)),
			Book:     modeFor(hasBook, supportedParamsBase),
		},
		Categories: capsCategoryDoc{Categories: renderCategories(categories)},
	}
	return doc
}

func modeFor(available bool, params string) capsSearchMode {
	if available {
		return capsSearchMode{Available: "yes", SupportedParams: params}
	}
	return capsSearchMode{Available: "no", SupportedParams: ""}
}

// paramsForTV / paramsForMovie / paramsForAudio derive the
// supportedParams string for a single mode from the union of ID
// schemes the indexers claimed. Mode-irrelevant IDs (e.g. tvdbid on
// movie-search) are filtered out so clients don't try them.
func paramsForTV(idScheme map[string]bool) string {
	out := []string{supportedParamsBase, "season", "ep"}
	if idScheme["tvdbid"] {
		out = append(out, "tvdbid")
	}
	if idScheme["rid"] {
		out = append(out, "rid")
	}
	if idScheme["tvmazeid"] {
		out = append(out, "tvmazeid")
	}
	return joinParams(out)
}

func paramsForMovie(idScheme map[string]bool) string {
	out := []string{supportedParamsBase}
	if idScheme["imdbid"] {
		out = append(out, "imdbid")
	}
	if idScheme["tmdbid"] {
		out = append(out, "tmdbid")
	}
	return joinParams(out)
}

func paramsForAudio(idScheme map[string]bool) string {
	out := []string{supportedParamsBase, "artist", "album"}
	return joinParams(out)
}

func joinParams(in []string) string {
	if len(in) == 0 {
		return ""
	}
	out := in[0]
	for _, p := range in[1:] {
		out += "," + p
	}
	return out
}

// renderCategories sorts the deduped category set and projects each
// onto a capsCategory. Family-level IDs get the canonical name from
// categoryFamilyNames; sub-category IDs use a "Category <id>"
// placeholder so the XML is valid even when we don't know the
// upstream label.
func renderCategories(set map[indexers.Category]bool) []capsCategory {
	ids := make([]int, 0, len(set))
	for c := range set {
		ids = append(ids, int(c))
	}
	sort.Ints(ids)
	out := make([]capsCategory, 0, len(ids))
	for _, id := range ids {
		out = append(out, capsCategory{
			ID:   strconv.Itoa(id),
			Name: nameForCategory(indexers.Category(id)),
		})
	}
	return out
}

func nameForCategory(c indexers.Category) string {
	if name, ok := categoryFamilyNames[c]; ok {
		return name
	}
	return "Category " + strconv.Itoa(int(c))
}
