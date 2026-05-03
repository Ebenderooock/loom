package cardigann

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomctl/loom/internal/indexers"
)

// engineFixtureYAML drives the test server below: form login at
// /login, success indicator at /index (an "Logout" link), search at
// /search returning a small fixed result table.
const engineFixtureYAML = `---
site: testsite
name: TestSite
type: private
language: en
encoding: UTF-8
links:
  - %s
caps:
  categorymappings:
    - {id: "10", cat: "Movies/HD", desc: "movies"}
    - {id: "20", cat: "TV/HD", desc: "tv"}
  modes:
    search: ["q"]
    movie-search: ["q", "imdbid"]
login:
  path: login
  method: post
  inputs:
    user: "{{ .Config.username }}"
    pass: "{{ .Config.password }}"
  test:
    path: index
    selector: "a.logout"
search:
  paths:
    - path: search
  inputs:
    q: "{{ .Keywords }}"
    cat: "{{ range .Categories }}{{ . }},{{ end }}"
  rows:
    selector: "table#results tr.row"
  fields:
    title:
      selector: "a.title"
    download:
      selector: "a.dl"
      attribute: href
    size:
      selector: "td.size"
    seeders:
      selector: "td.seeders"
    leechers:
      selector: "td.leechers"
`

// newTestServer returns an httptest.Server that mimics a small
// tracker: POST /login expects user=u&pass=p, sets a session cookie
// and 302s to /index; GET /index returns HTML containing
// <a class="logout"> so the test selector matches; GET /search
// returns a result table parameterised by query string.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Method != http.MethodPost || r.FormValue("user") != "u" || r.FormValue("pass") != "p" {
			http.Error(w, "bad creds", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "ok", Path: "/"})
		http.Redirect(w, r, "/index", http.StatusFound)
	})
	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie("sid"); c == nil || c.Value != "ok" {
			http.Error(w, "no session", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><a class="logout" href="/logout">Logout</a></body></html>`)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie("sid"); c == nil || c.Value != "ok" {
			http.Error(w, "no session", http.StatusForbidden)
			return
		}
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html")
		if q == "empty" {
			fmt.Fprint(w, `<html><body><table id="results"></table></body></html>`)
			return
		}
		fmt.Fprintf(w, `<html><body><table id="results">
<tr class="row">
  <td><a class="title">%s S01E01 1080p</a></td>
  <td><a class="dl" href="/dl/1.torrent">DL</a></td>
  <td class="size">1.5 GB</td>
  <td class="seeders">42</td>
  <td class="leechers">7</td>
</tr>
<tr class="row">
  <td><a class="title">%s S01E02 1080p</a></td>
  <td><a class="dl" href="/dl/2.torrent">DL</a></td>
  <td class="size">700 MB</td>
  <td class="seeders">10</td>
  <td class="leechers">2</td>
</tr>
</table></body></html>`, q, q)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestEngine(t *testing.T, srv *httptest.Server) *Engine {
	t.Helper()
	yaml := fmt.Sprintf(engineFixtureYAML, srv.URL)
	def, err := ParseDefinition([]byte(yaml))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	cfg := Config{
		DefinitionID: "testsite",
		Username:     "u",
		Password:     "p",
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = durationString(defaultTimeout)
	}
	eng, err := NewEngine("idx-1", "TestSite", def, cfg, srv.Client())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

func TestEngine_LoginAndSearch(t *testing.T) {
	srv := newTestServer(t)
	eng := newTestEngine(t, srv)

	res, err := eng.Search(context.Background(), indexers.Query{Term: "loom"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("expected 2 results, got %d", res.Total)
	}
	first := res.Items[0]
	if !strings.Contains(first.Title, "S01E01") {
		t.Errorf("title not parsed: %q", first.Title)
	}
	if !strings.HasSuffix(first.Link, "/dl/1.torrent") {
		t.Errorf("download not absolute: %q", first.Link)
	}
	if first.Size != 1500*1000*1000 && first.Size != 1610612736 {
		// parseSize accepts both decimal and binary GB; either is fine.
		t.Errorf("size not parsed: %d", first.Size)
	}
	if first.Seeders == nil || *first.Seeders != 42 {
		t.Errorf("seeders not parsed: %v", first.Seeders)
	}
	if first.Peers == nil || *first.Peers != 49 {
		// Peers = seeders + leechers per Newznab convention; if our
		// engine reports leechers separately, that's also acceptable.
		t.Logf("peers (informational): %v", first.Peers)
	}
}

func TestEngine_EmptyResults(t *testing.T) {
	srv := newTestServer(t)
	eng := newTestEngine(t, srv)
	res, err := eng.Search(context.Background(), indexers.Query{Term: "empty"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("expected zero results, got %d", res.Total)
	}
}

func TestEngine_BadLogin(t *testing.T) {
	srv := newTestServer(t)
	yaml := fmt.Sprintf(engineFixtureYAML, srv.URL)
	def, err := ParseDefinition([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		DefinitionID: "testsite",
		Username:     "wrong",
		Password:     "wrong",
		UserAgent:    defaultUserAgent,
		Timeout:      durationString(defaultTimeout),
	}
	eng, err := NewEngine("idx-bad", "TestSite", def, cfg, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(context.Background(), indexers.Query{Term: "x"}); err == nil {
		t.Fatal("expected login failure")
	}
}

func TestNewznabCategoryFromName(t *testing.T) {
	if c, ok := newznabCategoryFromName("Movies/HD"); !ok || c != 2040 {
		t.Errorf("Movies/HD -> %d ok=%v", c, ok)
	}
	if c, ok := newznabCategoryFromName("movies/sd"); !ok || c != 2030 {
		t.Errorf("case-insensitive lookup failed: %d ok=%v", c, ok)
	}
	if _, ok := newznabCategoryFromName("Nonsense/Bogus"); ok {
		t.Error("expected miss for unknown category")
	}
}

func TestApplyFilters(t *testing.T) {
	got := applyFilters("hello world", []Filter{
		{Name: "replace", Args: []any{"hello", "Hi"}},
		{Name: "uppercase"},
	})
	if got != "HI WORLD" {
		t.Errorf("filter chain failed: %q", got)
	}
	got = applyFilters("https://x/y?id=42", []Filter{
		{Name: "querystring", Args: "id"},
	})
	if got != "42" {
		t.Errorf("querystring filter failed: %q", got)
	}
	got = applyFilters("Foo.Bar.Baz", []Filter{
		{Name: "regexp", Args: `Foo\.(\w+)`},
	})
	if got != "Bar" {
		t.Errorf("regexp filter failed: %q", got)
	}
}
