package nzbget

import (
	"context"
	"fmt"
	"strings"
)

// Test confirms reachability and ControlUsername/ControlPassword
// validity by calling the JSON-RPC `version()` method. NZBGet
// returns its semver as a string ("21.1", "22.0"); anything else —
// auth failure, transport error, empty payload — is surfaced as a
// typed error.
func (c *Client) Test(ctx context.Context) error {
	var version string
	if err := c.call(ctx, "version", nil, &version); err != nil {
		return err
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("%w: NZBGet version probe returned empty payload", ErrServer)
	}
	return nil
}
