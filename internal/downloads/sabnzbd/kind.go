package sabnzbd

import (
	"context"
	"fmt"

	"github.com/ebenderooock/loom/internal/downloads"
)

// factory is the downloads.Factory closure registered for the
// "sabnzbd" kind. It parses the config blob, falls back to the
// top-level Definition columns where applicable, and returns a
// fully-wired *Client.
func factory(_ context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
	cfg, err := parseConfig(def.Config)
	if err != nil {
		return nil, fmt.Errorf("download client %q (sabnzbd): %w", def.ID, err)
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
	if cfg.BasePath == "" {
		cfg.BasePath = "/"
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("download client %q (sabnzbd): %w: host is required", def.ID, ErrConfig)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("download client %q (sabnzbd): %w: apikey is required", def.ID, ErrConfig)
	}
	return NewClient(def.ID, def.Name, cfg, downloads.HTTPClientForDefinition(def, cfg.timeout())), nil
}

func init() {
	downloads.RegisterKind(downloads.KindSABnzbd, factory)
}
