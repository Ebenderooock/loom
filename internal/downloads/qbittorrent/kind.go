package qbittorrent

import (
	"context"

	"github.com/loomctl/loom/internal/downloads"
)

// factory is the downloads.Factory the package registers under Kind.
// It is exported only as a side-effect of the init() registration so
// tests can exercise it through downloads.LookupKind.
func factory(_ context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
	return New(def)
}

func init() {
	downloads.RegisterKind(Kind, factory)
}
