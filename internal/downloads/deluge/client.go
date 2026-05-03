package deluge

import (
	"bytes"
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
	"sync/atomic"
	"time"

	"github.com/loomctl/loom/internal/downloads"
)

// Kind is the registry key under which this implementation registers
// itself with the downloads core.
const Kind = downloads.KindDeluge

// defaultUserAgent is sent on every request. Deluge does not require
// a particular UA but a stable identifier helps when tracing traffic
// in operator logs.
const defaultUserAgent = "Loom/0.1 (+https://loom.dev) deluge-client"

// defaultTimeout is the per-request HTTP timeout when the transport
// stack does not impose one of its own.
const defaultTimeout = 30 * time.Second

// sessionCookieName is the cookie Deluge's Web UI sets on a
// successful auth.login. It is shared by all subsequent /json
// requests and survives until the daemon's session_timeout elapses.
const sessionCookieName = "_session_id"

// Config is the JSON shape persisted in download_clients.config for
// Deluge rows. Host/Port/TLS/Password may also live on the parent
// Definition; values on Config win when both are present so
// operators can route everything through `config` if they prefer.
type Config struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	BasePath string `json:"base_path,omitempty"`
	Password string `json:"password,omitempty"`
}

// resolved merges a Config with the surrounding Definition so the
// client has a single struct of effective values.
type resolved struct {
	baseURL  *url.URL
	password string
}

// Client is a live Deluge Web UI JSON-RPC client. One Client per
// persisted download_clients row. Methods are safe for concurrent
// use; the cookie jar is shared with the underlying http.Client.
type Client struct {
	id       string
	name     string
	protocol downloads.Protocol

	cfg resolved

	http *http.Client

	// loginMu serialises authentication so a flood of concurrent
	// session-expiry retries does not fan out into N parallel
	// re-logins.
	loginMu sync.Mutex

	// rpcID is the monotonically increasing JSON-RPC `id` field.
	// Deluge does not actually correlate by id (each HTTP request
	// is its own conversation), but a unique value per call keeps
	// traces readable.
	rpcID atomic.Int64
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
		return nil, fmt.Errorf("deluge: building cookie jar: %w", err)
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
		return nil, errors.New("deluge: NewWithHTTPClient requires a non-nil http.Client")
	}
	if httpc.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("deluge: building cookie jar: %w", err)
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

// Protocol implements downloads.DownloadClient. Deluge is
// torrent-only.
func (c *Client) Protocol() downloads.Protocol { return c.protocol }

// parseConfig pulls the Deluge-specific fields off the Definition.
// Config-blob values override top-level fields so operators can drive
// everything through config if they prefer.
func parseConfig(def downloads.Definition) (resolved, error) {
	var cfg Config
	if len(def.Config) > 0 {
		if err := json.Unmarshal(def.Config, &cfg); err != nil {
			return resolved{}, fmt.Errorf("deluge: parsing config blob: %w", err)
		}
	}
	host := firstNonEmpty(cfg.Host, def.Host)
	port := cfg.Port
	if port == 0 {
		port = def.Port
	}
	tls := cfg.TLS || def.TLS
	password := firstNonEmpty(cfg.Password, def.Password)

	if host == "" {
		return resolved{}, fmt.Errorf("%w: host is required", ErrNotConfigured)
	}
	if password == "" {
		return resolved{}, fmt.Errorf("%w: password is required", ErrNotConfigured)
	}

	scheme := "http"
	if tls {
		scheme = "https"
	}
	hostport := host
	if port != 0 {
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
		return resolved{}, fmt.Errorf("deluge: assembling base URL %q: %w", raw, err)
	}
	return resolved{
		baseURL:  u,
		password: password,
	}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// rpcEndpoint returns the absolute URL of the JSON-RPC endpoint,
// honouring any reverse-proxy subpath the operator configured.
func (c *Client) rpcEndpoint() string {
	rel, _ := url.Parse("json")
	return c.cfg.baseURL.ResolveReference(rel).String()
}

// rpcRequest is the JSON-RPC envelope Deluge expects. Params is
// always a positional array — Deluge does not honour by-name params.
type rpcRequest struct {
	ID     int64 `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

// rpcResponse is Deluge's reply envelope. Either Result is set, or
// Error is set; never both. The daemon returns HTTP 200 even on
// application errors, so callers must inspect Error rather than the
// status code.
type rpcResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

// RPCError is the error subdocument inside a JSON-RPC reply. Deluge
// fills both Code (numeric) and Message (human-readable); newer
// builds also include a structured "data" object that we ignore.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// call performs an authenticated JSON-RPC round-trip. On a session
// timeout (auth.check_session reports false, or the daemon returns
// a "Not authenticated" RPC error), it logs in once and retries
// exactly once.
func (c *Client) call(ctx context.Context, method string, params []any, out any) error {
	// auth.* methods bootstrap the session — calling ensureLoggedIn
	// for them would recurse forever.
	if !strings.HasPrefix(method, "auth.") {
		if err := c.ensureLoggedIn(ctx); err != nil {
			return err
		}
	}

	body, status, rpcErr, err := c.rpcOnce(ctx, method, params, out)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden ||
		(rpcErr != nil && isAuthError(rpcErr)) {
		if loginErr := c.login(ctx); loginErr != nil {
			return loginErr
		}
		_, status, rpcErr, err = c.rpcOnce(ctx, method, params, out)
		if err != nil {
			return err
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%w: %s returned HTTP %d: %s",
			ErrServer, method, status, truncate(string(body), 200))
	}
	if rpcErr != nil {
		return fmt.Errorf("%w: %s: code=%d %s",
			ErrRPC, method, rpcErr.Code, rpcErr.Message)
	}
	return nil
}

// rpcOnce executes a single attempt, decoding into out when the
// response carries a non-error result. It returns the raw body so
// the caller can include it in error messages.
func (c *Client) rpcOnce(ctx context.Context, method string, params []any, out any) ([]byte, int, *RPCError, error) {
	if params == nil {
		params = []any{}
	}
	req := rpcRequest{
		ID:     c.rpcID.Add(1),
		Method: method,
		Params: params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("deluge: marshalling %s request: %w", method, err)
	}

	endpoint := c.rpcEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("deluge: building %s request: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("deluge: %s: %w", method, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, nil, fmt.Errorf("deluge: reading %s body: %w", method, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, resp.StatusCode, nil, nil
	}

	var env rpcResponse
	if err := json.Unmarshal(respBody, &env); err != nil {
		return respBody, resp.StatusCode, nil,
			fmt.Errorf("%w: %s decode: %w (body=%q)",
				ErrServer, method, err, truncate(string(respBody), 200))
	}
	if env.Error != nil {
		return respBody, resp.StatusCode, env.Error, nil
	}
	if out != nil && len(env.Result) > 0 && string(env.Result) != "null" {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return respBody, resp.StatusCode, nil,
				fmt.Errorf("%w: %s result decode: %w",
					ErrServer, method, err)
		}
	}
	return respBody, resp.StatusCode, nil, nil
}

// isAuthError matches the RPC errors Deluge surfaces when a session
// has expired. Deluge has changed the wording across releases (1.x,
// 2.x, dev branches), so we match on substrings rather than exact
// codes.
func isAuthError(e *RPCError) bool {
	msg := strings.ToLower(e.Message)
	switch {
	case strings.Contains(msg, "not authenticated"):
		return true
	case strings.Contains(msg, "bad login"):
		return true
	case strings.Contains(msg, "session"):
		return true
	}
	return false
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
