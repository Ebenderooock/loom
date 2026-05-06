package proxies

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FlareSolverrClient is a thin wrapper around a FlareSolverr endpoint
// (https://github.com/FlareSolverr/FlareSolverr). It exposes a
// RoundTripperFor that hands out per-Proxy-row http.RoundTripper
// instances so newznab/torznab clients can issue their searches as
// regular http.Client.Do calls.
//
// The package keeps the API narrow on purpose: only "request.get" is
// supported (search URLs are GETs), and only "request.headers" /
// "request.body" / "response.cookies" are translated.
type FlareSolverrClient struct {
	httpc          *http.Client
	defaultTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]string // proxyID -> sessionID for KindFlareSolverr/SessionMode=shared
}

// NewFlareSolverrClient builds a client. defaultTimeout is used when
// the per-row FlareSolverrConfig leaves MaxTimeoutSec at zero.
// httpc may be nil; the package falls back to a sensible default.
func NewFlareSolverrClient(httpc *http.Client, defaultTimeout time.Duration) *FlareSolverrClient {
	if httpc == nil {
		httpc = &http.Client{Timeout: 90 * time.Second}
	}
	if defaultTimeout <= 0 {
		defaultTimeout = 60 * time.Second
	}
	return &FlareSolverrClient{
		httpc:          httpc,
		defaultTimeout: defaultTimeout,
		sessions:       make(map[string]string),
	}
}

// RoundTripperFor returns an http.RoundTripper bound to (proxyID,
// cfg). Returned tripper is safe for concurrent use.
func (c *FlareSolverrClient) RoundTripperFor(proxyID string, cfg FlareSolverrConfig) http.RoundTripper {
	return &flareRoundTripper{c: c, proxyID: proxyID, cfg: cfg}
}

// Ping verifies the FlareSolverr endpoint is reachable. It calls the
// `sessions.list` command and treats any "ok" envelope as success.
// Used by POST /api/v1/proxies/{id}/test for KindFlareSolverr.
func (c *FlareSolverrClient) Ping(ctx context.Context, cfg FlareSolverrConfig) error {
	envelope, err := c.do(ctx, cfg, flareReq{Cmd: "sessions.list"})
	if err != nil {
		return err
	}
	if !strings.EqualFold(envelope.Status, "ok") {
		return fmt.Errorf("flaresolverr: %s", envelope.Message)
	}
	return nil
}

// flareReq is the shape FlareSolverr accepts on POST /v1.
type flareReq struct {
	Cmd        string         `json:"cmd"`
	URL        string         `json:"url,omitempty"`
	MaxTimeout int64          `json:"maxTimeout,omitempty"`
	Session    string         `json:"session,omitempty"`
	Cookies    []flareCookie  `json:"cookies,omitempty"`
}

// flareEnvelope is the wrapper returned by every command.
type flareEnvelope struct {
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Solution *flareSolution `json:"solution,omitempty"`
	Session  string         `json:"session,omitempty"`
}

// flareSolution carries the synthesised response when cmd ==
// "request.get".
type flareSolution struct {
	URL       string            `json:"url"`
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers"`
	Response  string            `json:"response"`
	Cookies   []flareCookie     `json:"cookies"`
	UserAgent string            `json:"userAgent"`
}

type flareCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (c *FlareSolverrClient) do(ctx context.Context, cfg FlareSolverrConfig, body flareReq) (flareEnvelope, error) {
	if body.MaxTimeout == 0 {
		ms := c.defaultTimeout.Milliseconds()
		if cfg.MaxTimeoutSec > 0 {
			ms = int64(cfg.MaxTimeoutSec) * 1000
		}
		body.MaxTimeout = ms
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return flareEnvelope{}, fmt.Errorf("flaresolverr: marshal request: %w", err)
	}
	endpoint := strings.TrimRight(cfg.URL, "/") + "/v1"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return flareEnvelope{}, fmt.Errorf("flaresolverr: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if dl, ok := ctx.Deadline(); ok {
		slog.Debug("flaresolverr: sending request", "cmd", body.Cmd, "endpoint", endpoint, "deadline_in", time.Until(dl).String(), "httpc_timeout", c.httpc.Timeout.String(), "maxTimeout_ms", body.MaxTimeout)
	} else {
		slog.Debug("flaresolverr: sending request", "cmd", body.Cmd, "endpoint", endpoint, "deadline", "none", "httpc_timeout", c.httpc.Timeout.String(), "maxTimeout_ms", body.MaxTimeout)
	}
	start := time.Now()
	resp, err := c.httpc.Do(req)
	slog.Debug("flaresolverr: response received", "cmd", body.Cmd, "elapsed", time.Since(start).String(), "err", err)
	if err != nil {
		return flareEnvelope{}, fmt.Errorf("flaresolverr: do request: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return flareEnvelope{}, fmt.Errorf("flaresolverr: read response: %w", err)
	}
	var env flareEnvelope
	if err := json.Unmarshal(respBytes, &env); err != nil {
		return flareEnvelope{}, fmt.Errorf("flaresolverr: decode envelope: %w (body=%q)", err, string(respBytes))
	}
	return env, nil
}

func (c *FlareSolverrClient) sessionFor(ctx context.Context, proxyID string, cfg FlareSolverrConfig) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sid, ok := c.sessions[proxyID]; ok && sid != "" {
		return sid, nil
	}
	env, err := c.do(ctx, cfg, flareReq{Cmd: "sessions.create"})
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(env.Status, "ok") || env.Session == "" {
		return "", fmt.Errorf("flaresolverr sessions.create: %s", env.Message)
	}
	c.sessions[proxyID] = env.Session
	return env.Session, nil
}

// flareRoundTripper turns a FlareSolverr "request.get" response into
// an http.Response. Only GET is supported — newznab/torznab issue
// pure GETs anyway, and POST/PUT/etc. are out of scope for Phase 2e.
type flareRoundTripper struct {
	c       *FlareSolverrClient
	proxyID string
	cfg     FlareSolverrConfig
}

func (rt *flareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("flaresolverr: unsupported method %s", req.Method)
	}
	body := flareReq{Cmd: "request.get", URL: req.URL.String()}
	// Forward cookies from the original request (e.g. definition-level
	// cookies like EZTV's layout=def_wlinks) so FlareSolverr's browser
	// session includes them.
	if cookies := req.Cookies(); len(cookies) > 0 {
		body.Cookies = make([]flareCookie, len(cookies))
		for i, c := range cookies {
			body.Cookies[i] = flareCookie{Name: c.Name, Value: c.Value}
		}
	}
	if rt.cfg.SessionMode == "shared" {
		sid, err := rt.c.sessionFor(req.Context(), rt.proxyID, rt.cfg)
		if err != nil {
			return nil, err
		}
		body.Session = sid
	}
	env, err := rt.c.do(req.Context(), rt.cfg, body)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(env.Status, "ok") || env.Solution == nil {
		return nil, fmt.Errorf("flaresolverr: %s", env.Message)
	}
	sol := env.Solution
	hdr := make(http.Header, len(sol.Headers))
	for k, v := range sol.Headers {
		hdr.Set(k, v)
	}
	for _, ck := range sol.Cookies {
		hdr.Add("Set-Cookie", fmt.Sprintf("%s=%s", ck.Name, ck.Value))
	}
	if sol.UserAgent != "" && hdr.Get("User-Agent") == "" {
		hdr.Set("User-Agent", sol.UserAgent)
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", sol.Status, http.StatusText(sol.Status)),
		StatusCode:    sol.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        hdr,
		Body:          io.NopCloser(strings.NewReader(sol.Response)),
		ContentLength: int64(len(sol.Response)),
		Request:       req,
	}, nil
}
