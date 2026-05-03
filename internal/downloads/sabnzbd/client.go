package sabnzbd

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
	"time"
)

const (
	defaultUserAgent = "Loom/0.1 (+https://loom.dev)"
	defaultTimeout   = 30 * time.Second
)

// Config is the parsed shape of Definition.Config for the SABnzbd
// kind. Host/Port/TLS/BasePath are mirrored from the top-level
// Definition fields when callers leave them blank in the JSON blob,
// because operators frequently fill those in via the dedicated
// columns instead of the config map.
type Config struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	BasePath string `json:"base_path,omitempty"`
	APIKey   string `json:"apikey"`

	// Timeout is the per-request HTTP timeout. Zero means
	// defaultTimeout.
	TimeoutSec int `json:"timeout_seconds,omitempty"`

	// UserAgent overrides the outbound User-Agent header. Zero-value
	// uses defaultUserAgent.
	UserAgent string `json:"user_agent,omitempty"`
}

// Client is the live SABnzbd download client. It is safe for
// concurrent use; the underlying http.Client carries the composed
// transport stack (proxy + throttle).
type Client struct {
	id   string
	name string
	cfg  Config
	http *http.Client
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
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.timeout()}
	}
	return &Client{id: id, name: name, cfg: cfg, http: httpClient}
}

// ID, Name, Kind, and Protocol satisfy downloads.DownloadClient.
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

// endpoint builds the absolute URL for a SABnzbd API call. mode is
// the value of the `mode=` query param; extra is appended verbatim
// (the apikey + output=json keys are stamped in here so callers
// cannot forget them).
func (c *Client) endpoint(mode string, extra url.Values) string {
	scheme := "http"
	if c.cfg.TLS {
		scheme = "https"
	}
	host := c.cfg.Host
	if c.cfg.Port > 0 {
		host = fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	}
	base := strings.TrimRight(c.cfg.BasePath, "/")
	if base == "" {
		base = ""
	} else if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}

	if extra == nil {
		extra = url.Values{}
	}
	extra.Set("mode", mode)
	extra.Set("output", "json")
	extra.Set("apikey", c.cfg.APIKey)

	return fmt.Sprintf("%s://%s%s/api?%s", scheme, host, base, extra.Encode())
}

// envelope is the SAB error envelope: {"status":false,"error":"..."}.
// SAB returns this with HTTP 200, so we have to peek at every JSON
// response.
type envelope struct {
	Status *bool  `json:"status"`
	Error  string `json:"error"`
}

// errorEnvelope inspects body for the SAB error envelope. Returns
// nil when the response is not an error.
func errorEnvelope(body []byte) error {
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return nil
	}
	if e.Status != nil && !*e.Status && e.Error != "" {
		if isAuthError(e.Error) {
			return fmt.Errorf("%w: %s", ErrAuth, e.Error)
		}
		return fmt.Errorf("%w: %s", ErrServer, e.Error)
	}
	return nil
}

// isAuthError matches the messages SAB uses for apikey rejection so
// the caller gets a typed ErrAuth instead of a generic ErrServer.
// The strings are stable across SAB 3.x.
func isAuthError(msg string) bool {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "api key"):
		return true
	case strings.Contains(low, "apikey"):
		return true
	case strings.Contains(low, "not authorized"):
		return true
	}
	return false
}

// getJSON performs an HTTP GET against the SAB API and decodes the
// response into out. Transport failures, non-2xx status, and SAB
// error envelopes are mapped to the typed sentinels.
func (c *Client) getJSON(ctx context.Context, fullURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %s", ErrUpstream, err.Error())
	}
	return c.do(req, out)
}

// postForm posts an application/x-www-form-urlencoded body. SAB
// accepts the same params either via query or POST body; we POST
// for write operations so the API key is not logged in URL access
// records.
func (c *Client) postForm(ctx context.Context, fullURL string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: build request: %s", ErrUpstream, err.Error())
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

// postMultipart uploads a multipart/form-data body. Used for the
// addfile path so an in-memory NZB can be handed directly to SAB.
func (c *Client) postMultipart(ctx context.Context, fullURL string, body *bytes.Buffer, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, body)
	if err != nil {
		return fmt.Errorf("%w: build request: %s", ErrUpstream, err.Error())
	}
	req.Header.Set("Content-Type", contentType)
	return c.do(req, out)
}

// do executes req, classifies the response, and decodes the body
// when out is non-nil. Always closes the response body.
func (c *Client) do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s", ErrUpstream, err.Error())
		}
		return fmt.Errorf("%w: %s", ErrUpstream, err.Error())
	}
	defer resp.Body.Close()

	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return fmt.Errorf("%w: read body: %s", ErrUpstream, rerr.Error())
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: HTTP %d", ErrAuth, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTP %d: %s", ErrUpstream, resp.StatusCode, truncate(body, 256))
	}
	if err := errorEnvelope(body); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decode JSON: %s", ErrUpstream, err.Error())
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
