package rss

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestFeedSource is a mock feed source for testing.
type TestFeedSource struct {
	id       string
	name     string
	interval time.Duration
	items    []*Item
	err      error
}

func (t *TestFeedSource) ID() string                     { return t.id }
func (t *TestFeedSource) Name() string                   { return t.name }
func (t *TestFeedSource) RefreshInterval() time.Duration { return t.interval }
func (t *TestFeedSource) Fetch(ctx interface{}) ([]*Item, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.items, nil
}

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	schema := `
	CREATE TABLE rss_items (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		link TEXT,
		published_at TIMESTAMP,
		source_id TEXT NOT NULL,
		guid TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		raw TEXT,
		UNIQUE(guid, source_id)
	);
	CREATE INDEX idx_rss_items_source_created ON rss_items(source_id, created_at DESC);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func setupTestLogger(t *testing.T) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors to reduce test noise
	}
	return slog.New(slog.NewTextHandler(nil, opts))
}

// TestStoreItemsDeduplication tests that duplicate items (same GUID+SourceID) are not stored twice.
func TestStoreItemsDeduplication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	ctx := context.Background()

	item1 := &Item{
		Title:       "Test Release",
		Link:        "http://example.com/1",
		PublishedAt: time.Now().UTC(),
		SourceID:    "source1",
		GUID:        "guid-001",
	}

	// First store: should add 1 item
	stored, deduped, err := storage.StoreItems(ctx, []*Item{item1})
	if err != nil {
		t.Fatalf("store items: %v", err)
	}
	if stored != 1 || deduped != 0 {
		t.Errorf("first store: stored=%d deduped=%d, want 1 and 0", stored, deduped)
	}

	// Second store with same GUID+SourceID: should dedupe
	stored, deduped, err = storage.StoreItems(ctx, []*Item{item1})
	if err != nil {
		t.Fatalf("store items again: %v", err)
	}
	if stored != 0 || deduped != 1 {
		t.Errorf("second store: stored=%d deduped=%d, want 0 and 1", stored, deduped)
	}

	// Verify total items in DB is still 1
	items, err := storage.GetRecentItems(ctx, 100, "")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item in db, got %d", len(items))
	}
}

// TestStoreItemsDifferentSources tests that same GUID from different sources is stored separately.
func TestStoreItemsDifferentSources(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	ctx := context.Background()

	item := &Item{
		Title:       "Test Release",
		Link:        "http://example.com/1",
		PublishedAt: time.Now().UTC(),
		GUID:        "same-guid",
	}

	item1 := *item
	item1.SourceID = "source1"

	item2 := *item
	item2.SourceID = "source2"

	// Store same GUID from two different sources
	stored, deduped, err := storage.StoreItems(ctx, []*Item{&item1, &item2})
	if err != nil {
		t.Fatalf("store items: %v", err)
	}
	if stored != 2 || deduped != 0 {
		t.Errorf("store: stored=%d deduped=%d, want 2 and 0", stored, deduped)
	}

	items, err := storage.GetRecentItems(ctx, 100, "")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items in db, got %d", len(items))
	}
}

// TestGetRecentItems tests retrieval of recent items with limit.
func TestGetRecentItems(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	ctx := context.Background()

	now := time.Now().UTC()
	items := []*Item{
		{Title: "Item 1", GUID: "guid-1", SourceID: "source1", PublishedAt: now.Add(-1 * time.Hour), CreatedAt: now.Add(-1 * time.Hour)},
		{Title: "Item 2", GUID: "guid-2", SourceID: "source1", PublishedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour)},
		{Title: "Item 3", GUID: "guid-3", SourceID: "source1", PublishedAt: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour)},
	}

	stored, _, err := storage.StoreItems(ctx, items)
	if err != nil {
		t.Fatalf("store items: %v", err)
	}
	if stored != 3 {
		t.Errorf("stored: %d, want 3", stored)
	}

	// Retrieve with limit
	retrieved, err := storage.GetRecentItems(ctx, 2, "")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(retrieved) != 2 {
		t.Errorf("retrieved: %d items, want 2", len(retrieved))
	}

	// Verify order (most recent first, based on created_at DESC)
	if retrieved[0].Title != "Item 1" {
		t.Errorf("first item: %s, want Item 1", retrieved[0].Title)
	}
	if retrieved[1].Title != "Item 2" {
		t.Errorf("second item: %s, want Item 2", retrieved[1].Title)
	}
}

// TestGetSourceItems tests retrieval of items filtered by source.
func TestGetSourceItems(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	ctx := context.Background()

	items := []*Item{
		{Title: "Source1-Item1", GUID: "guid-1", SourceID: "source1"},
		{Title: "Source1-Item2", GUID: "guid-2", SourceID: "source1"},
		{Title: "Source2-Item1", GUID: "guid-3", SourceID: "source2"},
	}

	storage.StoreItems(ctx, items)

	// Get items from source1
	retrieved, err := storage.GetRecentItems(ctx, 100, "source1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(retrieved) != 2 {
		t.Errorf("source1 items: %d, want 2", len(retrieved))
	}

	// Get items from source2
	retrieved, err = storage.GetRecentItems(ctx, 100, "source2")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(retrieved) != 1 {
		t.Errorf("source2 items: %d, want 1", len(retrieved))
	}
}

// TestSyncManager tests the sync orchestrator.
func TestSyncManager(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	logger := setupTestLogger(t)
	manager := NewSyncManager(storage, logger)

	ctx := context.Background()

	// Create test sources
	source1 := &TestFeedSource{
		id:   "source1",
		name: "Test Source 1",
		items: []*Item{
			{Title: "Item 1", GUID: "guid-1", SourceID: "source1", Link: "http://example.com/1"},
			{Title: "Item 2", GUID: "guid-2", SourceID: "source1", Link: "http://example.com/2"},
		},
	}

	source2 := &TestFeedSource{
		id:   "source2",
		name: "Test Source 2",
		items: []*Item{
			{Title: "Item 3", GUID: "guid-3", SourceID: "source2", Link: "http://example.com/3"},
		},
	}

	manager.RegisterSource(source1)
	manager.RegisterSource(source2)

	// Sync all sources
	err := manager.SyncFeeds(ctx)
	if err != nil {
		t.Fatalf("sync feeds: %v", err)
	}

	// Verify stats
	stats := manager.GetStats()
	if stats.TotalSyncs != 1 {
		t.Errorf("total syncs: %d, want 1", stats.TotalSyncs)
	}
	if stats.ItemsStored != 3 {
		t.Errorf("items stored: %d, want 3", stats.ItemsStored)
	}

	// Retrieve items
	items, err := manager.GetRecentItems(ctx, 100)
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("items: %d, want 3", len(items))
	}
}

// TestCleanupOldItems tests retention policy cleanup.
func TestCleanupOldItems(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	ctx := context.Background()

	now := time.Now().UTC()
	items := []*Item{
		{Title: "Recent", GUID: "guid-1", SourceID: "source1", PublishedAt: now, CreatedAt: now},
		{Title: "Old", GUID: "guid-2", SourceID: "source1", PublishedAt: now.Add(-30 * 24 * time.Hour), CreatedAt: now.Add(-30 * 24 * time.Hour)},
	}

	storage.StoreItems(ctx, items)

	// Delete items older than 7 days
	deleted, err := storage.DeleteOldItems(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("delete old: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted: %d, want 1", deleted)
	}

	// Verify only recent item remains
	retrieved, err := storage.GetRecentItems(ctx, 100, "")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(retrieved) != 1 {
		t.Errorf("remaining items: %d, want 1", len(retrieved))
	}
	if retrieved[0].Title != "Recent" {
		t.Errorf("remaining item: %s, want Recent", retrieved[0].Title)
	}
}

// TestSyncManagerEmpty tests syncing with no sources registered.
func TestSyncManagerEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	logger := setupTestLogger(t)
	manager := NewSyncManager(storage, logger)

	ctx := context.Background()

	// Sync with no sources (should not error)
	err := manager.SyncFeeds(ctx)
	if err != nil {
		t.Errorf("sync with no sources: %v", err)
	}

	stats := manager.GetStats()
	if stats.ItemsStored != 0 {
		t.Errorf("items stored: %d, want 0", stats.ItemsStored)
	}
}

// TestUnregisterSource tests removing a source.
func TestUnregisterSource(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)
	logger := slog.New(slog.NewTextHandler(nil, nil))
	manager := NewSyncManager(storage, logger)

	source := &TestFeedSource{id: "test", name: "Test"}
	manager.RegisterSource(source)
	manager.UnregisterSource("test")

	ctx := context.Background()
	err := manager.SyncFeeds(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Should complete without syncing anything
}
