package autosearch

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupOrchestratorDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create required tables
	schema := `
		CREATE TABLE rss_items (
			id TEXT PRIMARY KEY,
			guid TEXT NOT NULL,
			source_id TEXT NOT NULL,
			title TEXT,
			link TEXT,
			published_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			raw TEXT,
			UNIQUE(guid, source_id)
		);

		CREATE TABLE search_history (
			id TEXT PRIMARY KEY,
			rss_item_id TEXT NOT NULL,
			movie_id TEXT NOT NULL,
			title TEXT NOT NULL,
			year INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'pending',
			error_msg TEXT
		);

		CREATE TABLE movies (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			year INTEGER
		);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestOrchestratorTriggerSearches(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	orch := NewOrchestrator(db, nil, matcher)

	matches := []Match{
		{
			ItemID:     "i1",
			SourceID:   "s1",
			MovieID:    "m1",
			Title:      "Inception",
			Year:       2010,
			Confidence: 0.95,
			Reason:     "exact_title_match",
		},
	}

	count, err := orch.TriggerSearches(context.Background(), matches)
	if err != nil {
		t.Fatalf("TriggerSearches failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 search triggered, got %d", count)
	}

	// Verify it was stored
	rows, err := db.Query("SELECT id, status FROM search_history WHERE movie_id = ?", "m1")
	if err != nil {
		t.Fatalf("failed to query search_history: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Error("expected search event to be stored")
	}

	var eventID, status string
	rows.Scan(&eventID, &status)

	if status != "pending" {
		t.Errorf("expected pending status, got %s", status)
	}
}

func TestOrchestratorGetSearchHistory(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	orch := NewOrchestrator(db, nil, matcher)

	// Insert search events
	event := &SearchEvent{
		ID:        "e1",
		RSSItemID: "i1",
		MovieID:   "m1",
		Title:     "Inception",
		Year:      2010,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	if err := orch.storeSearchEvent(context.Background(), event); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	// Retrieve history
	history, err := orch.GetSearchHistory(context.Background(), "m1", 10)
	if err != nil {
		t.Fatalf("GetSearchHistory failed: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 event in history, got %d", len(history))
	}

	if history[0].Status != "pending" {
		t.Errorf("expected pending status, got %s", history[0].Status)
	}
}

func TestOrchestratorUpdateSearchStatus(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	orch := NewOrchestrator(db, nil, matcher)

	// Insert an event
	event := &SearchEvent{
		ID:        "e1",
		RSSItemID: "i1",
		MovieID:   "m1",
		Title:     "Inception",
		Year:      2010,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	orch.storeSearchEvent(context.Background(), event)

	// Update status
	err := orch.UpdateSearchStatus(context.Background(), "e1", "found", "")
	if err != nil {
		t.Fatalf("UpdateSearchStatus failed: %v", err)
	}

	// Verify update
	var status string
	db.QueryRow("SELECT status FROM search_history WHERE id = ?", "e1").Scan(&status)

	if status != "found" {
		t.Errorf("expected status 'found', got %s", status)
	}
}

func TestOrchestratorGetStats(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	orch := NewOrchestrator(db, nil, matcher)

	// Insert test events
	events := []SearchEvent{
		{ID: "e1", RSSItemID: "i1", MovieID: "m1", Title: "Inception", Year: 2010, CreatedAt: time.Now(), Status: "pending"},
		{ID: "e2", RSSItemID: "i2", MovieID: "m1", Title: "Inception", Year: 2010, CreatedAt: time.Now(), Status: "found"},
		{ID: "e3", RSSItemID: "i3", MovieID: "m1", Title: "Inception", Year: 2010, CreatedAt: time.Now(), Status: "error"},
	}

	for _, event := range events {
		if err := orch.storeSearchEvent(context.Background(), &event); err != nil {
			t.Fatalf("failed to store event: %v", err)
		}
	}

	stats, err := orch.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.PendingSearches != 1 {
		t.Errorf("expected 1 pending search, got %d", stats.PendingSearches)
	}

	if stats.CompletedSearhes != 1 {
		t.Errorf("expected 1 completed search, got %d", stats.CompletedSearhes)
	}

	if stats.FailedSearches != 1 {
		t.Errorf("expected 1 failed search, got %d", stats.FailedSearches)
	}

	if stats.TotalMatches != 3 {
		t.Errorf("expected 3 total matches, got %d", stats.TotalMatches)
	}
}

func TestOrchestratorMultipleMatches(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
		{ID: "m2", Title: "Interstellar", Year: 2014},
	})

	orch := NewOrchestrator(db, nil, matcher)

	matches := []Match{
		{ItemID: "i1", SourceID: "s1", MovieID: "m1", Title: "Inception", Year: 2010, Confidence: 0.95},
		{ItemID: "i2", SourceID: "s1", MovieID: "m2", Title: "Interstellar", Year: 2014, Confidence: 0.92},
	}

	count, err := orch.TriggerSearches(context.Background(), matches)
	if err != nil {
		t.Fatalf("TriggerSearches failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 searches triggered, got %d", count)
	}

	stats, _ := orch.GetStats(context.Background())
	if stats.TotalMatches != 2 {
		t.Errorf("expected 2 total matches, got %d", stats.TotalMatches)
	}
}

func TestOrchestratorEmptyMatches(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	orch := NewOrchestrator(db, nil, matcher)

	count, err := orch.TriggerSearches(context.Background(), []Match{})
	if err != nil {
		t.Fatalf("TriggerSearches failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 searches for empty matches, got %d", count)
	}
}

func TestOrchestratorDuplicateMatches(t *testing.T) {
	db := setupOrchestratorDB(t)
	defer db.Close()

	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	orch := NewOrchestrator(db, nil, matcher)

	// Same match triggered twice (shouldn't create duplicates due to primary key)
	match := Match{
		ItemID:     "i1",
		SourceID:   "s1",
		MovieID:    "m1",
		Title:      "Inception",
		Year:       2010,
		Confidence: 0.95,
	}

	count1, err := orch.TriggerSearches(context.Background(), []Match{match})
	if err != nil {
		t.Fatalf("first TriggerSearches failed: %v", err)
	}

	if count1 != 1 {
		t.Errorf("expected 1 search first time, got %d", count1)
	}

	// Second call with same match - should silently fail (UNIQUE constraint)
	// This tests idempotency: calling twice with same data should not increase count
	_, _ = orch.TriggerSearches(context.Background(), []Match{match})

	// Verify only one event exists (primary key prevents duplicates)
	var count int
	db.QueryRow("SELECT COUNT(*) FROM search_history WHERE movie_id = ?", "m1").Scan(&count)

	if count != 1 {
		t.Errorf("expected 1 event in DB, got %d (idempotency test)", count)
	}
}
