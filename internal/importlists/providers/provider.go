package providers

import "context"

// ListProvider fetches items from an external list source.
type ListProvider interface {
	// Fetch retrieves items from the provider. The list config supplies
	// URL, API key, access token, and JSON settings.
	Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error)
}

// ProviderConfig contains all fields a provider might need.
type ProviderConfig struct {
	URL         string
	APIKey      string
	AccessToken string
	Settings    string // JSON
}

// Item is the provider-agnostic result.
type Item struct {
	ExternalID string
	Title      string
	Year       int
	IMDbID     string
	TMDbID     string
	TVDbID     string
	MediaType  string // "movie" or "series"; empty defaults to list-level setting
}
