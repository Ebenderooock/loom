package deluge

import (
	"context"
	"fmt"
	"strings"
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
//   - daemon.get_version (or daemon.info on 1.3) proves the SID cookie is being honoured by the
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

	// Deluge 2.x exports daemon.get_version; 1.3 only has daemon.info.
	method := "daemon.get_version"
	var version string
	err := c.call(ctx, method, nil, &version)
	if err != nil && isUnknownMethod(err) {
		method = "daemon.info"
		err = c.call(ctx, method, nil, &version)
	}
	if err != nil {
		return fmt.Errorf("deluge: %s probe failed: %w", method, err)
	}
	if version == "" {
		return fmt.Errorf("%w: %s returned an empty version", ErrServer, method)
	}
	return nil
}

func isUnknownMethod(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unknown method")
}
