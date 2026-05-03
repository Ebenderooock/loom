package transmission

import (
	"context"
	"fmt"
	"strings"
)

// sessionInfo is the minimal slice of session-get we read in Test.
// Transmission always populates these on a healthy daemon, so empty
// values surface as a typed ErrServer.
type sessionInfo struct {
	Version   string `json:"version"`
	RPCVer    int    `json:"rpc-version"`
	RPCMin    int    `json:"rpc-version-minimum"`
	SessionID string `json:"session-id"`
}

// Test implements downloads.DownloadClient. It performs a single
// session-get round trip; the call inherently exercises the
// session-id handshake (when needed) and HTTP Basic credentials, so
// the returned nil is a strong "credentials and reachability are
// good" signal.
func (c *Client) Test(ctx context.Context) error {
	var info sessionInfo
	if err := c.call(ctx, "session-get", nil, &info); err != nil {
		return err
	}
	if strings.TrimSpace(info.Version) == "" {
		return fmt.Errorf("%w: session-get returned an empty version", ErrServer)
	}
	return nil
}
