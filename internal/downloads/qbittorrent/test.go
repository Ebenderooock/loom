package qbittorrent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Test implements downloads.DownloadClient. When credentials are set,
// it performs a fresh login and then fetches /api/v2/app/version. For
// whitelist-based setups with no credentials, it probes app/version
// directly.
func (c *Client) Test(ctx context.Context) error {
	var loginErr error
	if c.hasCredentials() {
		loginErr = c.login(ctx, true)
	}
	var (
		body []byte
		err  error
	)
	if loginErr == nil {
		body, err = c.get(ctx, "app/version", nil)
		if err != nil {
			return fmt.Errorf("qbittorrent: app/version probe failed: %w", err)
		}
	} else {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("app/version"), nil)
		if reqErr != nil {
			return fmt.Errorf("qbittorrent: app/version probe failed: %w", reqErr)
		}
		var status int
		body, status, err = c.roundTrip(ctx, req)
		if err != nil {
			return fmt.Errorf("qbittorrent: app/version probe failed: %w", err)
		}
		if status < 200 || status >= 300 {
			return loginErr
		}
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("%w: app/version returned an empty body", ErrServer)
	}
	return nil
}
