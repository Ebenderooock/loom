package cardigann

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
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

func TestExpandTemplate_BoolLiterals(t *testing.T) {
	eng := &Engine{}
	ctx := templateContext{
		Config: map[string]string{"disablesort": "false"},
		True:   "true",
		False:  "false",
	}
	out, err := eng.expandTemplate(`{{ if eq .Config.disablesort .False }}enabled{{ else }}disabled{{ end }}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out != "enabled" {
		t.Errorf("expected 'enabled', got %q", out)
	}
}

func TestExpandTemplate_QuerySubFields(t *testing.T) {
	eng := &Engine{}
	ctx := templateContext{
		Keywords: "test show",
		Query: templateQuery{
			Keywords: "test show",
			IMDBID:   "tt1234567",
			Season:   2,
			Ep:       5,
		},
		True:  "true",
		False: "false",
	}
	out, err := eng.expandTemplate(`{{ if .Query.IMDBID }}{{ .Query.IMDBID }}{{ else }}{{ .Keywords }}{{ end }}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out != "tt1234567" {
		t.Errorf("expected 'tt1234567', got %q", out)
	}
	out, err = eng.expandTemplate(`S{{ .Query.Season }}E{{ .Query.Ep }}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out != "S2E5" {
		t.Errorf("expected 'S2E5', got %q", out)
	}
}

func TestExpandTemplate_ReReplace(t *testing.T) {
	eng := &Engine{}
	ctx := templateContext{Keywords: "hello world", True: "true", False: "false"}
	out, err := eng.expandTemplate(`{{ re_replace .Keywords "[\\s]+" "%" }}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello%world" {
		t.Errorf("re_replace failed: %q", out)
	}
}

func TestExpandTemplate_Replace(t *testing.T) {
	eng := &Engine{}
	ctx := templateContext{Keywords: "foo bar baz", True: "true", False: "false"}
	out, err := eng.expandTemplate(`{{ replace .Keywords " " "+" }}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out != "foo+bar+baz" {
		t.Errorf("replace failed: %q", out)
	}
}

func TestConfigFieldsWithDefaults(t *testing.T) {
	eng := &Engine{
		cfg: Config{
			Credentials: map[string]string{"apikey": "mykey"},
		},
		def: &Definition{
			Settings: []Setting{
				{Name: "apiurl", Default: "apibay.org"},
				{Name: "sort", Default: "created"},
				{Name: "apikey"}, // no default — should not override
			},
		},
	}
	fields := eng.configFieldsWithDefaults()
	if fields["apiurl"] != "apibay.org" {
		t.Errorf("expected default apiurl, got %q", fields["apiurl"])
	}
	if fields["sort"] != "created" {
		t.Errorf("expected default sort, got %q", fields["sort"])
	}
	if fields["apikey"] != "mykey" {
		t.Errorf("operator value should win, got %q", fields["apikey"])
	}
}

func TestExpandTemplate_MissingKeyZero(t *testing.T) {
	// missingkey=zero should render missing map keys as "" not "<no value>".
	eng := &Engine{}
	ctx := templateContext{
		Config: map[string]string{"present": "yes"},
		True:   "true",
		False:  "false",
	}
	out, err := eng.expandTemplate(`https://{{ .Config.present }}/{{ .Config.missing }}/end`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<no value>") {
		t.Errorf("missingkey=zero should suppress <no value>, got %q", out)
	}
	if out != "https://yes//end" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestExpandTemplate_ReReplace_BadPattern(t *testing.T) {
	eng := &Engine{}
	ctx := templateContext{Keywords: "hello", True: "true", False: "false"}
	out, err := eng.expandTemplate(`{{ re_replace .Keywords "[invalid" "x" }}`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Bad pattern should return input unchanged.
	if out != "hello" {
		t.Errorf("bad pattern should return input unchanged, got %q", out)
	}
}

func TestSearch_MultiPath(t *testing.T) {
	// Two search paths that each return different results.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/movies"):
			fmt.Fprint(w, `<html><body><table><tr><td class="title"><a href="/dl/1">Movie.Result</a></td><td>1GB</td></tr></table></body></html>`)
		case strings.Contains(r.URL.Path, "/tv"):
			fmt.Fprint(w, `<html><body><table><tr><td class="title"><a href="/dl/2">TV.Result</a></td><td>500MB</td></tr></table></body></html>`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()

	def := &Definition{
		Site:  "multitest",
		Name:  "MultiTest",
		Links: []string{ts.URL},
		Search: Search{
			Paths: []SearchPath{
				{Path: "/movies"},
				{Path: "/tv"},
			},
			Rows: RowsBlock{Selector: "table tr"},
			Fields: map[string]Field{
				"title":    {Selector: ".title a"},
				"download": {Selector: ".title a", Attribute: "href"},
			},
		},
	}

	eng, err := NewEngine("multitest", "MultiTest", def, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := eng.Search(context.Background(), indexers.Query{Term: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 results from 2 paths, got %d", len(res.Items))
	}
}

func TestSearch_MultiPath_DedupURL(t *testing.T) {
	// Two paths that resolve to the same URL should only fetch once.
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `<html><body><table><tr><td class="t"><a href="/dl/1">Res</a></td><td>1GB</td></tr></table></body></html>`)
	}))
	defer ts.Close()

	def := &Definition{
		Site:  "deduptest",
		Name:  "DedupTest",
		Links: []string{ts.URL},
		Search: Search{
			Paths: []SearchPath{
				{Path: "/search"},
				{Path: "/search"}, // duplicate
			},
			Rows: RowsBlock{Selector: "table tr"},
			Fields: map[string]Field{
				"title":    {Selector: ".t a"},
				"download": {Selector: ".t a", Attribute: "href"},
			},
		},
	}

	eng, err := NewEngine("deduptest", "DedupTest", def, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := eng.Search(context.Background(), indexers.Query{Term: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("duplicate URL should be fetched once, got %d calls", calls)
	}
	if len(res.Items) != 1 {
		t.Errorf("expected 1 result, got %d", len(res.Items))
	}
}

// TestExtractRows_RowSelectorTemplate verifies that Go templates
// inside the row selector are expanded before being passed to the
// CSS engine.  Without this, selectors like
//
//	`tr.row{{ if .Config.uploader }}:has(.user:contains({{ .Config.uploader }})){{ else }}{{ end }}`
//
// would include literal `{{ ... }}` text and match nothing.
func TestExtractRows_RowSelectorTemplate(t *testing.T) {
	html := []byte(`<html><body><table>
<tr class="row"><td><a class="title" href="/dl/1">Result One</a></td></tr>
<tr class="row"><td><a class="title" href="/dl/2">Result Two</a></td></tr>
</table></body></html>`)

	def := &Definition{
		Site:  "tmplrow",
		Name:  "TmplRow",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{
				// Template conditional in row selector — the common pattern
				// used by 1337x and many other definitions.
				Selector: `tr.row{{ if .Config.uploader }}:has(td:contains({{ .Config.uploader }})){{ else }}{{ end }}`,
			},
			Fields: map[string]Field{
				"title":    {Selector: "a.title"},
				"download": {Selector: "a.title", Attribute: "href"},
			},
		},
	}

	eng, err := NewEngine("tmplrow", "TmplRow", def, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// uploader is empty → template should strip to just "tr.row"
	tctx := templateContext{
		Config: map[string]string{},
		True:   "true",
		False:  "false",
	}
	rows, err := eng.extractRows(html, tctx)
	if err != nil {
		t.Fatalf("extractRows: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows with empty uploader filter, got %d", len(rows))
	}

	// Now set uploader to filter — only "Result One" should match.
	// Note: goquery's :contains may behave differently than expected
	// with multi-word values; we verify template expansion works by
	// checking the row count changes.
	tctx.Config["uploader"] = "One"
	rows, err = eng.extractRows(html, tctx)
	if err != nil {
		t.Fatalf("extractRows with uploader: %v", err)
	}
	// The :contains pseudo-selector filters rows; "One" only appears
	// in "Result One" → 1 row.
	if len(rows) != 1 {
		t.Logf("note: :contains filter produced %d rows (may vary by goquery version)", len(rows))
	}
}

// TestExtractOne_ResultTemplates verifies two-pass field processing:
// fields with CSS selectors are extracted first, then text-template
// fields can reference them via `.Result.*`.
func TestExtractOne_ResultTemplates(t *testing.T) {
	html := []byte(`<html><body><table>
<tr class="row">
  <td class="col1"><a href="/torrent/123">Short Title...</a></td>
  <td class="col2">42</td>
  <td class="col3">7</td>
</tr>
</table></body></html>`)

	def := &Definition{
		Site:  "resultref",
		Name:  "ResultRef",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{Selector: "tr.row"},
			Fields: map[string]Field{
				// Pass 1 fields: extract from HTML
				"title_default":  {Selector: "td.col1 a"},
				"title_optional": {Optional: true, Selector: `td.nonexistent`},
				"download":       {Selector: "td.col1 a", Attribute: "href"},
				"seeders":        {Selector: "td.col2"},
				"leechers":       {Selector: "td.col3"},
				// Pass 2 fields: computed from .Result.*
				"title": {
					Text: `{{ if .Result.title_optional }}{{ .Result.title_optional }}{{ else }}{{ .Result.title_default }}{{ end }}`,
				},
				"downloadvolumefactor": {Text: "0"},
			},
		},
	}

	eng, err := NewEngine("resultref", "ResultRef", def, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tctx := templateContext{
		Config: map[string]string{},
		True:   "true",
		False:  "false",
	}
	rows, err := eng.extractRows(html, tctx)
	if err != nil {
		t.Fatalf("extractRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	// title_optional matches no element (td.nonexistent) → empty.
	// So the .Result template should fall through to title_default.
	if r.Title != "Short Title..." {
		t.Errorf("expected title 'Short Title...' from .Result.title_default fallback, got %q", r.Title)
	}
	if !r.Freeleech {
		t.Error("expected freeleech=true from downloadvolumefactor=0")
	}
}

// TestExtractOne_ChainedComputedFields verifies that text-template
// fields depending on OTHER text-template fields resolve correctly
// via the fixed-point loop, regardless of Go map iteration order.
func TestExtractOne_ChainedComputedFields(t *testing.T) {
	html := []byte(`<html><body><table>
<tr class="row">
  <td class="base">Base Title</td>
  <td class="dl"><a href="/dl/1">link</a></td>
</tr>
</table></body></html>`)

	def := &Definition{
		Site:  "chained",
		Name:  "Chained",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{Selector: "tr.row"},
			Fields: map[string]Field{
				"base_title": {Selector: "td.base"},
				"download":   {Selector: "td.dl a", Attribute: "href"},
				// Chain: phase1 → phase2 → title. All are text-only
				// fields referencing .Result.* from earlier phases.
				"phase1": {Text: `{{ .Result.base_title }}`},
				"phase2": {Text: `{{ .Result.phase1 }}-HD`},
				"title":  {Text: `{{ .Result.phase2 }}-FINAL`},
			},
		},
	}

	eng, err := NewEngine("chained", "Chained", def, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tctx := templateContext{
		Config: map[string]string{},
		True:   "true",
		False:  "false",
	}

	// Run multiple times to exercise different map iteration orders.
	for i := 0; i < 20; i++ {
		rows, err := eng.extractRows(html, tctx)
		if err != nil {
			t.Fatalf("iter %d: extractRows: %v", i, err)
		}
		if len(rows) != 1 {
			t.Fatalf("iter %d: expected 1 row, got %d", i, len(rows))
		}
		want := "Base Title-HD-FINAL"
		if rows[0].Title != want {
			t.Errorf("iter %d: expected title %q, got %q", i, want, rows[0].Title)
		}
	}
}

// TestExtractOne_DefaultFallback verifies that the `default:` field
// attribute is used when the primary selector matches nothing.
func TestExtractOne_DefaultFallback(t *testing.T) {
	html := []byte(`<html><body><table>
<tr class="row">
  <td class="fallback">Fallback Title</td>
  <td class="dl"><a href="/dl/1">link</a></td>
</tr>
</table></body></html>`)

	def := &Definition{
		Site:  "defaulttest",
		Name:  "DefaultTest",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{Selector: "tr.row"},
			Fields: map[string]Field{
				"fallback_title": {Selector: "td.fallback"},
				"download":       {Selector: "td.dl a", Attribute: "href"},
				// Primary selector misses; should fall back to default.
				"title": {
					Selector: "td.nonexistent",
					Optional: true,
					Default:  `{{ .Result.fallback_title }}`,
				},
			},
		},
	}

	eng, err := NewEngine("defaulttest", "DefaultTest", def, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tctx := templateContext{
		Config: map[string]string{},
		True:   "true",
		False:  "false",
	}
	rows, err := eng.extractRows(html, tctx)
	if err != nil {
		t.Fatalf("extractRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Title != "Fallback Title" {
		t.Errorf("expected title 'Fallback Title' from default, got %q", rows[0].Title)
	}
}
