package cardigann

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/cloudflare"
)

const (
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	defaultTimeout   = 120 * time.Second
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
	catMu     sync.RWMutex
	cats      []indexers.Category
	siteToNzb map[string][]indexers.Category // tracker id → newznab category ids
	nzbToSite map[indexers.Category][]string // newznab id → tracker ids

	// tmplCache caches parsed Go templates keyed by their raw string.
	tmplCache sync.Map // string → *template.Template

	// Cached result of configFieldsWithDefaults, computed once per engine.
	cachedConfigFields map[string]string
	cachedConfigOnce   sync.Once
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
		if indexers.IsTimeoutErr(err) {
			return fmt.Errorf("cardigann: test request: %w (%v)", indexers.ErrIndexerTimeout, err)
		}
		return fmt.Errorf("cardigann: test request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cardigann: test got status %d", resp.StatusCode)
	}
	return nil
}

// configFieldsWithDefaults merges operator-supplied config fields with
// default values from the YAML definition's settings block. Operator
// values always win; defaults fill gaps so templates like
// `{{ .Config.apiurl }}` resolve even without explicit user config.
//
// The result is cached for the lifetime of this Engine instance;
// callers that need updated config must construct a new Engine.
func (e *Engine) configFieldsWithDefaults() map[string]string {
	e.cachedConfigOnce.Do(func() {
		merged := e.cfg.fields()
		for _, s := range e.def.Settings {
			if s.Default != "" {
				if _, ok := merged[s.Name]; !ok {
					merged[s.Name] = s.Default
				}
			}
		}
		e.cachedConfigFields = merged
	})
	return e.cachedConfigFields
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
		if err := e.getLogin(ctx); err != nil {
			return err
		}
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
		v, terr := e.expandTemplate(tmpl, templateContext{Config: e.configFieldsWithDefaults(), True: "true", False: "false"})
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

// getLogin performs a GET-based login. The login URL is templated with
// the operator's config (e.g. passkey, apikey) so the server sets
// session cookies on success. This is used by older definitions where
// the login is a single URL with query parameters.
func (e *Engine) getLogin(ctx context.Context) error {
	loginPath := e.def.Login.Path
	// Expand template variables in the path (e.g. {{ .Config.passkey }}).
	tctx := templateContext{Config: e.configFieldsWithDefaults(), True: "true", False: "false"}
	expandedPath, terr := e.expandTemplate(loginPath, tctx)
	if terr != nil {
		return fmt.Errorf("cardigann: get login path template: %w", terr)
	}
	loginURL, err := e.resolveURL(expandedPath)
	if err != nil {
		return err
	}
	// Build URL with any login inputs as query parameters.
	if len(e.def.Login.Inputs) > 0 {
		u, perr := url.Parse(loginURL)
		if perr != nil {
			return fmt.Errorf("cardigann: parse login url: %w", perr)
		}
		q := u.Query()
		for k, tmpl := range e.def.Login.Inputs {
			v, terr2 := e.expandTemplate(tmpl, tctx)
			if terr2 != nil {
				return fmt.Errorf("cardigann: get login input %q: %w", k, terr2)
			}
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		loginURL = u.String()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return fmt.Errorf("cardigann: build get login request: %w", err)
	}
	req.Header.Set("User-Agent", e.cfg.UserAgent)
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("cardigann: get login request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cardigann: get login got status %d", resp.StatusCode)
	}
	if msg := matchErrorSelectors(body, e.def.Login.Error); msg != "" {
		return fmt.Errorf("cardigann: get login rejected: %s", msg)
	}
	return nil
}

// verifyLogin fetches the test path and asserts the success selector
// matches. When the definition omits the test block entirely we log a
// warning and trust the formLogin status check. When the test path is
// set but the selector is empty, we still fetch and check for HTTP
// errors and login error selectors.
func (e *Engine) verifyLogin(ctx context.Context) error {
	if e.def.Login.Test.Path == "" {
		slog.Warn("cardigann: login verification skipped — no test path defined",
			"indexer", e.id)
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
	// If a success selector is defined, require it to match.
	if e.def.Login.Test.Selector != "" {
		doc, parseErr := goquery.NewDocumentFromReader(bytes.NewReader(body))
		if parseErr != nil {
			return fmt.Errorf("cardigann: verify login parse: %w", parseErr)
		}
		if doc.Find(e.def.Login.Test.Selector).Length() == 0 {
			return errors.New("cardigann: login test selector did not match — credentials are probably wrong")
		}
		return nil
	}
	// No selector defined — check for login error markers as a fallback.
	if msg := matchErrorSelectors(body, e.def.Login.Error); msg != "" {
		return fmt.Errorf("cardigann: login verification failed (error selector matched): %s", msg)
	}
	slog.Warn("cardigann: login test path set but no selector — verification is weak",
		"indexer", e.id)
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

// Search implements indexers.Indexer.  When the definition declares
// multiple search paths (common for sites with separate movie/TV/music
// endpoints) the engine issues one HTTP request per path and merges
// the results, matching Prowlarr's behaviour.
func (e *Engine) Search(ctx context.Context, q indexers.Query) (*indexers.Results, error) {
	slog.Debug("cardigann: search start", "indexer", e.id, "term", q.Term, "imdb", q.IMDBID)
	if err := e.ensureLoggedIn(ctx); err != nil {
		return nil, err
	}

	paths := e.searchPaths()

	// Pre-compute shared values once for all paths.
	keywords := indexers.SanitizeTerm(q.Term, q.Season > 0 || q.Episode > 0)
	if len(e.def.Search.KeywordsFilters) > 0 {
		keywords = applyFilters(keywords, e.def.Search.KeywordsFilters)
		slog.Debug("cardigann: keywords filtered", "indexer", e.id, "raw", q.Term, "filtered", keywords, "filterCount", len(e.def.Search.KeywordsFilters))
	}
	configFields := e.configFieldsWithDefaults()
	categories := e.mapNewznabToSite(q.Categories)
	tctx := newTemplateContext(q, keywords, categories, configFields)

	var allRows []indexers.Result
	var pathErrors []error
	seen := map[string]struct{}{} // dedup by method + resolved URL
	pathsOK := 0

	for i, sp := range paths {
		method, target, params, headers, err := e.buildSearchRequestForPath(sp, tctx)
		if err != nil {
			slog.Warn("cardigann: search path error", "indexer", e.id, "path_idx", i, "err", err)
			pathErrors = append(pathErrors, fmt.Errorf("path %d build: %w", i, err))
			continue
		}

		// For JSON endpoints, prefer application/json Accept header
		// so the server is more likely to return JSON instead of HTML.
		if strings.EqualFold(sp.Response.Type, "json") {
			if headers.Get("Accept") == "" {
				headers.Set("Accept", "application/json")
			}
		}

		// Deduplicate: skip if this resolved request was already fetched.
		dedupKey := method + " " + target
		if len(params) > 0 {
			dedupKey += "?" + params.Encode()
		}
		if _, dup := seen[dedupKey]; dup {
			slog.Debug("cardigann: skipping duplicate URL", "indexer", e.id, "url", dedupKey)
			continue
		}
		seen[dedupKey] = struct{}{}

		if dl, ok := ctx.Deadline(); ok {
			slog.Debug("cardigann: search context", "indexer", e.id, "path_idx", i, "deadline_in", time.Until(dl).String())
		}
		slog.Debug("cardigann: search request", "indexer", e.id, "path_idx", i, "method", method, "target", target, "params", params.Encode())

		body, err := e.fetch(ctx, method, target, params, headers)
		if err != nil {
			slog.Warn("cardigann: search fetch error", "indexer", e.id, "path_idx", i, "err", err)
			pathErrors = append(pathErrors, fmt.Errorf("path %d fetch: %w", i, err))
			continue
		}
		slog.Debug("cardigann: search response", "indexer", e.id, "path_idx", i, "bodyLen", len(body))

		var rows []indexers.Result
		if strings.EqualFold(sp.Response.Type, "json") {
			// Guard: if the body looks like HTML/XML instead of JSON,
			// the site returned an error page or Cloudflare challenge.
			if looksLikeMarkup(body) {
				errMsg := fmt.Errorf("path %d: expected JSON but got HTML/XML response (possible Cloudflare challenge or error page)", i)
				slog.Warn("cardigann: response type mismatch", "indexer", e.id, "path_idx", i, "bodyPrefix", string(body[:min(200, len(body))]))
				pathErrors = append(pathErrors, errMsg)
				continue
			}
			rows, err = e.extractRowsJSON(body, tctx)
		} else {
			rows, err = e.extractRows(body, tctx)
		}
		if err != nil {
			slog.Warn("cardigann: search extract error", "indexer", e.id, "path_idx", i, "err", err)
			pathErrors = append(pathErrors, fmt.Errorf("path %d extract: %w", i, err))
			continue
		}
		slog.Debug("cardigann: search results", "indexer", e.id, "path_idx", i, "rowCount", len(rows))
		allRows = append(allRows, rows...)
		pathsOK++
	}

	// When every path failed, surface the errors so the registry marks
	// this indexer as "error" instead of silently reporting 0 results.
	if pathsOK == 0 && len(pathErrors) > 0 {
		return nil, fmt.Errorf("cardigann[%s]: all %d search paths failed: %w",
			e.id, len(paths), errors.Join(pathErrors...))
	}

	return &indexers.Results{
		IndexerID: e.id,
		Items:     allRows,
		Total:     len(allRows),
	}, nil
}

// searchPaths returns the ordered list of search endpoints. If the
// definition uses the single-path shorthand (`search.path`) it is
// wrapped in a one-element slice for uniform handling.
func (e *Engine) searchPaths() []SearchPath {
	if len(e.def.Search.Paths) > 0 {
		return append([]SearchPath(nil), e.def.Search.Paths...)
	}
	return []SearchPath{{
		Path:   e.def.Search.Path,
		Method: e.def.Search.Method,
	}}
}

// buildSearchRequestForPath assembles the URL, method, query parameters
// and headers for a single search path using the pre-built template
// context.
func (e *Engine) buildSearchRequestForPath(sp SearchPath, tctx templateContext) (method, target string, params url.Values, headers http.Header, err error) {
	path := sp.Path
	method = strings.ToUpper(sp.Method)
	if method == "" {
		method = strings.ToUpper(e.def.Search.Method)
	}
	if method == "" {
		method = http.MethodGet
	}

	// Expand Go templates in the path (e.g. "search/{{ .Keywords }}")
	// before resolving against the base URL.
	if strings.Contains(path, "{{") {
		path, err = e.expandTemplate(path, tctx)
		if err != nil {
			return "", "", nil, nil, fmt.Errorf("cardigann: search path template: %w", err)
		}
	}

	target, err = e.resolveURL(path)
	if err != nil {
		return "", "", nil, nil, err
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
	// Definitions embed keywords directly in the path/query (e.g.
	// "q.php?q={{ .Keywords }}" or "search/{{ .Keywords }}"). A
	// multi-word term leaves literal spaces in the URL, which Go sends
	// verbatim in the request line — an invalid request-target that
	// Cloudflare answers with a 403 (then wrongly triggers a
	// FlareSolverr solve). .NET (Jackett/Prowlarr) percent-encodes these
	// automatically; replicate that for spaces, the dominant case.
	full = strings.ReplaceAll(full, " ", "%20")
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
	// Set default Accept only if caller didn't provide one.
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/html, application/xhtml+xml, */*")
	}
	resp, derr := e.http.Do(req)
	if derr != nil {
		// Wrap timeout errors with the package-level sentinel so the
		// service layer can classify uniformly.
		if indexers.IsTimeoutErr(derr) {
			return nil, fmt.Errorf("cardigann: %s %s: %w (%v)", method, target, indexers.ErrIndexerTimeout, derr)
		}
		return nil, fmt.Errorf("cardigann: %s %s: %w", method, target, derr)
	}
	defer resp.Body.Close()
	slog.Debug("cardigann: fetch response", "url", full, "status", resp.StatusCode, "contentType", resp.Header.Get("Content-Type"))
	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return nil, fmt.Errorf("cardigann: read body: %w", rerr)
	}
	// CloudFlare challenge detection — if we reach here, the
	// RoundTripper's PostResponse solve either wasn't triggered
	// (no flaresolverr proxy) or it failed. Report the error.
	if cloudflare.IsChallenge(resp, body) {
		slog.Warn("cardigann: cloudflare challenge detected (unsolved)", "indexer", e.id, "url", full)
		return nil, fmt.Errorf("cardigann: %s %s: %w", method, target, indexers.ErrCloudFlareChallenge)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("cardigann: upstream status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("cardigann: rate limited (status 429): %w", indexers.ErrIndexerRateLimited)
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

// looksLikeMarkup returns true if the body appears to be HTML/XML
// rather than JSON. Used to detect Cloudflare challenge pages, error
// pages, or other non-JSON responses from indexers that should return JSON.
func looksLikeMarkup(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	// Skip leading whitespace/BOM.
	trimmed := bytes.TrimLeft(body, " \t\r\n\xef\xbb\xbf")
	if len(trimmed) == 0 {
		return false
	}
	// JSON always starts with '{', '[', or '"'. Anything starting with '<' is markup.
	if trimmed[0] == '<' {
		return true
	}
	return false
}

// extractRows parses the response HTML, runs the rows selector, and
// applies every field selector against each row to produce one
// indexers.Result per match.
//
// The function is the heart of the engine and intentionally chatty
// in its commentary: tracing a missing field through a stack of
// CSS/XPath selectors is the most common debugging task operators
// face, and the comments here map straight to that workflow.
func (e *Engine) extractRows(body []byte, tctx templateContext) ([]indexers.Result, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cardigann: parse search html: %w", err)
	}
	rowSelector := e.def.Search.Rows.Selector

	// Many definitions embed Go templates inside the row selector
	// (e.g. 1337x uses `{{ if .Config.uploader }}...{{ end }}`).
	// Expand them before passing to the CSS engine.
	if strings.Contains(rowSelector, "{{") {
		expanded, terr := e.expandTemplate(rowSelector, tctx)
		if terr != nil {
			return nil, fmt.Errorf("cardigann: row selector template: %w", terr)
		}
		rowSelector = expanded
	}

	rows := selectNodes(doc.Selection, rowSelector)
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		tables := doc.Find("table").Length()
		forumTables := doc.Find("table.forum_header_border").Length()
		trHover := doc.Find("tr[name='hover']").Length()
		magnets := doc.Find("a.magnet").Length()
		slog.Debug("cardigann: extractRows", "indexer", e.id, "rowSelector", rowSelector, "matchedRows", len(rows),
			"tables", tables, "forum_tables", forumTables, "tr_hover", trHover, "magnets", magnets)
	}

	// NoResultsMessage: when the definition declares a "no results"
	// string and the page contains it, return an empty result set
	// instead of trying to parse rows (which would yield 0 results
	// or spurious errors).
	if msg := e.def.Search.Rows.NoResultsMessage; msg != "" && len(rows) == 0 {
		if strings.Contains(string(body), msg) {
			slog.Debug("cardigann: noResultsMessage matched", "indexer", e.id, "message", msg)
			return nil, nil
		}
	}

	// Cardigann's `after:` strips a header row; we honour positive
	// values only because negatives have no upstream semantics.
	if e.def.Search.Rows.After > 0 && len(rows) > e.def.Search.Rows.After {
		rows = rows[e.def.Search.Rows.After:]
	}

	out := make([]indexers.Result, 0, len(rows))
	for _, node := range rows {
		r, ok := e.extractOne(node, tctx)
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
//
// Processing uses a fixed-point loop to support computed fields that
// can depend on other computed fields (chained `.Result.*` references):
//
//  1. Extract all fields that have a CSS/XPath `selector` (after
//     expanding any templates in the selector itself).
//  2. Repeatedly expand `text`-only fields until values stabilise,
//     handling chained dependencies like title → title_phase2 → title.
//  3. Apply `default` fallbacks for selector fields that matched nothing.
func (e *Engine) extractOne(node *goquery.Selection, tctx templateContext) (indexers.Result, bool) {
	r := indexers.Result{}
	values := map[string]string{}

	fieldCtx := tctx
	fieldCtx.Result = values

	// Sort field names once for deterministic iteration across all phases.
	fieldNames := sortedFieldNames(e.def.Search.Fields)

	// Phase 1 — selector-based extraction (raw HTML scraping).
	// Expand any templates in the selector itself first.
	for _, name := range fieldNames {
		field := e.def.Search.Fields[name]
		if field.Selector == "" {
			continue
		}
		sel := field.Selector
		if strings.Contains(sel, "{{") {
			var err error
			sel, err = e.expandTemplate(sel, fieldCtx)
			if err != nil {
				slog.Warn("cardigann: field selector template error",
					"indexer", e.id, "field", name, "err", err)
				continue
			}
		}
		v := e.applyFieldWithSelector(node, field, sel, fieldCtx)
		values[name] = v
	}

	// Phase 2 — text-template fields via fixed-point iteration.
	// Chained fields (e.g. title depends on title_phase2 which depends
	// on title_phase1) resolve across iterations.  We cap iterations
	// at the number of text fields to prevent infinite loops from cycles.
	maxPasses := 0
	for _, name := range fieldNames {
		field := e.def.Search.Fields[name]
		if field.Selector == "" && field.Text != "" {
			maxPasses++
		}
	}
	for pass := 0; pass <= maxPasses; pass++ {
		changed := false
		for _, name := range fieldNames {
			field := e.def.Search.Fields[name]
			if field.Selector != "" || field.Text == "" {
				continue
			}
			expanded := field.Text
			if strings.Contains(expanded, "{{") {
				var terr error
				expanded, terr = e.expandTemplate(expanded, fieldCtx)
				if terr != nil {
					slog.Warn("cardigann: field text template error",
						"indexer", e.id, "field", name, "err", terr)
					continue
				}
			}
			next := applyFilters(expanded, e.expandFilterArgs(field.Filters, fieldCtx))
			if values[name] != next {
				values[name] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Warn if fixed-point iteration exhausted all passes without converging.
	{
		countUnresolved := 0
		for _, name := range fieldNames {
			field := e.def.Search.Fields[name]
			if field.Selector != "" || field.Text == "" {
				continue
			}
			expanded := field.Text
			if strings.Contains(expanded, "{{") {
				var terr error
				expanded, terr = e.expandTemplate(expanded, fieldCtx)
				if terr != nil {
					continue
				}
			}
			next := applyFilters(expanded, e.expandFilterArgs(field.Filters, fieldCtx))
			if values[name] != next {
				countUnresolved++
			}
		}
		if countUnresolved > 0 {
			slog.Warn("cardigann: fixed-point iteration did not converge — possible cycle in computed fields",
				"indexer", e.id,
				"passes", maxPasses,
				"remaining_unresolved", countUnresolved)
		}
	}

	// Phase 3 — apply `default` fallbacks for selector fields that
	// matched nothing (empty string).
	for _, name := range fieldNames {
		field := e.def.Search.Fields[name]
		if field.Default == "" || values[name] != "" {
			continue
		}
		fallback := field.Default
		if strings.Contains(fallback, "{{") {
			var terr error
			fallback, terr = e.expandTemplate(fallback, fieldCtx)
			if terr != nil {
				slog.Warn("cardigann: field default template error",
					"indexer", e.id, "field", name, "err", terr)
				continue
			}
		}
		values[name] = applyFilters(fallback, e.expandFilterArgs(field.Filters, fieldCtx))
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

// applyFieldWithSelector runs one field's selector + filter pipeline
// selector (templates already resolved).
func (e *Engine) applyFieldWithSelector(row *goquery.Selection, f Field, sel string, ctx templateContext) string {
	filters := e.expandFilterArgs(f.Filters, ctx)
	if f.Text != "" && sel == "" {
		return applyFilters(f.Text, filters)
	}
	if sel == "" {
		return ""
	}
	nodes := selectNodes(row, sel)
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
	return applyFilters(raw, filters)
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

// templateQuery mirrors the Prowlarr/Jackett `.Query` sub-object that
// many Cardigann definitions reference (e.g. `{{ .Query.IMDBID }}`).
type templateQuery struct {
	Keywords    string
	IMDBID      string
	IMDBIDShort string // numeric part without "tt" prefix
	TVDBID      string
	TMDBID      string
	TVMazeID    string
	DoubanID    string
	Season      int
	Ep          int // alias used by most definitions
	Episode     int
	Year        string
	Genre       string
	Artist      string
	Album       string
	Author      string
	Title       string
	Type        string
}

// templateContext is the set of variables Cardigann templates can
// reference.  Top-level fields (IMDBID, Season, etc.) exist for
// backward compatibility with `{{ .IMDBID }}`; the Query sub-struct
// provides the same data under `{{ .Query.IMDBID }}`.  Both must be
// populated identically.
type templateContext struct {
	Keywords   string
	Query      templateQuery
	Categories []string
	IMDBID     string
	TVDBID     string
	TMDBID     string
	Season     int
	Episode    int
	Config     map[string]string

	// True / False are boolean constants that many Cardigann
	// definitions use in conditional expressions like
	// `{{ if eq .Config.disablesort .False }}`.
	True  string
	False string

	// Result holds field values extracted in the first pass of row
	// processing.  Text-template fields like
	//   `{{ if .Result.title_optional }}{{ .Result.title_optional }}{{ else }}{{ .Result.title_default }}{{ end }}`
	// reference these to compose computed values from other fields.
	Result map[string]string
}

// newTemplateContext builds a populated templateContext from an
// indexers.Query plus pre-computed keywords, categories, and config.
// Centralising construction avoids the fragile 20-field struct literal
// in every call site.
func newTemplateContext(q indexers.Query, keywords string, categories []string, config map[string]string) templateContext {
	// Top-level IMDBID is the short numeric form (no "tt" prefix) for
	// legacy Cardigann templates; Query.IMDBID retains the full "ttXXX" form.
	imdbShort := strings.TrimPrefix(q.IMDBID, "tt")
	return templateContext{
		Keywords: keywords,
		Query: templateQuery{
			Keywords:    keywords,
			IMDBID:      q.IMDBID,
			IMDBIDShort: imdbShort,
			TVDBID:      q.TVDBID,
			TMDBID:      q.TMDBID,
			Season:      q.Season,
			Ep:          q.Episode,
			Episode:     q.Episode,
		},
		Categories: categories,
		IMDBID:     imdbShort,
		TVDBID:     q.TVDBID,
		TMDBID:     q.TMDBID,
		Season:     q.Season,
		Episode:    q.Episode,
		Config:     config,
		True:       "true",
		False:      "false",
	}
}

// reCache caches compiled regular expressions used by re_replace.
// Patterns come from static YAML definitions and are reused across
// searches, so compiling once avoids redundant work.
var reCache sync.Map // string → *regexp.Regexp

// cardigannFuncs is the static FuncMap shared by all template
// expansions.  Hoisted to package level so it is allocated once.
var cardigannFuncs = template.FuncMap{
	"join": strings.Join,
	"replace": func(s, old, newStr string) string {
		return strings.ReplaceAll(s, old, newStr)
	},
	"re_replace": func(s, pattern, replacement string) string {
		cached, ok := reCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				slog.Warn("cardigann: re_replace: bad pattern",
					"pattern", pattern, "err", err)
				return s
			}
			cached, _ = reCache.LoadOrStore(pattern, compiled)
		}
		return cached.(*regexp.Regexp).ReplaceAllString(s, replacement)
	},
	"urlencode": url.QueryEscape,
}

// bufPool recycles bytes.Buffers used by expandTemplate to reduce
// allocation pressure during row-heavy searches.
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// expandTemplate renders tmpl with ctx using text/template. The
// "missingkey=zero" option ensures that missing map/struct fields
// render as empty strings rather than Go's default "<no value>",
// matching Prowlarr/Jackett behaviour.
func (e *Engine) expandTemplate(tmpl string, ctx templateContext) (string, error) {
	if tmpl == "" {
		return "", nil
	}
	// Check the per-engine template cache first.
	cached, ok := e.tmplCache.Load(tmpl)
	if !ok {
		parsed, err := template.New("cardigann").
			Option("missingkey=zero").
			Funcs(cardigannFuncs).
			Parse(tmpl)
		if err != nil {
			return "", fmt.Errorf("parse template: %w", err)
		}
		cached, _ = e.tmplCache.LoadOrStore(tmpl, parsed)
	}
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	if err := cached.(*template.Template).Execute(buf, ctx); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// expandFilterArgs returns a copy of filters with any Go template
// syntax in the Args expanded against ctx.  This is needed because
// Cardigann definitions can embed `.Result.*` references inside
// filter arguments (e.g. YTS uses `append` with
// `{{ .Result._quality }}`).  Without expansion the raw template
// string ends up in the output.
func (e *Engine) expandFilterArgs(filters []Filter, ctx templateContext) []Filter {
	if len(filters) == 0 {
		return filters
	}
	out := make([]Filter, len(filters))
	for i, f := range filters {
		out[i] = f
		out[i].Args = e.expandFilterArg(f.Args, ctx)
	}
	return out
}

func (e *Engine) expandFilterArg(arg any, ctx templateContext) any {
	switch v := arg.(type) {
	case string:
		if !strings.Contains(v, "{{") {
			return v
		}
		expanded, err := e.expandTemplate(v, ctx)
		if err != nil {
			slog.Warn("cardigann: filter arg template error",
				"indexer", e.id, "err", err)
			return v
		}
		return expanded
	case []any:
		cp := make([]any, len(v))
		for i, elem := range v {
			cp[i] = e.expandFilterArg(elem, ctx)
		}
		return cp
	case []string:
		cp := make([]any, len(v))
		for i, elem := range v {
			cp[i] = e.expandFilterArg(elem, ctx)
		}
		return cp
	default:
		return arg
	}
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

// sortedFieldNames returns the keys of a field map in sorted order
// so that extraction phases iterate deterministically.
func sortedFieldNames(fields map[string]Field) []string {
	names := make([]string, 0, len(fields))
	for n := range fields {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// parseInt is the small helper used by the engine; matches the
// newznab package convention.
// parseInt parses a human-readable integer string. It strips commas,
// dots used as thousand separators, and other non-digit noise so
// values like "1,234" or "1.234" (European thousand separator)
// parse correctly instead of silently returning 0.
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Fast path: pure digits (optionally with a leading minus).
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	// Strip thousand separators (commas and dots that aren't decimal).
	// Heuristic: if the string contains both '.' and ',' treat the
	// last one as decimal separator and others as thousand separators.
	// For integers we strip everything non-digit except a leading '-'.
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '-' {
			return r
		}
		return -1 // drop commas, dots, spaces, etc.
	}, s)
	n, _ := strconv.Atoi(cleaned)
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
