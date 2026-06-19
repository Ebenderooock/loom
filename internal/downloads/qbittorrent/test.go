package qbittorrent

import (
	"context"
	"fmt"
	"strings"
)

// Test implements downloads.DownloadClient. When credentials are set,
// it performs a fresh login and then fetches /api/v2/app/version. For
// whitelist-based setups with no credentials, it probes app/version
// directly.
func (c *Client) Test(ctx context.Context) error {
	if c.hasCredentials() {
		if err := c.login(ctx, true); err != nil {
			return err
		}
	}
	body, err := c.get(ctx, "app/version", nil)
	if err != nil {
		return fmt.Errorf("qbittorrent: app/version probe failed: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("%w: app/version returned an empty body", ErrServer)
	}
	return nil
}
