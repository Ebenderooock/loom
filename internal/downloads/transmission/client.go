package transmission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind is the registry key under which this implementation registers
// itself with the downloads core.
const Kind = downloads.KindTransmission

const (
	defaultUserAgent  = "Loom/0.1 (+https://loom.dev) transmission-client"
	defaultTimeout    = 30 * time.Second
	defaultRPCURL     = "/transmission/rpc"
	sessionHeader     = "X-Transmission-Session-Id"
	maxErrorBodyBytes = 512
)

// Config is the JSON shape persisted in download_clients.config for
// transmission rows. Host/Port/TLS/Username/Password may also live on
// the parent Definition; values on Config win when both are present
// so operators can route everything through `config` if they prefer.
type Config struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	RPCURL   string `json:"rpc_url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Categories is an optional fixed list of label names exposed
	// via Categories(). When empty the client returns the union of
	// labels actually present on torrents (see categories.go).
	Categories []string `json:"categories,omitempty"`
}

// resolved is the merged, normalised view of a Config + Definition.
// One value per concept, ready for the request loop.
type resolved struct {
	endpoint   *url.URL
	username   string
	password   string
	categories []string
}

// Client is a live Transmission RPC client. One Client per persisted
// download_clients row. Methods are safe for concurrent use; the
// session id is guarded by sessionMu so a flood of concurrent 409s
// does not fan out into N parallel handshakes.
type Client struct {
	id       string
	name     string
	protocol downloads.Protocol

	cfg  resolved
	http *http.Client

	sessionMu sync.RWMutex
	sessionID string
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
	httpc := &http.Client{
		Transport: rt,
		Timeout:   defaultTimeout,
	}
	return NewWithHTTPClient(def, cfg, httpc)
}

// NewWithHTTPClient builds a Client that uses the supplied
// *http.Client as-is. This entry point exists for tests and for
// operators who want to supply a fully custom transport (e.g.
// tracing-instrumented).
func NewWithHTTPClient(def downloads.Definition, cfg resolved, httpc *http.Client) (*Client, error) {
	if httpc == nil {
		return nil, errors.New("transmission: NewWithHTTPClient requires a non-nil http.Client")
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

// ID, Name, Kind, Protocol satisfy downloads.DownloadClient.

// ID returns the persisted download_clients.id this Client was
// hydrated from.
func (c *Client) ID() string { return c.id }

// Name returns the operator-facing display name set on the row.
func (c *Client) Name() string { return c.name }

// Kind returns the registry key for the Transmission driver.
func (c *Client) Kind() downloads.Kind { return Kind }

// Protocol implements downloads.DownloadClient. Transmission is
// torrent-only.
func (c *Client) Protocol() downloads.Protocol { return c.protocol }

// parseConfig pulls the transmission-specific fields off the
// Definition. Config-blob values override top-level fields so that
// operators can drive everything through config if they prefer.
func parseConfig(def downloads.Definition) (resolved, error) {
	var cfg Config
	if len(def.Config) > 0 {
		if err := json.Unmarshal(def.Config, &cfg); err != nil {
			return resolved{}, fmt.Errorf("%w: parsing config blob: %s", ErrConfig, err.Error())
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
	rpcURL := cfg.RPCURL
	if rpcURL == "" {
		rpcURL = defaultRPCURL
	}
	if !strings.HasPrefix(rpcURL, "/") {
		rpcURL = "/" + rpcURL
	}

	if host == "" {
		return resolved{}, fmt.Errorf("%w: host is required", ErrConfig)
	}

	scheme := "http"
	if tls {
		scheme = "https"
	}
	hostport := host
	if port != 0 {
		hostport = fmt.Sprintf("%s:%d", host, port)
	}
	raw := fmt.Sprintf("%s://%s%s", scheme, hostport, rpcURL)
	u, err := url.Parse(raw)
	if err != nil {
		return resolved{}, fmt.Errorf("%w: assembling RPC URL %q: %s", ErrConfig, raw, err.Error())
	}
	return resolved{
		endpoint:   u,
		username:   username,
		password:   password,
		categories: cfg.Categories,
	}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// rpcRequest is the JSON envelope every Transmission RPC call wraps.
// `tag` is echoed back verbatim; we leave it zero because Loom only
// issues one RPC per HTTP request.
type rpcRequest struct {
	Method    string `json:"method"`
	Arguments any    `json:"arguments,omitempty"`
	Tag       int    `json:"tag,omitempty"`
}

// rpcResponse is the matching reply. `result` is "success" on the
// happy path; anything else is a logical error and surfaces as
// ErrServer wrapping the upstream string.
type rpcResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments"`
	Tag       int             `json:"tag,omitempty"`
}

