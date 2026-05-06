package cardigann

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"github.com/loomctl/loom/internal/indexers"
	"golang.org/x/net/html"
)

const (
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	defaultTimeout   = 30 * time.Second
)

// Engine is the live indexers.Indexer for a single Cardigann
// definition. It owns the cookie jar (so login state survives across
// Search calls), the parsed definition, and the operator-supplied
// credentials.
//
// Engine is safe for concurrent use; loginMu serialises the login
// handshake so a burst of parallel Search calls performs the dance
// only once.
type Engine struct {
	id   string
	name string

	def *Definition
	cfg Config

	http *http.Client
	jar  http.CookieJar

	loginMu  sync.Mutex
	loggedIn bool

	// resolved category mapping cache. Built once on first use.
	catMu      sync.RWMutex
	cats       []indexers.Category
	siteToNzb  map[string][]indexers.Category // tracker id → newznab category ids
	nzbToSite  map[indexers.Category][]string // newznab id → tracker ids
}

// NewEngine builds a runnable Engine. httpClient may be nil; the
// constructor fits one with a cookie jar and the configured timeout.
// On any returned error, no goroutines or sockets have been started,
// so the caller can drop the half-built object safely.
func NewEngine(id, name string, def *Definition, cfg Config, httpClient *http.Client) (*Engine, error) {
	if def == nil {
		return nil, errors.New("cardigann: nil definition")
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = durationString(defaultTimeout)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cardigann: cookie jar: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout.duration()}
	}
	// Always layer our jar on top of whatever transport the caller
	// supplied: per-indexer proxy routing happens at the transport
	// level, cookies are independent.
	httpClient.Jar = jar
	if httpClient.Timeout == 0 {
		httpClient.Timeout = cfg.Timeout.duration()
	}

	e := &Engine{
		id:   id,
		name: name,
		def:  def,
		cfg:  cfg,
		http: httpClient,
		jar:  jar,
	}
	// Pre-seed any operator-supplied raw cookie. Cardigann lets a
	// site offer "cookie" as a login method (the operator pastes a
	// session cookie copied from a browser); when present we install
	// it on the jar before any HTTP traffic happens.
	if cookie := strings.TrimSpace(cfg.Cookie); cookie != "" {
		if err := e.installRawCookie(cookie); err != nil {
			return nil, err
		}
		e.loggedIn = true // a manual cookie is a manual session.
	}
	return e, nil
}

// ID implements indexers.Indexer.
func (e *Engine) ID() string { return e.id }

// Name implements indexers.Indexer.
func (e *Engine) Name() string { return e.name }

// Caps implements indexers.Indexer. It returns the modes and
// Newznab-mapped categories declared by the definition.
func (e *Engine) Caps() indexers.Caps {
	e.ensureCategories()
	e.catMu.RLock()
	defer e.catMu.RUnlock()
	return indexers.Caps{
		SearchTypes:  collectModes(e.def.Caps.Modes),
		Categories:   append([]indexers.Category(nil), e.cats...),
		SupportedIDs: collectSupportedIDs(e.def.Caps.Modes),
	}
}

// Test implements indexers.Indexer. For definitions with a login
// block, Test forces a fresh login. For definitions without one, it
// performs a HEAD on the base URL so the operator at least knows the
// host resolves.
func (e *Engine) Test(ctx context.Context) error {
	if e.def.Login != nil {
		// Force a re-login so Test exercises the credentials.
		e.loginMu.Lock()
		e.loggedIn = false
		e.loginMu.Unlock()
		return e.ensureLoggedIn(ctx)
	}
	base, err := e.baseURL()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return fmt.Errorf("cardigann: build test request: %w", err)
	}
	req.Header.Set("User-Agent", e.cfg.UserAgent)
	req.Header.Set("Accept", "text/html, application/xhtml+xml, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("cardigann: test request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cardigann: test got status %d", resp.StatusCode)
	}
	return nil
}

