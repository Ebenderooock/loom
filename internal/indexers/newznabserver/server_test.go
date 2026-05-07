package newznabserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/indexers"
)

// stubSearcher implements Searcher for end-to-end handler tests
// without standing up the real *indexers.Service. It records the
// query it received and returns a canned AggregatedResults.
type stubSearcher struct {
	registry *indexers.Registry
	got      indexers.Query
	results  []indexers.Result
}

func (s *stubSearcher) Registry() *indexers.Registry { return s.registry }
func (s *stubSearcher) Search(ctx context.Context, q indexers.Query, ids []string, _ time.Duration) indexers.AggregatedResults {
	s.got = q
	return indexers.AggregatedResults{
		Results: s.results,
		Errors:  map[string]string{},
	}
}

// stubVerifier accepts only "good".
type stubVerifier struct{}

func (stubVerifier) VerifyAPIKey(ctx context.Context, presentedKey string) error {
	if presentedKey == "good" {
		return nil
	}
	return errors.New("nope")
}

func newTestServer(t *testing.T, results []indexers.Result, withAuth bool) (*Server, *stubSearcher) {
	t.Helper()
	reg := indexers.NewRegistry()
	if err := reg.Register(&fakeIndexer{
		id:   "alpha",
		name: "Alpha",
		caps: indexers.Caps{
			SearchTypes: []string{"search", "tvsearch"},
			Categories:  []indexers.Category{indexers.CategoryTV},
		},
	}); err != nil {
		t.Fatal(err)
	}
	stub := &stubSearcher{registry: reg, results: results}
	opts := Options{Search: stub, Title: "Loom", Strapline: "test"}
	if withAuth {
		opts.Auth = stubVerifier{}
	}
	srv, err := NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	return srv, stub
}

func mountAndRequest(t *testing.T, srv *Server, target string) *http.Response {
	t.Helper()
	r := chi.NewRouter()
	srv.Mount(r)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Result()
}

func TestHandlerCaps(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, nil, false)
	resp := mountAndRequest(t, srv, "/api?t=caps")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/xml") {
		t.Errorf("content-type: %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	for _, want := range []string{
		`<?xml`,
		`<caps>`,
		`<server`,
		`<searching>`,
		`<search available="yes"`,
		`<tv-search available="yes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("caps body missing %q\n%s", want, out)
		}
	}
}

func TestHandlerSearchRendersResults(t *testing.T) {
	t.Parallel()
	results := []indexers.Result{
		{
			IndexerID: "alpha",
			Title:     "Hit One",
			GUID:      "g1",
			Link:      "https://x/1.nzb",
			Size:      999,
			Category:  []indexers.Category{indexers.CategoryTV},
			PubDate:   time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC),
		},
	}
	srv, stub := newTestServer(t, results, false)
	resp := mountAndRequest(t, srv, "/api?t=search&q=hit&cat=5000&limit=25")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if stub.got.Term != "hit" {
		t.Errorf("query term: %q", stub.got.Term)
	}
	if stub.got.Limit != 25 {
		t.Errorf("query limit: %d", stub.got.Limit)
	}
	if len(stub.got.Categories) != 1 || stub.got.Categories[0] != indexers.CategoryTV {
		t.Errorf("query categories: %v", stub.got.Categories)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, "<title>Hit One</title>") {
		t.Errorf("body missing item title:\n%s", out)
	}
	if !strings.Contains(out, `<newznab:attr name="indexer" value="alpha">`) {
		t.Errorf("body missing indexer attr:\n%s", out)
	}
}

func TestHandlerMovieParamsPropagate(t *testing.T) {
	t.Parallel()
	srv, stub := newTestServer(t, nil, false)
	_ = mountAndRequest(t, srv, "/api?t=movie&imdbid=tt0133093&tmdbid=603")
	if stub.got.IMDBID != "tt0133093" {
		t.Errorf("imdbid: %q", stub.got.IMDBID)
	}
	if stub.got.TMDBID != "603" {
		t.Errorf("tmdbid: %q", stub.got.TMDBID)
	}
	if len(stub.got.Categories) != 1 || stub.got.Categories[0] != indexers.CategoryMovies {
		t.Errorf("default movies cat missing: %v", stub.got.Categories)
	}
}

func TestHandlerTVSearchParamsPropagate(t *testing.T) {
	t.Parallel()
	srv, stub := newTestServer(t, nil, false)
	_ = mountAndRequest(t, srv, "/api?t=tvsearch&tvdbid=12345&season=2&ep=4")
	if stub.got.TVDBID != "12345" {
		t.Errorf("tvdbid: %q", stub.got.TVDBID)
	}
	if stub.got.Season != 2 || stub.got.Episode != 4 {
		t.Errorf("season/ep: %d/%d", stub.got.Season, stub.got.Episode)
	}
}

func TestHandlerUnknownModeReturnsXMLError(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, nil, false)
	resp := mountAndRequest(t, srv, "/api?t=details")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, `code="202"`) {
		t.Errorf("missing 202 error code:\n%s", out)
	}
	if !strings.Contains(out, "<error") {
		t.Errorf("missing error element:\n%s", out)
	}
}

func TestHandlerMissingTReturnsXMLError(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, nil, false)
	resp := mountAndRequest(t, srv, "/api")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `code="200"`) {
		t.Errorf("expected code 200:\n%s", body)
	}
}

func TestHandlerAuthRequiredWhenVerifierSet(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, nil, true)
	cases := []struct {
		name   string
		url    string
		status int
	}{
		{"no key", "/api?t=caps", http.StatusUnauthorized},
		{"bad key", "/api?t=caps&apikey=bad", http.StatusUnauthorized},
		{"good key", "/api?t=caps&apikey=good", http.StatusOK},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			resp := mountAndRequest(t, srv, c.url)
			if resp.StatusCode != c.status {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("want %d got %d\n%s", c.status, resp.StatusCode, body)
			}
		})
	}
}

func TestHandlerAcceptsXApiKeyHeader(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, nil, true)
	r := chi.NewRouter()
	srv.Mount(r)
	req := httptest.NewRequest(http.MethodGet, "/api?t=caps", nil)
	req.Header.Set("X-Api-Key", "good")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("X-Api-Key fallback failed: %d", rec.Code)
	}
}

func TestMountServesBothPaths(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, nil, false)
	r := chi.NewRouter()
	srv.Mount(r)
	for _, path := range []string{"/api?t=caps", "/api/v1/aggregate?t=caps"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("path %s: status %d", path, rec.Code)
		}
	}
}

func TestNewServerRequiresSearch(t *testing.T) {
	t.Parallel()
	if _, err := NewServer(Options{}); err == nil {
		t.Errorf("expected error for missing search backend")
	}
}
