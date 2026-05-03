package newznabserver

import (
	"encoding/xml"
	"strconv"
	"time"

	"github.com/loomctl/loom/internal/indexers"
)

// Newznab and Torznab XML namespaces. We always declare both on the
// channel element so a single feed can carry items from a mix of
// Usenet and torrent indexers — clients pick whichever attrs they
// understand.
const (
	atomNS    = "http://www.w3.org/2005/Atom"
	newznabNS = "http://www.newznab.com/DTD/2010/feeds/attributes/"
	torznabNS = "http://torznab.com/schemas/2015/feed"
)

// rssFeed is the Newznab/Torznab response envelope.
//
// The xmlns:* attributes are emitted as literal attribute names so
// clients see the prefixes they expect; Go's encoding/xml preserves
// the colon in attribute and element names verbatim, which is the
// idiomatic way to hand-render namespaced XML without a full schema
// generator.
type rssFeed struct {
	XMLName      xml.Name   `xml:"rss"`
	Version      string     `xml:"version,attr"`
	XMLNSAtom    string     `xml:"xmlns:atom,attr"`
	XMLNSNewznab string     `xml:"xmlns:newznab,attr"`
	XMLNSTorznab string     `xml:"xmlns:torznab,attr"`
	Channel      rssChannel `xml:"channel"`
}

// rssChannel is the channel element. AtomLink advertises the feed's
// own URL per RSS best practice; clients ignore it but it keeps
// validators happy.
type rssChannel struct {
	AtomLink    *atomLink `xml:"atom:link,omitempty"`
	Title       string    `xml:"title"`
	Description string    `xml:"description"`
	Link        string    `xml:"link"`
	Language    string    `xml:"language,omitempty"`
	Category    string    `xml:"category,omitempty"`
	Items       []rssItem `xml:"item"`
}

type atomLink struct {
	XMLName xml.Name `xml:"atom:link"`
	Href    string   `xml:"href,attr"`
	Rel     string   `xml:"rel,attr"`
	Type    string   `xml:"type,attr"`
}

// rssItem is one search result. Categories appear as repeated
// `<category>` elements per the Newznab spec; attrs are namespaced
// `newznab:` for usenet results and `torznab:` for torrent results.
type rssItem struct {
	XMLName     xml.Name      `xml:"item"`
	Title       string        `xml:"title"`
	GUID        guidElement   `xml:"guid"`
	Link        string        `xml:"link,omitempty"`
	Comments    string        `xml:"comments,omitempty"`
	PubDate     string        `xml:"pubDate,omitempty"`
	Categories  []string      `xml:"category,omitempty"`
	Description string        `xml:"description,omitempty"`
	Enclosure   *enclosure    `xml:"enclosure,omitempty"`
	NewznabAttr []newznabAttr `xml:",any"`
}

// guidElement keeps `isPermaLink="false"` explicit because some
// clients otherwise dereference the GUID as a URL.
type guidElement struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

// newznabAttr / torznabAttr render as `<newznab:attr name="..."
// value="..."/>` and `<torznab:attr ...>` respectively. Go preserves
// the literal local name with a colon, which is the canonical hack
// for this without a real namespace-aware encoder.
type newznabAttr struct {
	XMLName xml.Name
	Name    string `xml:"name,attr"`
	Value   string `xml:"value,attr"`
}

func makeNewznabAttr(name, value string) newznabAttr {
	return newznabAttr{XMLName: xml.Name{Local: "newznab:attr"}, Name: name, Value: value}
}

func makeTorznabAttr(name, value string) newznabAttr {
	return newznabAttr{XMLName: xml.Name{Local: "torznab:attr"}, Name: name, Value: value}
}

// feedOptions are the channel-level fields rendered onto the
// envelope. Title/Description/Link surface in some clients' indexer
// listings; SelfURL becomes the atom:link self reference.
type feedOptions struct {
	Title       string
	Description string
	Link        string
	SelfURL     string
}

