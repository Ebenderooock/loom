package cleanup

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "loom.db")},
	}
	db, err := storage.Open(context.Background(), cfg,
		slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db.DB()
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mkfile(t *testing.T, path string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// buildScenario creates a download root with a protected category dir holding a
// tracked download, plus a genuine orphan, and returns a wired Service.
func buildScenario(t *testing.T) (*Service, string, string) {
	t.Helper()
	db := openTestDB(t)
	store := NewStore(db)

	root := t.TempDir()
	// Tracked download nested under a category dir — the category dir must be
	// protected because it contains tracked content.
	trackedFile := filepath.Join(root, "tv", "Show S01", "ep01.mkv")
	mkfile(t, trackedFile)
	// A genuine orphan directory with junk.
	orphanDir := filepath.Join(root, "old-release")
	mkfile(t, filepath.Join(orphanDir, "junk.nfo"))

	svc := NewService(Options{
		Store: store,
		Roots: func(context.Context) ([]Root, error) {
			return []Root{{Path: root, ClientID: "c1", ClientName: "test"}}, nil
		},
		Tracked: func(context.Context) ([]string, error) {
			return []string{filepath.Join(root, "tv", "Show S01")}, nil
		},
		Recycle: func(_ context.Context, path, _ string) error {
			return os.RemoveAll(path)
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return svc, orphanDir, filepath.Join(root, "tv")
}

func TestScanFlagsOnlyOrphans(t *testing.T) {
	svc, orphanDir, categoryDir := buildScenario(t)
	ctx := context.Background()

	found, err := svc.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if found != 1 {
		t.Fatalf("expected exactly 1 orphan, got %d", found)
	}

	pending, err := svc.store.List(ctx, StatusPending)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending row, got %d", len(pending))
	}
	if pending[0].Path != orphanDir {
		t.Fatalf("orphan path = %q, want %q", pending[0].Path, orphanDir)
	}
	if pending[0].SizeBytes <= 0 {
		t.Errorf("expected non-zero orphan size, got %d", pending[0].SizeBytes)
	}

	// The category dir holding tracked content must never be flagged.
	if _, err := os.Stat(categoryDir); err != nil {
		t.Fatalf("category dir should still exist: %v", err)
	}
}

func TestScanIsIdempotent(t *testing.T) {
	svc, _, _ := buildScenario(t)
	ctx := context.Background()
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := svc.store.List(ctx, StatusPending)
	if len(pending) != 1 {
		t.Fatalf("rescan should not duplicate rows, got %d", len(pending))
	}
}

func TestApproveRecyclesOrphan(t *testing.T) {
	svc, orphanDir, _ := buildScenario(t)
	ctx := context.Background()
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := svc.store.List(ctx, StatusPending)
	if len(pending) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(pending))
	}

	if err := svc.Approve(ctx, pending[0].ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("orphan dir should be removed, stat err=%v", err)
	}
	deleted, _ := svc.store.List(ctx, StatusDeleted)
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted row, got %d", len(deleted))
	}
}

func TestScanResolvesVanishedOrphan(t *testing.T) {
	svc, orphanDir, _ := buildScenario(t)
	ctx := context.Background()
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	// Remove the orphan out-of-band, then rescan: the row should resolve away.
	if err := os.RemoveAll(orphanDir); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := svc.store.List(ctx, StatusPending)
	if len(pending) != 0 {
		t.Fatalf("vanished orphan should be resolved, got %d pending", len(pending))
	}
}

func TestApproveRevalidatesNowTracked(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	root := t.TempDir()
	orphanDir := filepath.Join(root, "release")
	mkfile(t, filepath.Join(orphanDir, "file.mkv"))

	trackedNow := false
	svc := NewService(Options{
		Store: store,
		Roots: func(context.Context) ([]Root, error) {
			return []Root{{Path: root, ClientID: "c1"}}, nil
		},
		Tracked: func(context.Context) ([]string, error) {
			if trackedNow {
				return []string{orphanDir}, nil
			}
			return nil, nil
		},
		Recycle: func(_ context.Context, path, _ string) error { return os.RemoveAll(path) },
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx := context.Background()
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := store.List(ctx, StatusPending)
	if len(pending) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(pending))
	}

	// It becomes tracked between scan and approval — must NOT be deleted.
	trackedNow = true
	if err := svc.Approve(ctx, pending[0].ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := os.Stat(orphanDir); err != nil {
		t.Fatalf("now-tracked dir must not be deleted: %v", err)
	}
}

func TestAutoDeleteRespectsRetention(t *testing.T) {
	svc, orphanDir, _ := buildScenario(t)
	ctx := context.Background()
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := svc.store.List(ctx, StatusPending)
	if len(pending) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(pending))
	}

	// Default retention is 7 days; a freshly-seen orphan must NOT auto-delete.
	deleted, err := svc.AutoDelete(ctx)
	if err != nil {
		t.Fatalf("AutoDelete: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("fresh orphan should not auto-delete, deleted=%d", deleted)
	}
	if _, err := os.Stat(orphanDir); err != nil {
		t.Fatalf("orphan should still exist: %v", err)
	}

	// Age the orphan past the retention window, then it should be removed.
	if _, err := svc.store.db.ExecContext(ctx,
		`UPDATE cleanup_orphans SET first_seen_at = ? WHERE id = ?`,
		"2000-01-01T00:00:00Z", pending[0].ID); err != nil {
		t.Fatal(err)
	}
	deleted, err = svc.AutoDelete(ctx)
	if err != nil {
		t.Fatalf("AutoDelete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("aged orphan should auto-delete, deleted=%d", deleted)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("aged orphan should be removed, stat err=%v", err)
	}
}

func TestAutoDeleteDisabledIsNoop(t *testing.T) {
	svc, orphanDir, _ := buildScenario(t)
	ctx := context.Background()
	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := svc.store.List(ctx, StatusPending)
	if err := svc.store.SaveSettings(ctx, Settings{AutoDeleteEnabled: false, RetentionDays: 7}); err != nil {
		t.Fatal(err)
	}
	// Age it, but auto-delete is off → must remain.
	if _, err := svc.store.db.ExecContext(ctx,
		`UPDATE cleanup_orphans SET first_seen_at = ? WHERE id = ?`,
		"2000-01-01T00:00:00Z", pending[0].ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.AutoDelete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("auto-delete disabled should remove nothing, deleted=%d", deleted)
	}
	if _, err := os.Stat(orphanDir); err != nil {
		t.Fatalf("orphan should still exist when auto-delete off: %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	got, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AutoDeleteEnabled || got.RetentionDays != 7 {
		t.Fatalf("default settings = %+v, want auto-delete on, 7 days", got)
	}

	if err := store.SaveSettings(ctx, Settings{AutoDeleteEnabled: false, RetentionDays: 14}); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetSettings(ctx)
	if got.AutoDeleteEnabled || got.RetentionDays != 14 {
		t.Fatalf("saved settings = %+v, want auto-delete off, 14 days", got)
	}
}

// TestScanSkipsLibraryOverlappingRoot ensures a download root that overlaps a
// media library is never scanned, so library content is never flagged.
func TestScanSkipsLibraryOverlappingRoot(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	root := t.TempDir()
	// A real media file living directly under the root.
	mkfile(t, filepath.Join(root, "Movie (2020)", "movie.mkv"))

	svc := NewService(Options{
		Store: store,
		Roots: func(context.Context) ([]Root, error) {
			return []Root{{Path: root, ClientID: "c1"}}, nil
		},
		Tracked:   func(context.Context) ([]string, error) { return nil, nil },
		Protected: func(context.Context) ([]string, error) { return []string{root}, nil },
		Recycle: func(_ context.Context, path, _ string) error {
			return os.RemoveAll(path)
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	found, err := svc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if found != 0 {
		t.Fatalf("scan must skip a library-overlapping root, found=%d", found)
	}
	pending, _ := store.List(ctx, StatusPending)
	if len(pending) != 0 {
		t.Fatalf("no orphans should be recorded for a library root, got %d", len(pending))
	}
}

// TestApproveRefusesLibraryOverlap ensures that if a library comes to overlap a
// previously-recorded orphan, approval refuses to delete it.
func TestApproveRefusesLibraryOverlap(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	root := t.TempDir()
	orphanDir := filepath.Join(root, "release")
	mkfile(t, filepath.Join(orphanDir, "file.mkv"))

	if err := store.Upsert(ctx, Orphan{Path: orphanDir, ClientID: "c1", Root: root, SizeBytes: 1}); err != nil {
		t.Fatal(err)
	}
	pending, _ := store.List(ctx, StatusPending)
	if len(pending) != 1 {
		t.Fatalf("setup: want 1 pending, got %d", len(pending))
	}

	svc := NewService(Options{
		Store: store,
		Roots: func(context.Context) ([]Root, error) {
			return []Root{{Path: root, ClientID: "c1"}}, nil
		},
		Tracked: func(context.Context) ([]string, error) { return nil, nil },
		// Library now overlaps the orphan path.
		Protected: func(context.Context) ([]string, error) { return []string{orphanDir}, nil },
		Recycle: func(_ context.Context, path, _ string) error {
			return os.RemoveAll(path)
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	if err := svc.Approve(ctx, pending[0].ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := os.Stat(orphanDir); err != nil {
		t.Fatalf("orphan overlapping a library must NOT be deleted: %v", err)
	}
}

// TestTrackedErrorAbortsScan ensures an incomplete tracked set (e.g. a
// download client that errored) aborts the scan rather than flagging live
// downloads as orphans.
func TestTrackedErrorAbortsScan(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	root := t.TempDir()
	mkfile(t, filepath.Join(root, "release", "file.mkv"))

	svc := NewService(Options{
		Store: store,
		Roots: func(context.Context) ([]Root, error) {
			return []Root{{Path: root, ClientID: "c1"}}, nil
		},
		Tracked: func(context.Context) ([]string, error) {
			return nil, context.DeadlineExceeded
		},
		Recycle: func(_ context.Context, path, _ string) error { return os.RemoveAll(path) },
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	if _, err := svc.Scan(ctx); err == nil {
		t.Fatal("scan must fail when the tracked set is unavailable")
	}
	pending, _ := store.List(ctx, StatusPending)
	if len(pending) != 0 {
		t.Fatalf("no orphans should be recorded when tracked set errored, got %d", len(pending))
	}
}
