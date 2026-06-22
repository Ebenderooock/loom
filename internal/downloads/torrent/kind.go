package torrent

import (
	"context"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind is the registry key under which this implementation registers itself.
const Kind = downloads.KindBuiltinTorrent

func factory(_ context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
	cfg, err := parseConfig(def)
	if err != nil {
		return nil, err
	}
	return New(def, cfg)
}

func init() {
	downloads.RegisterKind(Kind, factory)
}
