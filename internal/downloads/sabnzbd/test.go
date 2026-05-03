package sabnzbd

import (
	"context"
	"fmt"
	"strings"
)

// Test confirms reachability and apikey validity by calling
// mode=version. The endpoint returns {"version":"3.7.2"} on
// success; anything else (auth envelope, transport failure) is
// surfaced as a typed error.
func (c *Client) Test(ctx context.Context) error {
	var resp struct {
		Version string `json:"version"`
	}
	if err := c.getJSON(ctx, c.endpoint("version", nil), &resp); err != nil {
		return err
	}
	if strings.TrimSpace(resp.Version) == "" {
		return fmt.Errorf("%w: SABnzbd version probe returned empty payload", ErrServer)
	}
	return nil
}
