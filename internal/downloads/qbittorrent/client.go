package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind is the registry key under which this implementation registers
// itself with the downloads core.
const Kind = downloads.KindQBittorrent

// defaultUserAgent is sent on every request. qBittorrent does not
// require a particular UA but a stable identifier helps when tracing
// traffic in operator logs.
const defaultUserAgent = "Loom/0.1 (+https://loom.dev) qbittorrent-client"

// defaultTimeout is the per-request HTTP timeout when the
// transport stack does not impose one of its own.
const defaultTimeout = 30 * time.Second

// Config is the JSON shape persisted in download_clients.config for
// qBittorrent rows. Host/Port/TLS/Username/Password may also live on
// the parent Definition; values on Config win when both are present
// so operators can route everything through `config` if they prefer.
type Config struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	BasePath string `json:"base_path,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// resolved merges a Config with the surrounding Definition so the
// client has a single struct of effective values.
type resolved struct {
	baseURL  *url.URL
	username string
	password string
}

// loginGracePeriod is how long after a successful login we consider
// the session still valid. Concurrent 403 handlers that arrive within
// this window skip the redundant login and go straight to retry.
const loginGracePeriod = 5 * time.Second

// loginBackoff is the minimum interval between consecutive login
// attempts (successful or failed). This prevents a tight retry loop
// from flooding the qBittorrent login endpoint and triggering an IP ban.
const loginBackoff = 3 * time.Second

// Client is a live qBittorrent v2 Web API client. One Client per
// persisted download_clients row. Methods are safe for concurrent
// use; the cookie jar is shared with the underlying http.Client.
type Client struct {
	id       string
	name     string
	protocol downloads.Protocol

	cfg resolved

	http *http.Client

	// loginMu serialises authentication so a flood of concurrent
	// 403s does not fan out into N parallel re-logins.
	loginMu sync.Mutex
	// lastLoginAt records when the last successful login completed.
	// Protected by loginMu.
	lastLoginAt time.Time
	// lastLoginAttempt records when the last login attempt (success or
	// fail) was made so we can enforce a backoff. Protected by loginMu.
	lastLoginAttempt time.Time
}

// New is the production constructor. It composes the Loom transport
// stack (proxy + throttle) just like indexer kinds do. Tests should
// prefer NewWithHTTPClient to inject a transport that points at an
// httptest.Server.
func New(def downloads.Definition) (*Client, error) {
	cfg, err := parseConfig(def)
	if err != nil {
		return nil, err
	}
	rt, err := downloads.TransportForDefinition(def)
	if err != nil || rt == nil {
		rt = http.DefaultTransport
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("qbittorrent: building cookie jar: %w", err)
	}
	httpc := &http.Client{
		Transport: rt,
		Timeout:   defaultTimeout,
		Jar:       jar,
	}
	return NewWithHTTPClient(def, cfg, httpc)
}

// NewWithHTTPClient builds a Client that uses the supplied
// *http.Client as-is. Callers must ensure the http.Client has a
// non-nil cookie jar; auth state lives in the jar. This entry point
// exists for tests and for operators who want to supply a fully
// custom transport (e.g. tracing-instrumented).
func NewWithHTTPClient(def downloads.Definition, cfg resolved, httpc *http.Client) (*Client, error) {
	if httpc == nil {
		return nil, errors.New("qbittorrent: NewWithHTTPClient requires a non-nil http.Client")
	}
	if httpc.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("qbittorrent: building cookie jar: %w", err)
		}
		httpc.Jar = jar
	}
	proto := def.Protocol
	if proto == "" {
		proto = downloads.ProtocolTorrent
	}
	return &Client{
		id:       def.ID,
		name:     def.Name,
		protocol: proto,
		cfg:      cfg,
		http:     httpc,
	}, nil
}

// ID implements downloads.DownloadClient.
func (c *Client) ID() string { return c.id }

// Name implements downloads.DownloadClient.
func (c *Client) Name() string { return c.name }

// Kind implements downloads.DownloadClient.
func (c *Client) Kind() downloads.Kind { return Kind }

// Protocol implements downloads.DownloadClient. qBittorrent is
// torrent-only.
func (c *Client) Protocol() downloads.Protocol { return c.protocol }

// parseConfig pulls the qBittorrent-specific fields off the
// Definition. Config-blob values override top-level fields so that
// operators can drive everything through config if they prefer.
func parseConfig(def downloads.Definition) (resolved, error) {
	var cfg Config
	if len(def.Config) > 0 {
		if err := json.Unmarshal(def.Config, &cfg); err != nil {
			return resolved{}, fmt.Errorf("qbittorrent: parsing config blob: %w", err)
		}
	}
	host := firstNonEmpty(cfg.Host, def.Host)
	port := cfg.Port
	if port == 0 {
		port = def.Port
	}
	tls := cfg.TLS || def.TLS
	username := firstNonEmpty(cfg.Username, def.Username)
	password := firstNonEmpty(cfg.Password, def.Password)

	if host == "" {
		return resolved{}, fmt.Errorf("%w: host is required", ErrNotConfigured)
	}

	scheme := "http"
	if tls {
		scheme = "https"
	}
	hostport := host
	defaultPort := 80
	if tls {
		defaultPort = 443
	}
	if port != 0 && port != defaultPort {
		hostport = fmt.Sprintf("%s:%d", host, port)
	}
	basePath := cfg.BasePath
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	raw := fmt.Sprintf("%s://%s%s", scheme, hostport, basePath)
	u, err := url.Parse(raw)
	if err != nil {
		return resolved{}, fmt.Errorf("qbittorrent: assembling base URL %q: %w", raw, err)
	}
	return resolved{
		baseURL:  u,
		username: username,
		password: password,
	}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// endpoint resolves a v2 API path (e.g. "torrents/info") against the
// configured base URL, including any WebUI subpath the operator
// configured on the qBittorrent side.
func (c *Client) endpoint(apiPath string) string {
	apiPath = strings.TrimPrefix(apiPath, "/")
	rel, _ := url.Parse("api/v2/" + apiPath)
	return c.cfg.baseURL.ResolveReference(rel).String()
}

// login performs the cookie-based handshake with /api/v2/auth/login.
// On success the SID cookie is stored in the client's jar. Repeat
// calls are safe; the most recent cookie wins.
//
// qBittorrent enforces a Referer matching the host header on the
// login form, so we set it to the configured base URL. Some
// reverse-proxied installs also require Origin; we set both.
//
// The force parameter controls whether to skip the grace-period
// check. Normal re-login from do() passes false; explicit Test()
// calls pass true because Test() exists specifically to verify
// credentials against the server.
func (c *Client) login(ctx context.Context, force bool) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	now := time.Now()

	// If a recent login already succeeded and this isn't a forced
	// re-check, skip the round-trip. This collapses N concurrent
	// 403 retries into a single login.
	if !force && !c.lastLoginAt.IsZero() && now.Sub(c.lastLoginAt) < loginGracePeriod {
		return nil
	}

	// Enforce backoff between login attempts to avoid triggering
	// qBittorrent's IP ban on rapid-fire requests.
	if !c.lastLoginAttempt.IsZero() {
		wait := loginBackoff - now.Sub(c.lastLoginAttempt)
		if wait > 0 {
			c.loginMu.Unlock()
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				c.loginMu.Lock()
				return ctx.Err()
			}
			c.loginMu.Lock()
			// Re-check: another goroutine may have logged in while we waited.
			if !force && !c.lastLoginAt.IsZero() && time.Since(c.lastLoginAt) < loginGracePeriod {
				return nil
			}
		}
	}

	c.lastLoginAttempt = time.Now()

	form := url.Values{}
	form.Set("username", c.cfg.username)
	form.Set("password", c.cfg.password)

	endpoint := c.endpoint("auth/login")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("qbittorrent: building login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.cfg.baseURL.String())
	req.Header.Set("Origin", strings.TrimRight(c.cfg.baseURL.String(), "/"))
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent: login request to %q: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	switch {
	case resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "Ok.":
		c.lastLoginAt = time.Now()
		return nil
	case resp.StatusCode == http.StatusForbidden:
		// qBittorrent returns 403 when the IP has been
		// temporarily banned for too many failed logins.
		return fmt.Errorf("%w: %s", ErrAuthFailed, strings.TrimSpace(string(body)))
	case resp.StatusCode == http.StatusNotFound:
		// 404 means the URL is wrong — probably wrong host, port, or base path.
		return fmt.Errorf("qbittorrent: endpoint not found (HTTP 404) — tried %s — check the host, port, and base path", endpoint)
	default:
		return fmt.Errorf("%w: HTTP %d %s (URL: %s)", ErrAuthFailed, resp.StatusCode, strings.TrimSpace(string(body)), endpoint)
	}
}

// do performs an authenticated request, transparently re-authenticating
// once on a 403 response. The body is fully read and returned to the
// caller; per-endpoint helpers handle decoding.
func (c *Client) do(ctx context.Context, req *http.Request) ([]byte, error) {
	body, status, err := c.roundTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		// Session expired. Record the moment we saw the 403 so the
		// grace-period logic in login() can tell whether the session
		// was invalidated after the last successful login.
		c.invalidateSession()

		// Re-login and retry exactly once.
		if loginErr := c.login(ctx, false); loginErr != nil {
			return nil, loginErr
		}
		retry, err := cloneRequest(req)
		if err != nil {
			return nil, err
		}
		body, status, err = c.roundTrip(ctx, retry)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("%w: %s returned HTTP %d: %s",
			ErrServer, req.URL.Path, status, truncate(string(body), 200))
	}
	return body, nil
}

// invalidateSession clears the last-login timestamp so the next
// login() call won't be short-circuited by the grace period.
func (c *Client) invalidateSession() {
	c.loginMu.Lock()
	c.lastLoginAt = time.Time{}
	c.loginMu.Unlock()
}

// roundTrip executes a single attempt of the request. The body is
// fully drained and the (body, status) pair returned. Network errors
// surface with full context.
func (c *Client) roundTrip(ctx context.Context, req *http.Request) ([]byte, int, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", c.cfg.baseURL.String())
	}
	req = req.WithContext(ctx)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("qbittorrent: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("qbittorrent: reading %s body: %w", req.URL.Path, err)
	}
	return body, resp.StatusCode, nil
}

// cloneRequest produces a copy of req that can be re-sent. The
// request body is preserved when GetBody is set (true for any
// request whose body was attached via http.NewRequest with a
// strings.Reader / bytes.Reader).
func cloneRequest(req *http.Request) (*http.Request, error) {
	cp := req.Clone(req.Context())
	if req.Body != nil && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("qbittorrent: cloning request body: %w", err)
		}
		cp.Body = body
	}
	return cp, nil
}

// truncate clips s to n runes, appending an ellipsis when shortened.
// Used when surfacing server bodies in error messages so a misbehaving
// upstream cannot blow up the log line.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// postForm posts a url-encoded form to api/v2/<path>.
func (c *Client) postForm(ctx context.Context, apiPath string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint(apiPath), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(ctx, req)
}

// get performs a GET against api/v2/<path>?<params>.
func (c *Client) get(ctx context.Context, apiPath string, params url.Values) ([]byte, error) {
	endpoint := c.endpoint(apiPath)
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, req)
}
