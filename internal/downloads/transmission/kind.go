package transmission

import (
	"context"

	"github.com/ebenderooock/loom/internal/downloads"
)

// factory is the downloads.Factory the package registers under Kind.
// It is the same shape qbittorrent and sabnzbd use so a future
// auditor sees exactly one convention across kinds.
func factory(_ context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
	return New(def)
}

func init() {
	downloads.RegisterKind(Kind, factory)
}
