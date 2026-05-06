package connect

import (
	"context"
	"fmt"
	"net/http"
)

// traktProvider implements the Provider interface for Trakt.tv.
type traktProvider struct{}

func (t *traktProvider) Test(ctx context.Context, s ProviderSettings) error {
	if s.AccessToken == "" {
		return fmt.Errorf("trakt test: access_token is required")
	}
	if s.ClientID == "" {
		return fmt.Errorf("trakt test: client_id is required")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.trakt.tv/users/me", nil)
	if err != nil {
		return fmt.Errorf("trakt test: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", s.ClientID)
	req.Header.Set("Authorization", "Bearer "+s.AccessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trakt test: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("trakt test: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (t *traktProvider) NotifyLibraryUpdate(_ context.Context, _ ProviderSettings) error {
	// Trakt doesn't need library refresh notifications.
	return nil
}
