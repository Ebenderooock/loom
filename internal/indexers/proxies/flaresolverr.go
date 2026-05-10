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
// The client is fully stateless per request — no FlareSolverr sessions
// are created or reused, matching Prowlarr's approach. Only
// "request.get" is supported (search URLs are GETs).
type FlareSolverrClient struct {
	httpc          *http.Client
	defaultTimeout time.Duration

	// Per-domain UA cache: after a successful FlareSolverr solve, the
	// returned UserAgent is cached so subsequent requests to the same
	// domain can inject it without re-solving. Matches Prowlarr's
	// IIndexerProxy PreRequest UA injection pattern.
	uaMu    sync.RWMutex
	uaCache map[string]cachedUA // host → UA + expiry
}

// NewFlareSolverrClient builds a client. defaultTimeout is used when
// the per-row FlareSolverrConfig leaves MaxTimeoutSec at zero.
// httpc may be nil; the package falls back to a sensible default.
func NewFlareSolverrClient(httpc *http.Client, defaultTimeout time.Duration) *FlareSolverrClient {
	if httpc == nil {
		httpc = &http.Client{Timeout: 120 * time.Second}
	}
	if defaultTimeout <= 0 {
		defaultTimeout = 90 * time.Second
	}
	return &FlareSolverrClient{
		httpc:          httpc,
		defaultTimeout: defaultTimeout,
		uaCache:        make(map[string]cachedUA),
	}
}

const uaCacheTTL = 24 * time.Hour

type cachedUA struct {
	userAgent string
	expiresAt time.Time
}

// CachedUserAgent returns the cached UserAgent for a domain, if fresh.
func (c *FlareSolverrClient) CachedUserAgent(host string) (string, bool) {
	c.uaMu.RLock()
	defer c.uaMu.RUnlock()
	entry, ok := c.uaCache[host]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.userAgent, true
}

// cacheUserAgent stores a solved UserAgent for the domain.
func (c *FlareSolverrClient) cacheUserAgent(host, ua string) {
	if ua == "" || host == "" {
		return
	}
	c.uaMu.Lock()
	defer c.uaMu.Unlock()
	c.uaCache[host] = cachedUA{
		userAgent: ua,
		expiresAt: time.Now().Add(uaCacheTTL),
	}
	slog.Debug("flaresolverr: cached UA for domain", "host", host, "ua", ua[:min(40, len(ua))]+"...")
}

// RoundTripperFor returns an http.RoundTripper that follows Prowlarr's
// PreRequest/PostResponse pattern: requests go direct first, FlareSolverr
// is called only when Cloudflare is detected, and solutions are cached.
func (c *FlareSolverrClient) RoundTripperFor(_ string, cfg FlareSolverrConfig) http.RoundTripper {
	return &flareRoundTripper{
		c:           c,
		cfg:         cfg,
		base:        http.DefaultTransport,
		cookieCache: make(map[string][]flareCookie),
	}
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
	Cmd        string        `json:"cmd"`
	URL        string        `json:"url,omitempty"`
	MaxTimeout int64         `json:"maxTimeout,omitempty"`
	Cookies    []flareCookie `json:"cookies,omitempty"`
}

