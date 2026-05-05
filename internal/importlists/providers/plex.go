package providers

import "context"

// PlexProvider fetches a Plex watchlist via RSS feed URL.
// Plex exposes watchlists as RSS feeds; this wraps the generic RSS provider.
type PlexProvider struct {
	rss *RSSProvider
}

// NewPlexWatchlist returns a Plex watchlist provider.
func NewPlexWatchlist() *PlexProvider {
	return &PlexProvider{rss: NewRSS()}
}

func (p *PlexProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	// Plex watchlist RSS feeds use the same format as standard RSS.
	return p.rss.Fetch(ctx, cfg)
}
