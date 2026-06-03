package newznab

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

const (
	defaultUserAgent = "Loom/0.1 (+https://loom.dev)"
	defaultTimeout   = 120 * time.Second
)

// attrFlavour distinguishes which extended-attribute namespace the
// upstream uses. The two protocols share an envelope; only the
// `<*:attr>` element differs.
type attrFlavour int

const (
	flavourNewznab attrFlavour = iota
	flavourTorznab
)

func (f attrFlavour) kind() indexers.Kind {
	if f == flavourTorznab {
		return KindTorznab
	}
	return KindNewznab
}

// Client is the live Indexer for a single Newznab/Torznab endpoint.
//
// One Client is built per persisted indexer row at hydrate time; it
// is safe for concurrent use, with the in-memory caps cache guarded
// by capsMu.
type Client struct {
	id      string
	name    string
	cfg     Config
	http    *http.Client
	persist indexers.CapsCache // optional, may be nil

	capsMu  sync.RWMutex
	capsHit bool
	caps    indexers.Caps
}

// NewClient builds a Client with the given config and HTTP client.
// httpClient may be nil; the constructor will install a default with
// the configured timeout.
func NewClient(id, name string, cfg Config, httpClient *http.Client, persist indexers.CapsCache) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout.duration()}
	}
	c := &Client{
		id:      id,
		name:    name,
		cfg:     cfg,
		http:    httpClient,
		persist: persist,
	}
	c.preloadCachedCaps()
	return c
}

// preloadCachedCaps seeds caps from the DB so the first Search after a
// restart doesn't have to wait on a network round-trip. Failures here
// are logged-by-callers (we silently fall back to lazy fetch).
func (c *Client) preloadCachedCaps() {
	if c.persist == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	caps, ok, err := c.persist.Load(ctx, c.id)
	if err != nil || !ok {
		return
	}
	c.capsMu.Lock()
	c.caps = caps
	c.capsHit = true
	c.capsMu.Unlock()
}

// ID implements indexers.Indexer.
func (c *Client) ID() string { return c.id }

// Name implements indexers.Indexer.
func (c *Client) Name() string { return c.name }

// Caps implements indexers.Indexer. The first call lazy-fetches; later
// calls return the cached snapshot. Errors during lazy fetch surface
// as an empty Caps (callers who need the error should use Test).
func (c *Client) Caps() indexers.Caps {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.Timeout.duration())
	defer cancel()
	return c.capsWithContext(ctx)
}

// capsWithContext returns cached caps when present, otherwise lazy-fetches
// using the supplied context so the caller's deadline (e.g. the
// per-indexer search timeout) governs the network round-trip instead of
// an unbounded background fetch.
func (c *Client) capsWithContext(ctx context.Context) indexers.Caps {
	c.capsMu.RLock()
	if c.capsHit {
		defer c.capsMu.RUnlock()
		return c.caps
	}
	c.capsMu.RUnlock()

	caps, err := c.fetchAndStoreCaps(ctx)
	if err != nil {
		return indexers.Caps{}
	}
	return caps
}

// Test implements indexers.Indexer. It refreshes the caps cache and
// returns nil on success, a typed error on failure.
func (c *Client) Test(ctx context.Context) error {
	_, err := c.fetchAndStoreCaps(ctx)
	return err
}

// fetchAndStoreCaps drives the caps round-trip end-to-end: HTTP GET,
// parse, in-memory cache, optional DB persist.
func (c *Client) fetchAndStoreCaps(ctx context.Context) (indexers.Caps, error) {
	body, err := c.get(ctx, c.buildURL("caps", url.Values{}))
	if err != nil {
		return indexers.Caps{}, err
	}
	caps, err := parseCapsResponse(body)
	if err != nil {
		return indexers.Caps{}, fmt.Errorf("newznab caps from %q: %w", c.cfg.URL, err)
	}
	c.capsMu.Lock()
	c.caps = caps
	c.capsHit = true
	c.capsMu.Unlock()
	if c.persist != nil {
		_ = c.persist.Save(ctx, c.id, caps)
	}
	return caps, nil
}

// buildURL composes the request URL for mode and extra params,
// stamping in apikey from config. Callers pass mode unprefixed
// ("search", "tvsearch", "movie"…); we add `t=`.
func (c *Client) buildURL(mode string, extra url.Values) string {
	if extra == nil {
		extra = url.Values{}
	}
	extra.Set("t", mode)
	extra.Set("apikey", c.cfg.APIKey)
	sep := "?"
	if strings.Contains(c.cfg.URL, "?") {
		sep = "&"
	}
	return c.cfg.URL + sep + extra.Encode()
}