// flareEnvelope is the wrapper returned by every command.
type flareEnvelope struct {
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Solution *flareSolution `json:"solution,omitempty"`
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

// flareRoundTripper implements Prowlarr's PreRequest/PostResponse
// pattern: requests go DIRECT to the indexer first. Only when the
// response is a Cloudflare challenge does it call FlareSolverr to
// solve, cache the UA + cookies, and retry. Subsequent requests
// inject the cached UA to avoid re-solving.
type flareRoundTripper struct {
	c    *FlareSolverrClient
	cfg  FlareSolverrConfig
	base http.RoundTripper // direct transport (no proxy)

	// Per-domain cookie cache (cf_clearance etc.).
	cookieMu    sync.RWMutex
	cookieCache map[string][]flareCookie // host → cookies
}

func (rt *flareRoundTripper) cachedCookies(host string) []flareCookie {
	rt.cookieMu.RLock()
	defer rt.cookieMu.RUnlock()
	return rt.cookieCache[host]
}

func (rt *flareRoundTripper) storeCookies(host string, cookies []flareCookie) {
	if len(cookies) == 0 || host == "" {
		return
	}
	rt.cookieMu.Lock()
	defer rt.cookieMu.Unlock()
	rt.cookieCache[host] = cookies
}

func (rt *flareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("flaresolverr: unsupported method %s", req.Method)
	}

	host := req.URL.Hostname()

	// PreRequest: inject cached UA + cookies from prior FlareSolverr
	// solve — matches Prowlarr's PreRequest pattern.
	if ua, ok := rt.c.CachedUserAgent(host); ok && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	if cached := rt.cachedCookies(host); len(cached) > 0 {
		for _, ck := range cached {
			req.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
		}
	}

	// Step 1: try direct.
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// PostResponse: check if CF-protected. Read body for deep inspection.
	var body []byte
	if resp.Body != nil {
		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	if !isCloudflareResponse(resp, body) {
		return resp, nil // not CF, return as-is
	}

	slog.Info("flaresolverr: cloudflare detected, solving via FlareSolverr",
		"host", host, "status", resp.StatusCode)

	// Step 2: call FlareSolverr to solve the challenge.
	fsBody := flareReq{Cmd: "request.get", URL: req.URL.String()}
	if cookies := req.Cookies(); len(cookies) > 0 {
		fsBody.Cookies = make([]flareCookie, len(cookies))
		for i, c := range cookies {
			fsBody.Cookies[i] = flareCookie{Name: c.Name, Value: c.Value}
		}
	}
	env, err := rt.c.do(req.Context(), rt.cfg, fsBody)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: solve failed: %w", err)
	}
	if !strings.EqualFold(env.Status, "ok") || env.Solution == nil {
		return nil, fmt.Errorf("flaresolverr: solve rejected: %s", env.Message)
	}
	sol := env.Solution

	// Cache the solved UA + cookies for this domain.
	if sol.UserAgent != "" {
		rt.c.cacheUserAgent(host, sol.UserAgent)
	}
	rt.storeCookies(host, sol.Cookies)

	slog.Info("flaresolverr: challenge solved, retrying request",
		"host", host, "ua", sol.UserAgent[:min(40, len(sol.UserAgent))])

	// Step 3: retry the original request with solved cookies + UA.
	retry, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: build retry request: %w", err)
	}
	// Copy original headers.
	for k, vs := range req.Header {
		for _, v := range vs {
			retry.Header.Add(k, v)
		}
	}
	// Override UA with solved value.
	if sol.UserAgent != "" {
		retry.Header.Set("User-Agent", sol.UserAgent)
	}
	// Inject solved cookies.
	for _, ck := range sol.Cookies {
		retry.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
	}

	return base.RoundTrip(retry)
}

// isCloudflareResponse checks if a response is a Cloudflare challenge.
// Matches Prowlarr's CloudFlareDetectionService.
func isCloudflareResponse(resp *http.Response, body []byte) bool {
	if resp == nil {
		return false
	}
	server := strings.ToLower(resp.Header.Get("Server"))
	isCF := strings.Contains(server, "cloudflare") ||
		strings.Contains(server, "cloudflare-nginx") ||
		strings.Contains(server, "ddos-guard")
	if isCF && (resp.StatusCode == 403 || resp.StatusCode == 503) {
		return true
	}
	if resp.Header.Get("CF-RAY") != "" && resp.StatusCode >= 400 {
		return true
	}
	// Body-based detection (matching Prowlarr's HTML pattern checks).
	if len(body) > 0 && (resp.StatusCode == 403 || resp.StatusCode == 503) {
		lower := strings.ToLower(string(body[:min(8192, len(body))]))
		for _, sig := range cfSignatures {
			if strings.Contains(lower, sig) {
				return true
			}
		}
	}
	return false
}

var cfSignatures = []string{
	"just a moment",
	"cf-browser-verification",
	"cdn-cgi/challenge-platform",
	"challenges.cloudflare.com",
	"attention required! | cloudflare",
	"<title>ddos-guard</title>",
}
