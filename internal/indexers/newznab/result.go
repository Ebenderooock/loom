package newznab

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

// rssEnvelope is the Newznab/Torznab response shell. Items live under
// channel/item; both attribute namespaces unmarshal into the same
// Attr slice so search.go doesn't need to branch on flavour at parse
// time.
type rssEnvelope struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
	// Some indexers (e.g. NZBHydra2) decorate the channel with a
	// totalItems hint via newznab:response. We capture both forms.
	Response       responseMeta `xml:"response"`
	TotalAvailable string       `xml:"totalAvailable"`
}

// responseMeta carries the `<newznab:response offset="0" total="42"/>`
// hint when present.
type responseMeta struct {
	Offset string `xml:"offset,attr"`
	Total  string `xml:"total,attr"`
}

type rssItem struct {
	Title       string    `xml:"title"`
	GUID        string    `xml:"guid"`
	Link        string    `xml:"link"`
	Comments    string    `xml:"comments"`
	PubDate     string    `xml:"pubDate"`
	Description string    `xml:"description"`
	Categories  []string  `xml:"category"`
	Enclosure   enclosure `xml:"enclosure"`
	// Both xmlns:newznab and xmlns:torznab declare an "attr" element
	// in their namespace. Go's encoding/xml matches by local name
	// when no Space tag is specified, so the same field captures
	// either flavour.
	Attrs []rssAttr `xml:"attr"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type rssAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// parseSearchResponse parses the upstream XML and projects rows onto
// indexers.Result. indexerID and flavour are stamped into each row so
// downstream consumers can branch without re-reading config.
func parseSearchResponse(body []byte, indexerID string, flavour attrFlavour) (*indexers.Results, error) {
	if !looksLikeXML(body) {
		return nil, fmt.Errorf("%w: not xml", ErrMalformedXML)
	}
	var env rssEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMalformedXML, err.Error())
	}
	items := make([]indexers.Result, 0, len(env.Channel.Items))
	for _, it := range env.Channel.Items {
		items = append(items, mapItem(it, indexerID, flavour))
	}
	sortByPubDateDesc(items)
	total := parseInt(env.Channel.Response.Total)
	if total == 0 {
		total = parseInt(env.Channel.TotalAvailable)
	}
	if total == 0 {
		total = len(items)
	}
	return &indexers.Results{
		IndexerID: indexerID,
		Items:     items,
		Total:     total,
	}, nil
}

// mapItem builds a single Result from one RSS item, choosing the
// attribute mapper that matches the upstream flavour.
func mapItem(it rssItem, indexerID string, flavour attrFlavour) indexers.Result {
	r := indexers.Result{
		IndexerID: indexerID,
		Title:     strings.TrimSpace(it.Title),
		GUID:      strings.TrimSpace(it.GUID),
		Link:      pickLink(it),
		InfoURL:   strings.TrimSpace(it.Comments),
		PubDate:   parseRFC1123Z(it.PubDate),
		Size:      parseInt64(it.Enclosure.Length),
		Category:  mapCategoryStrings(it.Categories),
	}
	if flavour == flavourTorznab {
		mapTorznabAttrs(it.Attrs, &r)
	} else {
		mapNewznabAttrs(it.Attrs, &r)
	}
	return r
}

// pickLink prefers <enclosure url=...> when present (Newznab uses it
// for the .nzb download link); otherwise falls back to <link>.
func pickLink(it rssItem) string {
	if u := strings.TrimSpace(it.Enclosure.URL); u != "" {
		return u
	}
	return strings.TrimSpace(it.Link)
}

// mapNewznabAttrs surfaces the Newznab-only fields we care about
// today: size (when not in enclosure), grabs, files, group.
func mapNewznabAttrs(attrs []rssAttr, r *indexers.Result) {
	for _, a := range attrs {
		switch strings.ToLower(a.Name) {
		case "size":
			if r.Size == 0 {
				r.Size = parseInt64(a.Value)
			}
		case "category":
			if id := parseInt(a.Value); id != 0 {
				r.Category = appendUnique(r.Category, indexers.Category(id))
			}
		case "guid":
			if r.GUID == "" {
				r.GUID = a.Value
			}
		case "comments":
			if r.InfoURL == "" {
				r.InfoURL = a.Value
			}
		case "quality":
			r.Quality = a.Value
		}
	}
}

// mapTorznabAttrs surfaces seeders, peers, infohash, and magneturl.
// The torznab spec lets trackers report either `peers` directly or
// only `leechers`; we reconcile both shapes so callers see a
// consistent Peers value. Seeders/Peers are stored as *int so a
// genuine "0 seeders right now" is distinguishable from "indexer did
// not report this attribute at all".
func mapTorznabAttrs(attrs []rssAttr, r *indexers.Result) {
	var (
		seeders, leechers     int
		seedersSeen, peersSet bool
	)
	for _, a := range attrs {
		switch strings.ToLower(a.Name) {
		case "size":
			if r.Size == 0 {
				r.Size = parseInt64(a.Value)
			}
		case "seeders":
			seeders = parseInt(a.Value)
			seedersSeen = true
			r.Seeders = intPtr(seeders)
		case "peers":
			r.Peers = intPtr(parseInt(a.Value))
			peersSet = true
		case "leechers":
			leechers = parseInt(a.Value)
		case "infohash":
			r.Infohash = strings.TrimSpace(a.Value)
		case "magneturl":
			r.MagnetURI = strings.TrimSpace(a.Value)
		case "category":
			if id := parseInt(a.Value); id != 0 {
				r.Category = appendUnique(r.Category, indexers.Category(id))
			}
		}
	}
	// When the upstream only reported `leechers`, synthesise Peers
	// as seeders+leechers per the torznab convention. We only do
	// this if we actually saw a seeders attribute, so we never
	// fabricate a Peers value out of thin air for indexers that
	// omit both fields.
	if !peersSet && seedersSeen && leechers > 0 {
		r.Peers = intPtr(seeders + leechers)
	}
}

// intPtr is a small helper for setting *int fields on Result without
// scattering literal `&v` expressions everywhere.
func intPtr(v int) *int { return &v }

// mapCategoryStrings turns the "<category>5040</category>" list into
// typed Category IDs, ignoring non-numeric labels (some trackers emit
// human-readable names there).
func mapCategoryStrings(in []string) []indexers.Category {
	out := make([]indexers.Category, 0, len(in))
	seen := map[indexers.Category]bool{}
	for _, s := range in {
		if id := parseInt(s); id != 0 && !seen[indexers.Category(id)] {
			seen[indexers.Category(id)] = true
			out = append(out, indexers.Category(id))
		}
	}
	return out
}

func appendUnique(in []indexers.Category, c indexers.Category) []indexers.Category {
	for _, existing := range in {
		if existing == c {
			return in
		}
	}
	return append(in, c)
}

// parseRFC1123Z accepts both RFC1123Z (RFC2822 with numeric zone, the
// Newznab spec) and RFC1123 (textual zone, used by some indexers).
func parseRFC1123Z(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// sortByPubDateDesc puts newest entries first; equal-pub-date items
// keep upstream order.
func sortByPubDateDesc(rows []indexers.Result) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].PubDate.After(rows[j].PubDate)
	})
}
