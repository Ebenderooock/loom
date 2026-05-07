package transmission

import (
	"context"
	"fmt"

	"github.com/ebenderooock/loom/internal/downloads"
)

// freeSpaceArgs/Reply describe the free-space RPC. We probe the
// daemon's session-default download-dir; the value is the same one
// Transmission's web UI shows in the dashboard footer.
type freeSpaceArgs struct {
	Path string `json:"path"`
}

type freeSpaceReply struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size-bytes"`
}

// sessionDownloadDir is the slice of session-get we need to discover
// the path to query.
type sessionDownloadDir struct {
	DownloadDir string `json:"download-dir"`
}

// FreeSpace implements downloads.DownloadClient.
//
// Returns -1 (the downloads-package "unknown" sentinel) if the
// daemon cannot resolve its download directory or refuses the
// free-space probe; that lets the health-check job persist a sane
// row even on misconfigured hosts.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	var sess sessionDownloadDir
	if err := c.call(ctx, "session-get", nil, &sess); err != nil {
		return -1, fmt.Errorf("transmission: discovering download-dir: %w", err)
	}
	if sess.DownloadDir == "" {
		return -1, nil
	}

	var reply freeSpaceReply
	if err := c.call(ctx, "free-space", freeSpaceArgs{Path: sess.DownloadDir}, &reply); err != nil {
		return -1, err
	}
	if reply.SizeBytes < 0 {
		return -1, nil
	}
	return reply.SizeBytes, nil
}

// Compile-time guard: the Client must satisfy downloads.DownloadClient.
// If a method drifts the build breaks here, not at registry hydrate
// time.
var _ downloads.DownloadClient = (*Client)(nil)
