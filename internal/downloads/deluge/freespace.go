package deluge

import (
	"context"

	"github.com/ebenderooock/loom/internal/downloads"
)

// FreeSpace implements downloads.DownloadClient. We ask the daemon
// for the active download_location config value and then for the
// free space rooted at that path. Returns -1 when the daemon does
// not advertise a path (rare, but happens on misconfigured
// installations) or when get_free_space reports a negative result.
//
// Two RPC round-trips is acceptable here: FreeSpace is invoked from
// the periodic health checker rather than per-request hot paths.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	var location string
	if err := c.call(ctx, "core.get_config_value",
		[]any{"download_location"}, &location); err != nil {
		return -1, err
	}
	if location == "" {
		return -1, nil
	}
	var bytes int64
	if err := c.call(ctx, "core.get_free_space",
		[]any{location}, &bytes); err != nil {
		return -1, err
	}
	if bytes < 0 {
		return -1, nil
	}
	return bytes, nil
}

// Compile-time guard: keep the Client honest about implementing the
// downloads contract. If a method drifts the build breaks here, not
// at registry hydrate time.
var _ downloads.DownloadClient = (*Client)(nil)
