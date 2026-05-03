package newznabserver

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/loomctl/loom/internal/indexers"
)

// fakeIndexer is a minimal indexers.Indexer used by caps and search
// tests. It records what Caps/Search were called with so assertions
// can be precise; nothing depends on the real Service or storage.
type fakeIndexer struct {
	id       string
	name     string
	caps     indexers.Caps
	results  []indexers.Result
	searched bool
	err      error
}

func (f *fakeIndexer) ID() string         { return f.id }
func (f *fakeIndexer) Name() string       { return f.name }
func (f *fakeIndexer) Caps() indexers.Caps { return f.caps }
func (f *fakeIndexer) Test(ctx context.Context) error { return f.err }
func (f *fakeIndexer) Search(ctx context.Context, q indexers.Query) (*indexers.Results, error) {
	f.searched = true
	if f.err != nil {
		return nil, f.err
	}
	return &indexers.Results{IndexerID: f.id, Items: f.results, Total: len(f.results)}, nil
}

func TestAggregateCapsUnion(t *testing.T) {
	t.Parallel()
	a := &fakeIndexer{
		id:   "a",
		name: "Alpha",
		caps: indexers.Caps{
			SearchTypes:  []string{"search", "tvsearch"},
			Categories:   []indexers.Category{indexers.CategoryTV, 5040},
			SupportedIDs: []string{"tvdbid"},
		},
	}
	b := &fakeIndexer{
		id:   "b",
		name: "Beta",
		caps: indexers.Caps{
			SearchTypes:  []string{"search", "movie"},
			Categories:   []indexers.Category{indexers.CategoryMovies, 5040},
			SupportedIDs: []string{"imdbid", "tmdbid"},
		},
	}
	doc := aggregateCaps([]indexers.Indexer{a, b}, "Loom", "Test")

	if doc.Searching.Search.Available != "yes" {
		t.Errorf("search should be available")
	}
	if doc.Searching.Movie.Available != "yes" {
		t.Errorf("movie should be available")
	}
	if doc.Searching.TVSearch.Available != "yes" {
		t.Errorf("tvsearch should be available")
	}
	if doc.Searching.Audio.Available != "no" {
		t.Errorf("audio should be no")
	}
	wantParams := "q,imdbid,tmdbid"
	if doc.Searching.Movie.SupportedParams != wantParams {
		t.Errorf("movie supportedParams: want %q got %q",
			wantParams, doc.Searching.Movie.SupportedParams)
	}
	// Categories must be deduped (5040 appears in both indexers)
	// and sorted ascending.
	gotIDs := make([]string, len(doc.Categories.Categories))
	for i, c := range doc.Categories.Categories {
		gotIDs[i] = c.ID
	}
	want := []string{"2000", "5000", "5040"}
	if strings.Join(gotIDs, ",") != strings.Join(want, ",") {
		t.Errorf("category IDs: want %v got %v", want, gotIDs)
	}
}

func TestAggregateCapsEmptyDefaultsToSearch(t *testing.T) {
	t.Parallel()
	doc := aggregateCaps(nil, "Loom", "Test")
	if doc.Searching.Search.Available != "yes" {
		t.Errorf("empty registry must still expose t=search; got %q",
			doc.Searching.Search.Available)
	}
	if len(doc.Categories.Categories) != 0 {
		t.Errorf("expected zero categories, got %d",
			len(doc.Categories.Categories))
	}
}

func TestRenderItemTorrentVsUsenet(t *testing.T) {
	t.Parallel()
	seeders := 7
	torrent := indexers.Result{
		IndexerID: "trk",
		Title:     "T",
		Link:      "https://x/dl/1",
		Size:      100,
		Infohash:  "abc",
		Seeders:   &seeders,
	}
	usenet := indexers.Result{
		IndexerID: "nzb",
		Title:     "U",
		Link:      "https://nzb/x.nzb",
		Size:      200,
	}
	titem := renderItem(torrent)
	uitem := renderItem(usenet)
	if titem.Enclosure.Type != "application/x-bittorrent" {
		t.Errorf("torrent enclosure type: %q", titem.Enclosure.Type)
	}
	if uitem.Enclosure.Type != "application/x-nzb" {
		t.Errorf("usenet enclosure type: %q", uitem.Enclosure.Type)
	}
	// Verify the namespace switch: torrent renders torznab:attr,
	// usenet renders newznab:attr. We check on the marshalled bytes
	// because XMLName comparison is fiddly.
	tb, _ := xml.Marshal(titem)
	ub, _ := xml.Marshal(uitem)
	if !strings.Contains(string(tb), "<torznab:attr") {
		t.Errorf("torrent item missing torznab attrs: %s", tb)
	}
	if !strings.Contains(string(ub), "<newznab:attr") {
		t.Errorf("usenet item missing newznab attrs: %s", ub)
	}
}

func TestFormatPubDateZero(t *testing.T) {
	t.Parallel()
	if got := formatPubDate(time.Time{}); got != "" {
		t.Errorf("zero time should render empty, got %q", got)
	}
	got := formatPubDate(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	if got != "Tue, 02 Jan 2024 03:04:05 +0000" {
		t.Errorf("format: %q", got)
	}
}

func TestParseCategoryList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []indexers.Category
	}{
		{"", nil},
		{"all", nil},
		{"2000", []indexers.Category{2000}},
		{"2000,5040", []indexers.Category{2000, 5040}},
		{" 2000 , 5040 ", []indexers.Category{2000, 5040}},
		{"2000,abc,5040", []indexers.Category{2000, 5040}},
	}
	for _, c := range cases {
		got := parseCategoryList(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parse %q: want %v got %v", c.in, c.want, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parse %q: want %v got %v", c.in, c.want, got)
			}
		}
	}
}
