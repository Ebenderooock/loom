package indexers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/throttle"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func openTestDB(t *testing.T) (storage.DB, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "loom.db")},
	}
	db, err := storage.Open(context.Background(), cfg, quietLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db, db.DB()
}

func TestSQLiteRepositoryCRUD(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	ctx := context.Background()

	def := indexers.Definition{
		ID:         "null-1",
		Kind:       indexers.KindNull,
		Name:       "Null One",
		Enabled:    true,
		Priority:   25,
		Config:     json.RawMessage(`{"hello":"world"}`),
		Categories: []indexers.Category{indexers.CategoryMovies, indexers.CategoryTV},
		Tags:       []string{"public", "free"},
	}
	saved, err := repo.Create(ctx, def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.ID != def.ID || string(saved.Kind) != string(def.Kind) {
		t.Fatalf("Create returned wrong row: %#v", saved)
	}
	if len(saved.Categories) != 2 || saved.Categories[0] != indexers.CategoryMovies {
		t.Fatalf("Categories round-trip failed: %#v", saved.Categories)
	}
	if len(saved.Tags) != 2 || saved.Tags[1] != "free" {
		t.Fatalf("Tags round-trip failed: %#v", saved.Tags)
	}

	got, err := repo.Get(ctx, "null-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Null One" {
		t.Fatalf("Get name: %q", got.Name)
	}

	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("List: %v len=%d", err, len(all))
	}

	name := "Renamed"
	patched, err := repo.Patch(ctx, indexers.Patch{ID: "null-1", Name: &name})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if patched.Name != "Renamed" {
		t.Fatalf("Patch did not apply: %q", patched.Name)
	}

	disabled := false
	_, err = repo.Patch(ctx, indexers.Patch{ID: "null-1", Enabled: &disabled})
	if err != nil {
		t.Fatalf("Patch enabled: %v", err)
	}
	enabled, err := repo.ListEnabled(ctx)
	if err != nil || len(enabled) != 0 {
		t.Fatalf("ListEnabled: %v len=%d", err, len(enabled))
	}

	if err := repo.Delete(ctx, "null-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "null-1"); err == nil {
		t.Fatal("expected ErrNotFound after Delete")
	}
}

func TestSQLiteRepositoryRateLimitRoundTrip(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	ctx := context.Background()

	if _, err := repo.Create(ctx, indexers.Definition{
		ID: "rl-1", Kind: indexers.KindNull, Name: "RL One", Enabled: true, Priority: 25,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Default state: every column is NULL → repository returns
	// zero-value Config so Resolve() picks the package defaults.
	got, err := repo.GetRateLimit(ctx, "rl-1")
	if err != nil {
		t.Fatalf("GetRateLimit (initial): %v", err)
	}
	if got.PerMinute != 0 || got.Burst != 0 || got.MaxRetries != -1 {
		t.Fatalf("expected zero/sentinel values, got %#v", got)
	}

	want := throttle.Config{PerMinute: 30, Burst: 2, MaxRetries: 5}
	if err := repo.SetRateLimit(ctx, "rl-1", want); err != nil {
		t.Fatalf("SetRateLimit: %v", err)
	}
	got, err = repo.GetRateLimit(ctx, "rl-1")
	if err != nil {
		t.Fatalf("GetRateLimit (after set): %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %#v want %#v", got, want)
	}

	// Setting MaxRetries=0 must persist as 0, not "use default".
	zero := throttle.Config{PerMinute: 30, Burst: 2, MaxRetries: 0}
	if err := repo.SetRateLimit(ctx, "rl-1", zero); err != nil {
		t.Fatalf("SetRateLimit zero: %v", err)
	}
	got, err = repo.GetRateLimit(ctx, "rl-1")
	if err != nil {
		t.Fatalf("GetRateLimit (after zero): %v", err)
	}
	if got.MaxRetries != 0 {
		t.Fatalf("expected MaxRetries=0 to persist, got %d", got.MaxRetries)
	}
}

func TestServiceCreateAndHydrate(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	svc, err := indexers.NewService(indexers.ServiceOptions{Repository: repo, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := context.Background()

	def := indexers.Definition{
		ID:      "n",
		Kind:    indexers.KindNull,
		Name:    "Null",
		Enabled: true,
	}
	if _, err := svc.Create(ctx, def); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := svc.Registry().Get("n"); !ok {
		t.Fatal("registry not populated after Create")
	}

	h, err := svc.TestOne(ctx, "n")
	if err != nil {
		t.Fatalf("TestOne: %v", err)
	}
	if h.Status != indexers.StatusOK {
		t.Fatalf("status: %s", h.Status)
	}
}

func TestServiceUnknownKindRejected(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	svc, _ := indexers.NewService(indexers.ServiceOptions{Repository: repo, Logger: quietLogger()})
	_, err := svc.Create(context.Background(), indexers.Definition{
		ID: "x", Kind: "imaginary", Name: "x",
	})
	if err == nil {
		t.Fatal("expected unknown kind error")
	}
}
