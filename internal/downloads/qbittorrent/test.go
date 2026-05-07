package qbittorrent

import (
	"context"
	"fmt"
	"strings"
)

// Test implements downloads.DownloadClient. It performs a fresh
// login and then fetches /api/v2/app/version. Both round-trips are
// required: login proves the credentials are still valid; the
// app/version call proves the SID cookie is being honoured by the
// authenticated endpoints. Returns nil on success.
func (c *Client) Test(ctx context.Context) error {
	if err := c.login(ctx, true); err != nil {
		return err
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
