package downloads

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func openHistoryTestStore(t *testing.T) *HistoryStore {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "loom.db")},
	}
	db, err := storage.Open(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate storage: %v", err)
	}
	return NewHistoryStore(db.DB())
}

func TestHistoryStoreRecordsCompletedAndFailed(t *testing.T) {
	t.Parallel()
	store := openHistoryTestStore(t)
	ctx := context.Background()

	completedAt := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	if err := store.RecordCompletion(ctx, &DownloadCompletedEvent{
		DownloadID:  "dl-complete-1",
		ClientID:    "client-1",
		Title:       "Completed Download",
		Category:    "movies",
		CompletedAt: completedAt,
	}); err != nil {
		t.Fatalf("record completion: %v", err)
	}

	failedAt := completedAt.Add(5 * time.Minute)
	if err := store.RecordFailure(ctx, "client-1", "dl-failed-1", "Failed Download", "tv", failedAt); err != nil {
		t.Fatalf("record failure: %v", err)
	}

	if !store.WasCompleted(ctx, "client-1", "dl-complete-1") {
		t.Fatalf("expected completed download to be recorded")
	}
	if store.WasCompleted(ctx, "client-1", "dl-failed-1") {
		t.Fatalf("failed download should not be treated as completed")
	}
	if !store.WasRecorded(ctx, "client-1", "dl-failed-1", "failed") {
		t.Fatalf("expected failed download to be recorded with failed status")
	}

	entries, err := store.List(ctx, 50, 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(entries))
	}
}