// renderFeed builds an rssFeed from the aggregated results plus a
// channel-options struct. Items are rendered in the order supplied;
// callers (handler.go) sort first by pubDate desc to match the
// upstream convention.
func renderFeed(results []indexers.Result, opts feedOptions) rssFeed {
	feed := rssFeed{
		Version:      "2.0",
		XMLNSAtom:    atomNS,
		XMLNSNewznab: newznabNS,
		XMLNSTorznab: torznabNS,
		Channel: rssChannel{
			Title:       opts.Title,
			Description: opts.Description,
			Link:        opts.Link,
			Language:    "en-us",
		},
	}
	if opts.SelfURL != "" {
		feed.Channel.AtomLink = &atomLink{
			Href: opts.SelfURL,
			Rel:  "self",
			Type: "application/rss+xml",
		}
	}
	feed.Channel.Items = make([]rssItem, 0, len(results))
	for _, r := range results {
		feed.Channel.Items = append(feed.Channel.Items, renderItem(r))
	}
	return feed
}

// renderItem maps one Result onto an RSS item. The torrent vs usenet
// split is decided by infohash/magnet presence: only torrent results
// declare those, so they pick the torznab namespace; everything else
// (including indexers with no extra metadata) falls back to the
// newznab namespace, which is what Sonarr/Radarr default to anyway.
func renderItem(r indexers.Result) rssItem {
	item := rssItem{
		Title: r.Title,
		GUID: guidElement{
			IsPermaLink: "false",
			Value:       firstNonEmpty(r.GUID, r.Link),
		},
		Link:       r.Link,
		Comments:   r.InfoURL,
		PubDate:    formatPubDate(r.PubDate),
		Categories: categoriesAsStrings(r.Category),
	}
	if r.Link != "" && r.Size > 0 {
		item.Enclosure = &enclosure{
			URL:    r.Link,
			Length: r.Size,
			Type:   enclosureTypeFor(r),
		}
	}
	item.NewznabAttr = renderAttrs(r)
	return item
}

// renderAttrs picks the right namespace for the attrs and emits the
// fields Sonarr/Radarr/Lidarr actually consume. We always include
// size and category; torrent results add seeders/peers/infohash and
// optionally magneturl; quality/indexer get newznab attrs.
func renderAttrs(r indexers.Result) []newznabAttr {
	isTorrent := r.Infohash != "" || r.MagnetURI != "" || r.Seeders != nil || r.Peers != nil
	build := makeNewznabAttr
	if isTorrent {
		build = makeTorznabAttr
	}
	out := make([]newznabAttr, 0, 8)
	if r.Size > 0 {
		out = append(out, build("size", strconv.FormatInt(r.Size, 10)))
	}
	for _, c := range r.Category {
		out = append(out, build("category", strconv.Itoa(int(c))))
	}
	if r.Quality != "" {
		out = append(out, build("quality", r.Quality))
	}
	if r.IndexerID != "" {
		out = append(out, build("indexer", r.IndexerID))
	}
	if isTorrent {
		if r.Seeders != nil {
			out = append(out, build("seeders", strconv.Itoa(*r.Seeders)))
		}
		if r.Peers != nil {
			out = append(out, build("peers", strconv.Itoa(*r.Peers)))
		}
		if r.Infohash != "" {
			out = append(out, build("infohash", r.Infohash))
		}
		if r.MagnetURI != "" {
			out = append(out, build("magneturl", r.MagnetURI))
		}
	}
	return out
}

// enclosureTypeFor returns the MIME type RSS clients expect on
// <enclosure>. Torrent results advertise application/x-bittorrent;
// everything else (including indexers we can't classify) gets the
// Newznab nzb MIME, which is what Prowlarr does for unknown items.
func enclosureTypeFor(r indexers.Result) string {
	if r.Infohash != "" || r.MagnetURI != "" || r.Seeders != nil {
		return "application/x-bittorrent"
	}
	return "application/x-nzb"
}

// formatPubDate emits RFC1123Z (the Newznab spec) for a non-zero
// time. A zero time renders empty; some clients skip items without a
// pubDate but most accept it.
func formatPubDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC1123Z)
}

func categoriesAsStrings(in []indexers.Category) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, strconv.Itoa(int(c)))
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