// call issues a single RPC, transparently performing the session-id
// handshake on a 409 and decoding `arguments` into out (when non-nil).
func (c *Client) call(ctx context.Context, method string, args any, out any) error {
	body, err := json.Marshal(rpcRequest{Method: method, Arguments: args})
	if err != nil {
		return fmt.Errorf("%w: encoding RPC %q: %s", ErrUpstream, method, err.Error())
	}

	resp, status, err := c.exchange(ctx, method, body)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		// Handshake: server told us its current session id. Adopt
		// it and replay the request exactly once.
		if newID := strings.TrimSpace(headerFromBytes(resp)); newID != "" {
			c.setSessionID(newID)
		}
		resp, status, err = c.exchange(ctx, method, body)
		if err != nil {
			return err
		}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%w: HTTP %d", ErrAuth, status)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%w: %s returned HTTP %d: %s", ErrUpstream, method, status, truncate(resp, maxErrorBodyBytes))
	}

	var env rpcResponse
	if err := json.Unmarshal(resp, &env); err != nil {
		return fmt.Errorf("%w: decoding %s response: %s", ErrUpstream, method, err.Error())
	}
	if env.Result != "success" {
		return fmt.Errorf("%w: %s: %s", ErrServer, method, strings.TrimSpace(env.Result))
	}
	if out == nil {
		return nil
	}
	if len(env.Arguments) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Arguments, out); err != nil {
		return fmt.Errorf("%w: decoding %s arguments: %s", ErrUpstream, method, err.Error())
	}
	return nil
}

// exchange performs a single HTTP attempt of the RPC. It returns the
// raw body for the caller to inspect — call() needs the response
// header on a 409 path, which we encode in-band via headerFromBytes
// because http.Response is consumed inside this helper.
//
// The first return value is the response body on a 2xx/non-409 path,
// or the value of the X-Transmission-Session-Id header (wrapped via
// headerToBytes) on a 409. status is always the real HTTP status.
func (c *Client) exchange(ctx context.Context, method string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: building %s request: %s", ErrUpstream, method, err.Error())
	}
	// Replayable body for the (single) 409 retry.
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)
	if id := c.getSessionID(); id != "" {
		req.Header.Set(sessionHeader, id)
	}
	if c.cfg.username != "" || c.cfg.password != "" {
		req.SetBasicAuth(c.cfg.username, c.cfg.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %s: %s", ErrUpstream, method, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		// Capture the new session id from the header and surface
		// it to the caller via the body slot. The actual response
		// body on a 409 is "409: Conflict" plain text, which we
		// do not need to read.
		newID := resp.Header.Get(sessionHeader)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
		return headerToBytes(newID), http.StatusConflict, nil
	}

	bodyOut, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: reading %s body: %s", ErrUpstream, method, err.Error())
	}
	return bodyOut, resp.StatusCode, nil
}

// headerToBytes/headerFromBytes box a session-id string in/out of the
// []byte slot exchange() returns. The encoding is deliberately a no-op
// because session ids are ASCII; the named helpers exist so the
// 409-path is greppable and self-documenting.
func headerToBytes(id string) []byte   { return []byte(id) }
func headerFromBytes(b []byte) string  { return string(b) }

func (c *Client) getSessionID() string {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.sessionID
}

func (c *Client) setSessionID(id string) {
	c.sessionMu.Lock()
	c.sessionID = id
	c.sessionMu.Unlock()
}

// truncate clips body for inclusion in an error message so a
// misbehaving upstream cannot blow up a single log line.
func truncate(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "…"
}