// get performs an HTTP GET with the configured user agent. It maps
// transport-layer failures into typed errors.
func (c *Client) get(ctx context.Context, fullURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("newznab build request: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/xml, text/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %s", ErrTimeout, err.Error())
		}
		return nil, fmt.Errorf("%w: %s", ErrUpstream, err.Error())
	}
	defer resp.Body.Close()

	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return nil, fmt.Errorf("%w: read body: %s", ErrUpstream, rerr.Error())
	}
	// Detect Cloudflare challenge pages before HTTP status classification,
	// so a 403 from Cloudflare is not misclassified as ErrAuthFailed.
	if looksLikeCloudFlare(resp, body) {
		return nil, fmt.Errorf("%w: status %d", ErrCloudFlare, resp.StatusCode)
	}
	if err := classifyHTTP(resp, body); err != nil {
		return nil, err
	}
	if upstream := decodeUpstreamError(body); upstream != nil {
		return nil, upstream
	}
	return body, nil
}

// classifyHTTP turns the response status into the typed error
// taxonomy. Non-2xx is the only case that returns an error.
func classifyHTTP(resp *http.Response, body []byte) error {
	switch {
	case resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: status %d", ErrAuthFailed, resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: status %d", ErrRateLimited, resp.StatusCode)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	case resp.StatusCode >= 400:
		return fmt.Errorf("%w: status %d (body: %s)", ErrUpstream,
			resp.StatusCode, snippet(body))
	}
	return nil
}

// looksLikeCloudFlare detects Cloudflare JS challenge / CAPTCHA pages
// that some indexers sit behind. We check the Server header and body
// markers rather than status code alone, because CF can return 403, 503,
// or other codes.
func looksLikeCloudFlare(resp *http.Response, body []byte) bool {
	server := strings.ToLower(resp.Header.Get("Server"))
	if !strings.Contains(server, "cloudflare") {
		return false
	}
	b := strings.ToLower(string(body))
	return strings.Contains(b, "cf-browser-verification") ||
		strings.Contains(b, "cf_chl_opt") ||
		strings.Contains(b, "jschl-answer") ||
		strings.Contains(b, "challenge-platform")
}

// snippet trims to a manageable size for log/error messages.
func snippet(b []byte) string {
	const max = 160
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// upstreamError matches the Newznab-spec error envelope:
//
//	<error code="100" description="Incorrect user credentials"/>
type upstreamError struct {
	XMLName     xml.Name `xml:"error"`
	Code        string   `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

// decodeUpstreamError returns nil when the body is not an error
// envelope. We only match unambiguous shapes to avoid swallowing
// real responses that happen to contain the word "error".
func decodeUpstreamError(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "<?xml") {
		if idx := strings.Index(trimmed, "?>"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[idx+2:])
		}
	}
	if !strings.HasPrefix(trimmed, "<error") {
		return nil
	}
	var ue upstreamError
	if err := xml.Unmarshal(body, &ue); err != nil {
		return nil
	}
	code := parseInt(ue.Code)
	desc := strings.ToLower(strings.TrimSpace(ue.Description))

	// Detect rate limiting by description text (Sonarr matches these).
	if strings.Contains(desc, "request limit reached") ||
		strings.Contains(desc, "api limit") ||
		strings.Contains(desc, "rate limit") ||
		strings.Contains(desc, "too many requests") {
		return fmt.Errorf("%w: code=%s description=%q",
			ErrRateLimited, ue.Code, ue.Description)
	}

	// Newznab spec: codes 100-199 are all authentication/authorisation errors.
	if code >= 100 && code <= 199 {
		return fmt.Errorf("%w: code=%s description=%q",
			ErrAuthFailed, ue.Code, ue.Description)
	}

	return fmt.Errorf("%w: code=%s description=%q",
		ErrUpstream, ue.Code, ue.Description)
}

// parseInt is a tiny shared helper so search/result code can keep
// happy paths readable.
// parseInt parses a human-readable integer, stripping commas and
// other thousand separators so "1,234" correctly returns 1234.
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return -1
	}, s)
	n, _ := strconv.Atoi(cleaned)
	return n
}

// parseInt64 mirrors parseInt for 64-bit fields (Size).
func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
