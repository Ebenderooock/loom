package newznab

import (
	"net/url"
	"strings"
	"testing"

	"github.com/loomctl/loom/internal/indexers"
)

func TestParseSearchResponse_NewznabAttrs(t *testing.T) {
	t.Parallel()
	got, err := parseSearchResponse(loadFixture(t, "tvsearch.xml"), "ix-1", flavourNewznab)
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	if got.IndexerID != "ix-1" {
		t.Errorf("IndexerID = %q, want ix-1", got.IndexerID)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(got.Items))
	}
	if got.Total != 2 {
		t.Errorf("Total = %d, want 2", got.Total)
	}
	first := got.Items[0]
	if !strings.Contains(first.Title, "S02E03") {
		t.Errorf("first.Title = %q (sort by date desc broken?)", first.Title)
	}
	if first.Size != 2147483648 {
		t.Errorf("first.Size = %d", first.Size)
	}
	if !strings.Contains(first.Link, "abc123") {
		t.Errorf("first.Link = %q", first.Link)
	}
	if first.Quality != "1080p" {
		t.Errorf("first.Quality = %q", first.Quality)
	}
	if first.PubDate.IsZero() {
		t.Errorf("first.PubDate not parsed")
	}
	if len(first.Category) == 0 || first.Category[0] != indexers.Category(5040) {
		t.Errorf("first.Category = %v", first.Category)
	}
}

func TestParseSearchResponse_TorznabSeedersInfohash(t *testing.T) {
	t.Parallel()
	got, err := parseSearchResponse(loadFixture(t, "torznab_search.xml"), "tx-1", flavourTorznab)
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(got.Items))
	}
	first := got.Items[0]
	if first.Seeders == nil || *first.Seeders != 120 {
		t.Errorf("first.Seeders = %v, want *120", first.Seeders)
	}
	if first.Peers == nil || *first.Peers != 125 {
		t.Errorf("first.Peers = %v, want *125", first.Peers)
	}
	if first.Infohash != "DEADBEEFCAFE0000DEADBEEFCAFE0000DEADBEEF" {
		t.Errorf("first.Infohash = %q", first.Infohash)
	}
	if !strings.HasPrefix(first.MagnetURI, "magnet:?xt=urn:btih:DEADBEEF") {
		t.Errorf("first.MagnetURI = %q", first.MagnetURI)
	}
	if first.Quality != "" {
		t.Errorf("first.Quality = %q, want empty (no longer carries infohash)", first.Quality)
	}
	if first.Size != 3145728000 {
		t.Errorf("first.Size = %d", first.Size)
	}

	second := got.Items[1]
	// seeders=0 + leechers=3 → peers should fall back to 0+3.
	if second.Peers == nil || *second.Peers != 3 {
		t.Errorf("second.Peers = %v, want *3 (leecher fallback)", second.Peers)
	}
	if second.Seeders == nil || *second.Seeders != 0 {
		t.Errorf("second.Seeders = %v, want *0", second.Seeders)
	}
	if second.Infohash != "0000FEEDFACE0000FEEDFACE0000FEEDFACE0000" {
		t.Errorf("second.Infohash = %q", second.Infohash)
	}
	if second.MagnetURI != "" {
		t.Errorf("second.MagnetURI = %q, want empty", second.MagnetURI)
	}
}

func TestParseSearchResponse_NewznabLeavesTorrentFieldsZero(t *testing.T) {
	t.Parallel()
	got, err := parseSearchResponse(loadFixture(t, "tvsearch.xml"), "ix-1", flavourNewznab)
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	for i, item := range got.Items {
		if item.Seeders != nil {
			t.Errorf("items[%d].Seeders = %v, want nil for usenet", i, item.Seeders)
		}
		if item.Peers != nil {
			t.Errorf("items[%d].Peers = %v, want nil for usenet", i, item.Peers)
		}
		if item.Infohash != "" {
			t.Errorf("items[%d].Infohash = %q, want empty for usenet", i, item.Infohash)
		}
		if item.MagnetURI != "" {
			t.Errorf("items[%d].MagnetURI = %q, want empty for usenet", i, item.MagnetURI)
		}
	}
}

func TestParseSearchResponse_Empty(t *testing.T) {
	t.Parallel()
	got, err := parseSearchResponse(loadFixture(t, "empty.xml"), "ix-empty", flavourNewznab)
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	if len(got.Items) != 0 {
		t.Errorf("Items = %d, want 0", len(got.Items))
	}
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0", got.Total)
	}
}

func TestParseSearchResponse_MalformedXML(t *testing.T) {
	t.Parallel()
	_, err := parseSearchResponse([]byte("not xml at all"), "x", flavourNewznab)
	if err == nil {
		t.Fatal("expected ErrMalformedXML")
	}
}

func TestBuildQuery_Pagination(t *testing.T) {
	t.Parallel()
	q := indexers.Query{Term: "ubuntu", Limit: 25, Categories: []indexers.Category{4000}}
	mode, params := buildQuery(q, Config{})
	if mode != "search" {
		t.Errorf("mode = %q, want search", mode)
	}
	if params.Get("q") != "ubuntu" {
		t.Errorf("q = %q", params.Get("q"))
	}
	if params.Get("limit") != "25" {
		t.Errorf("limit = %q", params.Get("limit"))
	}
	if params.Get("offset") != "0" {
		t.Errorf("offset = %q", params.Get("offset"))
	}
	if params.Get("cat") != "4000" {
		t.Errorf("cat = %q", params.Get("cat"))
	}
}

func TestBuildQuery_RoutesToMovieAndTV(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		q    indexers.Query
		want string
	}{
		{"imdb routes to movie", indexers.Query{IMDBID: "tt0111161"}, "movie"},
		{"tmdb routes to movie", indexers.Query{TMDBID: "550"}, "movie"},
		{"tvdb routes to tvsearch", indexers.Query{TVDBID: "12345"}, "tvsearch"},
		{"season routes to tvsearch", indexers.Query{Season: 2}, "tvsearch"},
		{"empty routes to search", indexers.Query{Term: "x"}, "search"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mode, _ := buildQuery(tc.q, Config{})
			if mode != tc.want {
				t.Errorf("mode = %q, want %q", mode, tc.want)
			}
		})
	}
}

func TestBuildQuery_StripsIMDBPrefix(t *testing.T) {
	t.Parallel()
	_, params := buildQuery(indexers.Query{IMDBID: "tt0111161"}, Config{})
	if got := params.Get("imdbid"); got != "0111161" {
		t.Errorf("imdbid = %q, want 0111161", got)
	}
}

func TestBuildURL_AppendsAPIKey(t *testing.T) {
	t.Parallel()
	c := &Client{cfg: Config{URL: "https://example.com/api", APIKey: "secret"}}
	u := c.buildURL("search", url.Values{"q": []string{"hello"}})
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Query().Get("apikey") != "secret" {
		t.Errorf("apikey missing: %s", u)
	}
	if parsed.Query().Get("t") != "search" {
		t.Errorf("t param missing: %s", u)
	}
	if parsed.Query().Get("q") != "hello" {
		t.Errorf("q param missing: %s", u)
	}
}