// baseURL returns the active base URL with any trailing slash trimmed.
// If the operator configured a URL override it takes precedence;
// otherwise Links[0] from the YAML definition is used.
func (e *Engine) baseURL() (string, error) {
	if u := strings.TrimSpace(e.cfg.URL); u != "" {
		return strings.TrimRight(u, "/"), nil
	}
	if len(e.def.Links) == 0 {
		return "", errors.New("cardigann: definition has no links")
	}
	return strings.TrimRight(e.def.Links[0], "/"), nil
}

// resolveURL joins a relative path against the site base URL. Paths
// that already start with "http(s)://" are returned unchanged.
func (e *Engine) resolveURL(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	base, err := e.baseURL()
	if err != nil {
		return "", err
	}
	if path == "" {
		return base, nil
	}
	if strings.HasPrefix(path, "/") {
		return base + path, nil
	}
	return base + "/" + path, nil
}

// installRawCookie parses a "name=value; name2=value2" string and
// installs the cookies on the jar against the site base URL.
func (e *Engine) installRawCookie(raw string) error {
	base, err := e.baseURL()
	if err != nil {
		return err
	}
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("cardigann: parse base url for cookie: %w", err)
	}
	parts := strings.Split(raw, ";")
	cookies := make([]*http.Cookie, 0, len(parts))
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(kv[0]),
			Value: strings.TrimSpace(kv[1]),
			Path:  "/",
		})
	}
	if len(cookies) == 0 {
		return errors.New("cardigann: cookie config did not contain any name=value pairs")
	}
	e.jar.SetCookies(u, cookies)
	return nil
}

// ensureLoggedIn drives the login flow at most once per Engine
// lifetime (or after an explicit Test() reset). It is a no-op for
// definitions that omit the login block, or when the operator
// configured a raw cookie (set in NewEngine).
func (e *Engine) ensureLoggedIn(ctx context.Context) error {
	if e.def.Login == nil {
		return nil
	}
	e.loginMu.Lock()
	defer e.loginMu.Unlock()
	if e.loggedIn {
		return nil
	}
	method := strings.ToLower(strings.TrimSpace(e.def.Login.Method))
	switch method {
	case "", "form", "post":
		if err := e.formLogin(ctx); err != nil {
			return err
		}
	case "cookie":
		// "cookie" expects the operator to have supplied a Cookie in
		// config; NewEngine handled that. If it didn't, the Cookie
		// field was empty.
		if strings.TrimSpace(e.cfg.Cookie) == "" {
			return errors.New("cardigann: login.method=cookie requires `cookie` in indexer config")
		}
	case "get":
		return errors.New("cardigann: login.method=get is not supported in this build")
	default:
		return fmt.Errorf("cardigann: unsupported login method %q", e.def.Login.Method)
	}
	if err := e.verifyLogin(ctx); err != nil {
		return err
	}
	e.loggedIn = true
	return nil
}

