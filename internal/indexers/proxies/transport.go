package proxies

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// BuildTransport returns an http.RoundTripper that routes outbound
// HTTP through the supplied Proxy row. The returned transport must be
// safe for concurrent use; callers may share one instance per Proxy
// row across many indexer clients.
//
// Returning the package's default http.Transport for proxy.Enabled =
// false would silently neuter the user's choice; we return an error
// instead so the service can refuse to attach a disabled proxy to an
// indexer.
func BuildTransport(p Proxy, fs *FlareSolverrClient) (http.RoundTripper, error) {
	if !p.Enabled {
		return nil, fmt.Errorf("proxy %q is disabled", p.ID)
	}
	switch p.Kind {
	case KindHTTP, KindHTTPS:
		cfg, err := ParseHTTPConfig(p.Config)
		if err != nil {
			return nil, err
		}
		return buildHTTPTransport(cfg)
	case KindSOCKS5:
		cfg, err := ParseSOCKS5Config(p.Config)
		if err != nil {
			return nil, err
		}
		return buildSOCKS5Transport(cfg)
	case KindFlareSolverr:
		cfg, err := ParseFlareSolverrConfig(p.Config)
		if err != nil {
			return nil, err
		}
		if fs == nil {
			return nil, errors.New("flaresolverr proxy: client missing")
		}
		return fs.RoundTripperFor(p.ID, cfg), nil
	default:
		return nil, fmt.Errorf("unknown proxy kind %q", p.Kind)
	}
}

func buildHTTPTransport(cfg HTTPConfig) (http.RoundTripper, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("http proxy: parse url: %w", err)
	}
	if cfg.Username != "" || cfg.Password != "" {
		u.User = url.UserPassword(cfg.Username, cfg.Password)
	}
	// Clone DefaultTransport to keep its conservative timeouts.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http proxy: default transport is not *http.Transport")
	}
	t := base.Clone()
	t.Proxy = http.ProxyURL(u)
	return t, nil
}

func buildSOCKS5Transport(cfg SOCKS5Config) (http.RoundTripper, error) {
	var auth *proxy.Auth
	if cfg.Username != "" || cfg.Password != "" {
		auth = &proxy.Auth{User: cfg.Username, Password: cfg.Password}
	}
	dialer, err := proxy.SOCKS5("tcp", cfg.Address, auth, &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("socks5 proxy: %w", err)
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("socks5 proxy: default transport is not *http.Transport")
	}
	t := base.Clone()
	t.Proxy = nil
	// Prefer the context-aware Dial when the underlying dialer
	// supports it (golang.org/x/net/proxy.SOCKS5 does as of x/net
	// >= 0.27 via the proxy.ContextDialer interface).
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		t.DialContext = cd.DialContext
	} else {
		t.Dial = dialer.Dial //nolint:staticcheck // SA1019: fallback for older x/net.
	}
	return t, nil
}

// Provider is the indexers.TransportProvider implementation backed by
// the proxies repository. It caches built transports per proxy ID
// and busts the cache on Update / Delete (the Service calls
// Invalidate). Lookups for unknown / disabled proxies fall back to
// returning an error so callers can decide whether to use the
// default transport.
type Provider struct {
	repo Repository
	fs   *FlareSolverrClient

	mu    sync.RWMutex
	cache map[string]http.RoundTripper
}

// NewProvider builds a Provider over repo. The FlareSolverr client
// is optional; if nil, FlareSolverr proxies will fail to build.
func NewProvider(repo Repository, fs *FlareSolverrClient) *Provider {
	return &Provider{repo: repo, fs: fs, cache: make(map[string]http.RoundTripper)}
}

// TransportFor implements indexers.TransportProvider.
func (p *Provider) TransportFor(proxyID string) (http.RoundTripper, error) {
	if proxyID == "" {
		return http.DefaultTransport, nil
	}
	p.mu.RLock()
	if rt, ok := p.cache[proxyID]; ok {
		p.mu.RUnlock()
		return rt, nil
	}
	p.mu.RUnlock()

	// Slow path: load + build.
	row, err := p.repo.Get(context.Background(), proxyID)
	if err != nil {
		return nil, err
	}
	rt, err := BuildTransport(row, p.fs)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[proxyID] = rt
	p.mu.Unlock()
	return rt, nil
}

// Invalidate drops the cached transport for proxyID. Callers (the
// Service) invoke this on Replace/Patch/Delete so the next indexer
// build picks up the new config. Empty string clears the entire
// cache.
func (p *Provider) Invalidate(proxyID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if proxyID == "" {
		p.cache = make(map[string]http.RoundTripper)
		return
	}
	delete(p.cache, proxyID)
}
