// Package rss provides RSS feed monitoring and sync functionality for Phase 5e-a.
//
// This package handles:
//  - Fetching and parsing RSS feeds from configured sources (indexers and user sources)
//  - Deduplication using GUID and source ID
//  - Storage in the rss_items table
//  - Periodic sync orchestration with configurable refresh intervals
//  - Metrics for feed health monitoring
//
// RSS items are normalized into a uniform interface, enabling both RSS feeds and
// web scraper sources to be consumed by the auto-search subsystem (Phase 5e-b).
//
// Example usage:
//
//	manager := rss.NewSyncManager(db, logger, indexerRegistry)
//	ctx := context.Background()
//	if err := manager.SyncFeeds(ctx); err != nil {
//		log.Fatalf("sync failed: %v", err)
//	}
//	items, err := manager.GetRecentItems(ctx, 100)
package rss

import (
	"time"
)

// Item represents a normalized RSS item from any feed source.
type Item struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	PublishedAt time.Time `json:"published_at"`
	SourceID    string    `json:"source_id"`
	SourceName  string    `json:"source_name"`
	GUID        string    `json:"guid"`
	Raw         string    `json:"raw"`
	CreatedAt   time.Time `json:"created_at"`
}

// FeedSource abstracts different RSS source types (indexer feeds, user RSS, scrapers).
type FeedSource interface {
	// ID returns a stable identifier for this source.
	ID() string

	// Name returns a human-readable name.
	Name() string

	// Fetch retrieves items from the source.
	// ctx should be a context.Context; implementations may accept interface{} for flexibility.
	// Returns []*Item (normalized), error if fetch fails.
	Fetch(ctx interface{}) ([]*Item, error)

	// RefreshInterval returns how often to sync this source.
	RefreshInterval() time.Duration
}

// Stats tracks RSS sync health metrics.
type Stats struct {
	TotalSyncs      int64
	SuccessfulSyncs int64
	FailedSyncs     int64
	LastSyncAt      time.Time
	LastSyncError   string
	ItemsStored     int64
	ItemsDeduped    int64
}