// formLogin performs the form-encoded POST that most Cardigann
// definitions use. Inputs are templated against the Config so
// {{ .Config.username }} expands to the operator-supplied value.
func (e *Engine) formLogin(ctx context.Context) error {
	loginURL, err := e.resolveURL(e.def.Login.Path)
	if err != nil {
		return err
	}
	form := url.Values{}
	for k, tmpl := range e.def.Login.Inputs {
		v, terr := e.expandTemplate(tmpl, templateContext{Config: e.cfg.fields()})
		if terr != nil {
			return fmt.Errorf("cardigann: login input %q: %w", k, terr)
		}
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("cardigann: build login request: %w", err)
	}
	req.Header.Set("User-Agent", e.cfg.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("cardigann: login request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// A login that yields a 4xx/5xx (other than the redirect chain
	// already followed by net/http) almost certainly failed.
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cardigann: login got status %d", resp.StatusCode)
	}
	if msg := matchErrorSelectors(body, e.def.Login.Error); msg != "" {
		return fmt.Errorf("cardigann: login rejected: %s", msg)
	}
	return nil
}

// verifyLogin fetches the test path and asserts the success selector
// matches. When the definition omits the test block we trust the
// formLogin status check.
func (e *Engine) verifyLogin(ctx context.Context) error {
	if e.def.Login.Test.Path == "" || e.def.Login.Test.Selector == "" {
		return nil
	}
	testURL, err := e.resolveURL(e.def.Login.Test.Path)
	if err != nil {
		return err
	}
	body, err := e.fetch(ctx, http.MethodGet, testURL, nil, nil)
	if err != nil {
		return fmt.Errorf("cardigann: verify login: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cardigann: verify login parse: %w", err)
	}
	if doc.Find(e.def.Login.Test.Selector).Length() == 0 {
		return errors.New("cardigann: login test selector did not match — credentials are probably wrong")
	}
	return nil
}

// matchErrorSelectors runs each error selector against the body and
// returns the first non-empty hit. We use goquery here because the
// Cardigann error selectors are CSS, not XPath.
func matchErrorSelectors(body []byte, blocks []ErrorBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	for _, b := range blocks {
		sel := doc.Find(b.Selector)
		if sel.Length() == 0 {
			continue
		}
		// Plain text message takes precedence.
		if b.Message.Text != "" {
			return b.Message.Text
		}
		// Prowlarr-style: extract message from a nested CSS selector.
		if b.Message.Selector != "" {
			msgSel := sel.Find(b.Message.Selector)
			if msgSel.Length() > 0 {
				return strings.TrimSpace(msgSel.First().Text())
			}
		}
		return strings.TrimSpace(sel.First().Text())
	}
	return ""
}

// Search implements indexers.Indexer.
func (e *Engine) Search(ctx context.Context, q indexers.Query) (*indexers.Results, error) {
	if err := e.ensureLoggedIn(ctx); err != nil {
		return nil, err
	}
	method, target, params, headers, err := e.buildSearchRequest(q)
	if err != nil {
		return nil, err
	}
	body, err := e.fetch(ctx, method, target, params, headers)
	if err != nil {
		return nil, err
	}
	rows, err := e.extractRows(body)
	if err != nil {
		return nil, err
	}
	return &indexers.Results{
		IndexerID: e.id,
		Items:     rows,
		Total:     len(rows),
	}, nil
}

// buildSearchRequest assembles the URL, method, query parameters and
// headers from the search block and the user query. Result is the
// data needed to drive fetch().
func (e *Engine) buildSearchRequest(q indexers.Query) (method, target string, params url.Values, headers http.Header, err error) {
	path := e.def.Search.Path
	method = strings.ToUpper(e.def.Search.Method)
	if method == "" {
		method = http.MethodGet
	}
	if path == "" && len(e.def.Search.Paths) > 0 {
		path = e.def.Search.Paths[0].Path
		if m := e.def.Search.Paths[0].Method; m != "" {
			method = strings.ToUpper(m)
		}
	}
	target, err = e.resolveURL(path)
	if err != nil {
		return "", "", nil, nil, err
	}

	// Apply keyword filters (e.g. replace spaces with hyphens for
	// URL-slug sites like EZTV) before template expansion.
	keywords := q.Term
	if len(e.def.Search.KeywordsFilters) > 0 {
		keywords = applyFilters(keywords, e.def.Search.KeywordsFilters)
	}

	tctx := templateContext{
		Keywords:   keywords,
		Query:      q.Term,
		Categories: e.mapNewznabToSite(q.Categories),
		IMDBID:     strings.TrimPrefix(q.IMDBID, "tt"),
		TVDBID:     q.TVDBID,
		TMDBID:     q.TMDBID,
		Season:     q.Season,
		Episode:    q.Episode,
		Config:     e.cfg.fields(),
	}

	params = url.Values{}
	for k, tmpl := range e.def.Search.Inputs {
		v, terr := e.expandTemplate(tmpl, tctx)
		if terr != nil {
			return "", "", nil, nil, fmt.Errorf("cardigann: search input %q: %w", k, terr)
		}
		// Cardigann uses `$raw` to inject a pre-encoded fragment.
		// We expose it here as a flat appended query string so a
		// definition can emit "c1=1&c5=1" for multi-category posts.
		if k == "$raw" && v != "" {
			extras, perr := url.ParseQuery(v)
			if perr != nil {
				return "", "", nil, nil, fmt.Errorf("cardigann: search $raw is not valid query: %w", perr)
			}
			for ek, evs := range extras {
				for _, ev := range evs {
					params.Add(ek, ev)
				}
			}
			continue
		}
		params.Set(k, v)
	}

	headers = http.Header{}
	for k, vals := range e.def.Search.Headers {
		v, terr := e.expandTemplate(vals.First(), tctx)
		if terr != nil {
			return "", "", nil, nil, fmt.Errorf("cardigann: search header %q: %w", k, terr)
		}
		headers.Set(k, v)
	}
	return method, target, params, headers, nil
}

// fetch performs a cookie-aware HTTP request and returns the body.
// For GET we encode params on the query string; for POST we send them
// as application/x-www-form-urlencoded.
func (e *Engine) fetch(ctx context.Context, method, target string, params url.Values, headers http.Header) ([]byte, error) {
	var (
		req  *http.Request
		err  error
		full = target
	)
	if method == "" {
		method = http.MethodGet
	}
	if method == http.MethodGet && len(params) > 0 {
		sep := "?"
		if strings.Contains(full, "?") {
			sep = "&"
		}
		full = full + sep + params.Encode()
	}
	if method == http.MethodPost {
		req, err = http.NewRequestWithContext(ctx, method, full, strings.NewReader(params.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, method, full, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("cardigann: build request %s %s: %w", method, target, err)
	}
	req.Header.Set("User-Agent", e.cfg.UserAgent)
	req.Header.Set("Accept", "text/html, application/xhtml+xml, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Referer helps bypass Cloudflare bot checks on search pages.
	if base, berr := e.baseURL(); berr == nil {
		req.Header.Set("Referer", base+"/")
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, derr := e.http.Do(req)
	if derr != nil {
		return nil, fmt.Errorf("cardigann: %s %s: %w", method, target, derr)
	}
	defer resp.Body.Close()
	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return nil, fmt.Errorf("cardigann: read body: %w", rerr)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("cardigann: upstream status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// Drop the logged-in flag so a subsequent search retries the
		// login dance. We still return the auth error to the caller
		// rather than silently retrying.
		e.loginMu.Lock()
		e.loggedIn = false
		e.loginMu.Unlock()
		return nil, fmt.Errorf("cardigann: unauthorized (status %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("cardigann: status %d", resp.StatusCode)
	}
	return body, nil
}

// extractRows parses the response HTML, runs the rows selector, and
// applies every field selector against each row to produce one
// indexers.Result per match.
//
// The function is the heart of the engine and intentionally chatty
// in its commentary: tracing a missing field through a stack of
// CSS/XPath selectors is the most common debugging task operators
// face, and the comments here map straight to that workflow.
func (e *Engine) extractRows(body []byte) ([]indexers.Result, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cardigann: parse search html: %w", err)
	}
	rowSelector := e.def.Search.Rows.Selector
	rows := selectNodes(doc.Selection, rowSelector)
	// Cardigann's `after:` strips a header row; we honour positive
	// values only because negatives have no upstream semantics.
	if e.def.Search.Rows.After > 0 && len(rows) > e.def.Search.Rows.After {
		rows = rows[e.def.Search.Rows.After:]
	}

	out := make([]indexers.Result, 0, len(rows))
	for _, node := range rows {
		r, ok := e.extractOne(node)
		if !ok {
			continue
		}
		r.IndexerID = e.id
		out = append(out, r)
	}
	return out, nil
}

// extractOne walks every field selector against a single row node.
// A row is dropped when it has neither title nor download link — both
// are required for the result to be useful downstream.
func (e *Engine) extractOne(node *goquery.Selection) (indexers.Result, bool) {
	r := indexers.Result{}
	values := map[string]string{}
	for name, field := range e.def.Search.Fields {
		v := e.applyField(node, field)
		values[name] = v
	}
	r.Title = strings.TrimSpace(values["title"])
	r.GUID = strings.TrimSpace(values["guid"])
	r.Link = strings.TrimSpace(values["download"])
	if r.Link == "" {
		r.Link = strings.TrimSpace(values["link"])
	}
	r.InfoURL = strings.TrimSpace(values["details"])
	if r.InfoURL == "" {
		r.InfoURL = strings.TrimSpace(values["comments"])
	}
	r.Quality = strings.TrimSpace(values["quality"])
	r.MagnetURI = strings.TrimSpace(values["magnet"])
	r.Infohash = strings.TrimSpace(values["infohash"])
	if size := values["size"]; size != "" {
		r.Size = parseSize(size)
	}
	if pd := values["date"]; pd != "" {
		r.PubDate = parseDateBestEffort(pd)
	}
	if seedersStr, ok := values["seeders"]; ok && seedersStr != "" {
		v := parseInt(seedersStr)
		r.Seeders = &v
	}
	if peersStr, ok := values["peers"]; ok && peersStr != "" {
		v := parseInt(peersStr)
		r.Peers = &v
	} else if leechersStr, ok := values["leechers"]; ok && leechersStr != "" && r.Seeders != nil {
		v := *r.Seeders + parseInt(leechersStr)
		r.Peers = &v
	}
	if cat := values["category"]; cat != "" {
		r.Category = e.mapSiteCategory(cat)
	}
	// Tracker intelligence flags. downloadvolumefactor=0 means
	// freeleech; the value is a multiplier extracted by the YAML
	// definition's selector/case chain.
	if dvf := values["downloadvolumefactor"]; dvf != "" {
		r.Freeleech = dvf == "0" || dvf == "0.0"
	}
	// Some definitions carry an explicit "internal" or "scene" field
	// via boolean-style selectors (presence of a CSS class or image).
	if iv := values["_internal"]; iv != "" && iv != "0" {
		r.Internal = true
	}
	if sv := values["_scene"]; sv != "" && sv != "0" {
		r.Scene = true
	}
	// Resolve relative URLs against the site base.
	r.Link = e.absoluteURL(r.Link)
	r.InfoURL = e.absoluteURL(r.InfoURL)

	if r.Title == "" || (r.Link == "" && r.MagnetURI == "") {
		return r, false
	}
	return r, true
}

// applyField runs one field's selector + filter pipeline against the
// row node. Returns "" when the selector misses (and the field is
// optional) or when the chain produces an empty string.
func (e *Engine) applyField(row *goquery.Selection, f Field) string {
	if f.Text != "" && f.Selector == "" {
		return applyFilters(f.Text, f.Filters)
	}
	if f.Selector == "" {
		return ""
	}
	nodes := selectNodes(row, f.Selector)
	if len(nodes) == 0 {
		return ""
	}
	first := nodes[0]
	var raw string
	if f.Attribute != "" {
		raw, _ = first.Attr(f.Attribute)
	} else {
		raw = strings.TrimSpace(first.Text())
	}
	if f.Remove != "" {
		raw = strings.ReplaceAll(raw, f.Remove, "")
	}
	return applyFilters(raw, f.Filters)
}

// absoluteURL resolves rel against the definition's base URL. A
// blank input yields a blank output (callers treat that as "no
// link").
func (e *Engine) absoluteURL(ref string) string {
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "magnet:") {
		return ref
	}
	base, err := e.baseURL()
	if err != nil {
		return ref
	}
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

// selectNodes accepts either a CSS selector or an XPath expression
// and returns the matching nodes wrapped as goquery selections.
//
// We detect XPath by leading "/" or "(": those characters are not
// valid at the start of a CSS selector. The rest of the engine
// operates on goquery.Selection so callers don't need to branch.
func selectNodes(scope *goquery.Selection, selector string) []*goquery.Selection {
	if selector == "" {
		return nil
	}
	if isXPath(selector) {
		return runXPath(scope, selector)
	}
	out := []*goquery.Selection{}
	scope.Find(selector).Each(func(_ int, s *goquery.Selection) {
		out = append(out, s)
	})
	return out
}

// isXPath returns true when the selector should be evaluated as
// XPath. It is a heuristic but matches Cardigann's own parser.
func isXPath(selector string) bool {
	s := strings.TrimSpace(selector)
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "(")
}

// runXPath evaluates expr against the scope using htmlquery and
// re-wraps each hit in a goquery.Selection so the rest of the engine
// stays uniform. We do this by serialising the matched node back to
// HTML and re-parsing — slow, but the tracker pages we care about
// are tiny by web standards.
func runXPath(scope *goquery.Selection, expr string) []*goquery.Selection {
	if scope.Length() == 0 {
		return nil
	}
	rootNode := scope.Get(0)
	hits, err := htmlquery.QueryAll(rootNode, expr)
	if err != nil || len(hits) == 0 {
		return nil
	}
	out := make([]*goquery.Selection, 0, len(hits))
	for _, n := range hits {
		out = append(out, wrapHTMLNode(n))
	}
	return out
}

// wrapHTMLNode re-wraps a raw *html.Node as a goquery.Selection.
func wrapHTMLNode(n *html.Node) *goquery.Selection {
	doc := goquery.NewDocumentFromNode(n)
	return doc.Selection
}

// templateContext is the set of variables Cardigann input templates
// can reference via {{ .Keywords }}, {{ .Config.username }}, etc.
type templateContext struct {
	Keywords   string
	Query      string
	Categories []string
	IMDBID     string
	TVDBID     string
	TMDBID     string
	Season     int
	Episode    int
	Config     map[string]string
}

// expandTemplate renders tmpl with ctx using text/template. Loom does
// not support the full Cardigann template surface (no `if join` etc);
// we ship the variable-substitution subset and rely on Go template
// `range` for category fan-out, which covers the public-tracker
// definitions we have tested.
func (e *Engine) expandTemplate(tmpl string, ctx templateContext) (string, error) {
	if tmpl == "" {
		return "", nil
	}
	t, err := template.New("cardigann").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// ensureCategories lazily resolves the category mapping into both
// directions (site → newznab and newznab → site). Cardigann has two
// schema variants (flat `categories:` map and the modern
// `categorymappings:` list); we accept both.
func (e *Engine) ensureCategories() {
	e.catMu.RLock()
	if e.cats != nil {
		e.catMu.RUnlock()
		return
	}
	e.catMu.RUnlock()

	e.catMu.Lock()
	defer e.catMu.Unlock()
	if e.cats != nil {
		return
	}
	siteToNzb := map[string][]indexers.Category{}
	nzbToSite := map[indexers.Category][]string{}
	allCats := map[indexers.Category]bool{}

	for siteID, nzb := range e.def.Caps.Categories {
		c, ok := newznabCategoryFromName(nzb)
		if !ok {
			// Try parsing as a numeric category ID for legacy format.
			n, err := atoiSafe(nzb)
			if err != nil {
				continue
			}
			c = indexers.Category(n)
		}
		siteToNzb[siteID] = append(siteToNzb[siteID], c)
		nzbToSite[c] = append(nzbToSite[c], siteID)
		allCats[c] = true
	}
	for _, m := range e.def.Caps.CategoryMappings {
		nzb, ok := newznabCategoryFromName(m.Cat)
		if !ok {
			continue
		}
		siteToNzb[m.ID] = append(siteToNzb[m.ID], nzb)
		nzbToSite[nzb] = append(nzbToSite[nzb], m.ID)
		allCats[nzb] = true
	}
	// Apply operator overrides so the API can patch a single mapping
	// without editing the YAML file.
	for siteID, nzb := range e.cfg.CategoryOverrides {
		c := indexers.Category(nzb)
		siteToNzb[siteID] = []indexers.Category{c}
		nzbToSite[c] = append(nzbToSite[c], siteID)
		allCats[c] = true
	}
	cats := make([]indexers.Category, 0, len(allCats))
	for c := range allCats {
		cats = append(cats, c)
	}
	e.cats = cats
	e.siteToNzb = siteToNzb
	e.nzbToSite = nzbToSite
}

// mapNewznabToSite expands a list of Newznab category IDs into the
// site's per-tracker IDs, preserving order. Categories that have no
// mapping are silently skipped: surfacing them as upstream errors
// would block the entire search.
func (e *Engine) mapNewznabToSite(nzbs []indexers.Category) []string {
	if len(nzbs) == 0 {
		return nil
	}
	e.ensureCategories()
	e.catMu.RLock()
	defer e.catMu.RUnlock()
	out := []string{}
	seen := map[string]bool{}
	for _, c := range nzbs {
		for _, id := range e.nzbToSite[c] {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// mapSiteCategory maps a single tracker category id back to Newznab
// category ids. Used when projecting search results.
func (e *Engine) mapSiteCategory(siteID string) []indexers.Category {
	e.ensureCategories()
	e.catMu.RLock()
	defer e.catMu.RUnlock()
	if cs, ok := e.siteToNzb[siteID]; ok {
		out := make([]indexers.Category, len(cs))
		copy(out, cs)
		return out
	}
	return nil
}

// collectModes flattens the `modes:` map into the slice shape the
// indexers.Caps API expects.
func collectModes(modes map[string][]string) []string {
	out := make([]string, 0, len(modes))
	for m := range modes {
		out = append(out, m)
	}
	return out
}

// collectSupportedIDs gathers ID parameter names ("imdbid", "tvdbid",
// …) across every mode the definition advertises. Empty slice when
// the definition only supports free-text search.
func collectSupportedIDs(modes map[string][]string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, params := range modes {
		for _, p := range params {
			p = strings.TrimSpace(p)
			if p == "" || p == "q" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// parseInt is the small helper used by the engine; matches the
// newznab package convention.
func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// sizeRegexp matches "1.23 GB", "12345 bytes", "12 MiB". Numbers may
// use comma thousand separators (Cardigann's `replace` filter handles
// the common cases but a fall-back here keeps the test surface
// small).
var sizeRegexp = regexp.MustCompile(`(?i)([0-9.,]+)\s*(KB|MB|GB|TB|KIB|MIB|GIB|TIB|B|BYTES)?`)

// parseSize converts a human "1.2 GB" string into bytes. Returns 0
// when the input is not parseable.
func parseSize(s string) int64 {
	m := sizeRegexp.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) == 0 {
		return 0
	}
	num := strings.ReplaceAll(m[1], ",", "")
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(m[2]) {
	case "KB", "KIB":
		v *= 1024
	case "MB", "MIB":
		v *= 1024 * 1024
	case "GB", "GIB":
		v *= 1024 * 1024 * 1024
	case "TB", "TIB":
		v *= 1024 * 1024 * 1024 * 1024
	}
	return int64(v)
}

// dateLayouts is the small set of date formats observed across the
// public-tracker definitions used for testing. Cardigann itself
// supports a much larger set (and a custom parser); we accept
// reasonable defaults and leave per-tracker overrides to the
// dateparse filter.
var dateLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"02-01-2006 15:04:05",
	"02/01/2006 15:04:05",
}

// parseDateBestEffort tries each known layout in order. Returns the
// zero time when nothing matches.
func parseDateBestEffort(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
