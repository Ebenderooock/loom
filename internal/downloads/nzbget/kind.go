package nzbget

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ebenderooock/loom/internal/downloads"
)

// httpClientFactory builds the *http.Client a Client uses for all
// outbound traffic. It composes the per-definition transport stack
// (proxy + throttle) via downloads.TransportForDefinition; tests
// override the factory to point at httptest.NewServer.
//
// The seam intentionally mirrors the indexer kind packages and the
// SABnzbd / qBittorrent kinds so a future audit confirms one
// convention covers both subsystems.
var httpClientFactory = func(cfg Config, def downloads.Definition) *http.Client {
	rt, err := downloads.TransportForDefinition(def)
	if err != nil || rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{Timeout: cfg.timeout(), Transport: rt}
}

// SetHTTPClientFactory installs a custom builder. Production
// callers do not need this; the test suite uses it to inject an
// httptest transport without monkey-patching DialTLS.
func SetHTTPClientFactory(f func(cfg Config, def downloads.Definition) *http.Client) {
	httpClientFactory = f
}

// factory is the downloads.Factory closure registered for the
// "nzbget" kind. It parses the config blob, falls back to the
// top-level Definition columns where applicable, validates the
// minimum-required fields, and returns a fully-wired *Client.
func factory(_ context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
	cfg, err := parseConfig(def.Config)
	if err != nil {
		return nil, fmt.Errorf("download client %q (nzbget): %w", def.ID, err)
	}
	if cfg.Host == "" {
		cfg.Host = def.Host
	}
	if cfg.Port == 0 {
		cfg.Port = def.Port
	}
	if !cfg.TLS {
		cfg.TLS = def.TLS
	}
	if cfg.Username == "" {
		cfg.Username = def.Username
	}
	if cfg.Password == "" {
		cfg.Password = def.Password
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "/"
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("download client %q (nzbget): %w: host is required", def.ID, ErrConfig)
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("download client %q (nzbget): %w: control username and password are required", def.ID, ErrConfig)
	}
	return NewClient(def.ID, def.Name, cfg, httpClientFactory(cfg, def)), nil
}

func init() {
	downloads.RegisterKind(downloads.KindNZBGet, factory)
}
