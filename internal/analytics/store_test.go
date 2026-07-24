package analytics

import (
	"context"
	"testing"
	"time"
)

func TestStore_PruneHistory(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	oldTS := time.Now().UTC().Add(-120 * 24 * time.Hour)
	newTS := time.Now().UTC()

	if err := store.InsertOpen(ctx, HistoryRecord{
		ID:           "old",
		ConnectionID: "c1",
		Provider:     "plex",
		SessionKey:   "s1",
		MediaID:      "m1",
		StartedAt:    oldTS,
		LastSeenAt:   oldTS,
	}); err != nil {
		t.Fatalf("insert old row: %v", err)
	}
	if err := store.InsertOpen(ctx, HistoryRecord{
		ID:           "new",
		ConnectionID: "c1",
		Provider:     "plex",
		SessionKey:   "s2",
		MediaID:      "m2",
		StartedAt:    newTS,
		LastSeenAt:   newTS,
	}); err != nil {
		t.Fatalf("insert new row: %v", err)
	}

	deleted, err := store.PruneHistory(ctx, time.Now().UTC().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("PruneHistory: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	history, err := store.ListHistory(ctx, HistoryFilter{})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(history) != 1 || history[0].ID != "new" {
		t.Fatalf("expected only new row to remain, got %+v", history)
	}
}
