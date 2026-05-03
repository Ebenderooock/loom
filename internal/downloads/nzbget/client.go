package nzbget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultUserAgent = "Loom/0.1 (+https://loom.dev)"
	defaultTimeout   = 30 * time.Second
)

// Config is the parsed shape of Definition.Config for the NZBGet
// kind. Host/Port/TLS/BasePath are mirrored from the top-level
// Definition fields when callers leave them blank in the JSON blob,
// because operators frequently fill those in via the dedicated
// columns instead of the config map.
type Config struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	BasePath string `json:"base_path,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Timeout is the per-request HTTP timeout. Zero means
	// defaultTimeout.
	TimeoutSec int `json:"timeout_seconds,omitempty"`

	// UserAgent overrides the outbound User-Agent header. Zero-value
	// uses defaultUserAgent.
	UserAgent string `json:"user_agent,omitempty"`
}

// Client is the live NZBGet download client. It is safe for
// concurrent use; the underlying http.Client carries the composed
// transport stack (proxy + throttle).
type Client struct {
	id   string
	name string
	cfg  Config
	http *http.Client

	// idCounter is the rolling JSON-RPC `id` field. Atomicity is
	// not required because the id is only used to correlate request
	// and response within one round trip; we just want non-zero
	// values for log readability.
	idCounter int
}

// NewClient constructs a Client from id/name/cfg. httpClient may be
// nil; the constructor installs a default with the configured
// timeout and http.DefaultTransport. Production callers should pass
// a client built via downloads.TransportForDefinition so per-client
// proxy and rate-limit policies apply.
func NewClient(id, name string, cfg Config, httpClient *http.Client) *Client {
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "/"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.timeout()}
	}
	return &Client{id: id, name: name, cfg: cfg, http: httpClient}
}

// ID returns the persisted download_clients.id.
func (c *Client) ID() string { return c.id }

// Name returns the operator-facing display name set on the row.
func (c *Client) Name() string { return c.name }

// timeout resolves the configured request timeout, falling back to
// defaultTimeout when unset.
func (cfg Config) timeout() time.Duration {
	if cfg.TimeoutSec <= 0 {
		return defaultTimeout
	}
	return time.Duration(cfg.TimeoutSec) * time.Second
}

// rpcEndpoint builds the absolute URL for the NZBGet JSON-RPC
// endpoint. NZBGet exposes JSON-RPC at <base_path>jsonrpc; XML-RPC
// is at the same path with method appended (we do not use XML-RPC).
func (c *Client) rpcEndpoint() string {
	scheme := "http"
	if c.cfg.TLS {
		scheme = "https"
	}
	host := c.cfg.Host
	if c.cfg.Port > 0 {
		host = fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	}
	base := strings.TrimRight(c.cfg.BasePath, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return fmt.Sprintf("%s://%s%s/jsonrpc", scheme, host, base)
}

// rpcRequest is the JSON-RPC 2.0 envelope we send. NZBGet ignores
// jsonrpc/version mismatches but the field is required by spec.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

// rpcError is the JSON-RPC 2.0 error sub-envelope.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is the JSON-RPC 2.0 response envelope. Result is held
// as RawMessage so each method can decode the shape it expects
// without paying for an intermediate map traversal.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

// call performs a JSON-RPC call against NZBGet. The decoded
// `result` field is unmarshalled into out (when non-nil). Auth
// failures, transport errors, and JSON-RPC error envelopes map to
// the typed sentinels.
func (c *Client) call(ctx context.Context, method string, params []any, out any) error {
	if params == nil {
		params = []any{}
	}
	c.idCounter++
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      c.idCounter,
	})
	if err != nil {
		return fmt.Errorf("%w: marshal %s: %s", ErrUpstream, method, err.Error())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcEndpoint(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build request: %s", ErrUpstream, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	// NZBGet requires HTTP Basic Auth before the JSON-RPC envelope
	// is even parsed; sending the header on every call avoids the
	// 401 → retry round trip that comes with WWW-Authenticate.
	if c.cfg.Username != "" || c.cfg.Password != "" {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s: %s", ErrUpstream, method, err.Error())
		}
		return fmt.Errorf("%w: %s: %s", ErrUpstream, method, err.Error())
	}
	defer resp.Body.Close()

	raw, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return fmt.Errorf("%w: read body: %s", ErrUpstream, rerr.Error())
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: HTTP %d", ErrAuth, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s: HTTP %d: %s", ErrUpstream, method, resp.StatusCode, truncate(raw, 256))
	}

	var env rpcResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("%w: decode envelope for %s: %s", ErrUpstream, method, err.Error())
	}
	if env.Error != nil {
		return fmt.Errorf("%w: %s: %s (code %d)", ErrServer, method, env.Error.Message, env.Error.Code)
	}
	if out == nil {
		return nil
	}
	if len(env.Result) == 0 || string(env.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("%w: decode result for %s: %s", ErrUpstream, method, err.Error())
	}
	return nil
}

// truncate trims body down for inclusion in error messages.
func truncate(body []byte, n int) string {
	if len(body) > n {
		return string(body[:n]) + "…"
	}
	return string(body)
}

// parseConfig decodes a Definition.Config JSON blob into Config.
// Empty/nil blobs return a zero-value Config so the factory can
// fall back to the Definition's top-level Host/Port/TLS columns.
func parseConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("%w: %s", ErrConfig, err.Error())
	}
	return cfg, nil
}
