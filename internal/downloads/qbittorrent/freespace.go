package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/loomctl/loom/internal/downloads"
)

// mainData is the (heavily) reduced shape of /api/v2/sync/maindata.
// The endpoint is large; we extract only the field we need.
type mainData struct {
	ServerState struct {
		FreeSpaceOnDisk *int64 `json:"free_space_on_disk"`
	} `json:"server_state"`
}

// FreeSpace implements downloads.DownloadClient. Returns -1 when the
// running qBittorrent does not advertise free_space_on_disk (older
// 4.1.x builds and bare-metal Windows installs in particular). The
// downloads core treats -1 as "unknown" and elides it from health
// rows.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	body, err := c.get(ctx, "sync/maindata", nil)
	if err != nil {
		return -1, err
	}
	var md mainData
	if err := json.Unmarshal(body, &md); err != nil {
		return -1, fmt.Errorf("qbittorrent: parsing sync/maindata: %w", err)
	}
	if md.ServerState.FreeSpaceOnDisk == nil {
		return -1, nil
	}
	return *md.ServerState.FreeSpaceOnDisk, nil
}

// Compile-time guard: keep the Client honest about implementing the
// downloads contract. If a method drifts the build breaks here, not
// at registry hydrate time.
var _ downloads.DownloadClient = (*Client)(nil)
