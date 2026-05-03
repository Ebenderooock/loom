package deluge

import (
	"context"
	"fmt"
)

// Test implements downloads.DownloadClient. It performs a fresh
// login, then asks the Web UI whether it is currently bound to a
// daemon, then asks the daemon for its version. All three round-
// trips are required:
//
//   - login proves the configured password is still valid.
//   - web.connected proves the Web UI has a daemon to forward to.
//     Without this check, every operational call would fail with
//     a confusing "Not connected" RPC error well after Test passed.
//   - daemon.info proves the SID cookie is being honoured by the
//     authenticated endpoints and gives operators a version number
//     to log alongside the success.
//
// Returns nil on success.
func (c *Client) Test(ctx context.Context) error {
	if err := c.login(ctx); err != nil {
		return err
	}

	var connected bool
	if err := c.call(ctx, "web.connected", nil, &connected); err != nil {
		return err
	}
	if !connected {
		return ErrDaemonNotConnected
	}

	var version string
	if err := c.call(ctx, "daemon.info", nil, &version); err != nil {
		return fmt.Errorf("deluge: daemon.info probe failed: %w", err)
	}
	if version == "" {
		return fmt.Errorf("%w: daemon.info returned an empty version", ErrServer)
	}
	return nil
}
