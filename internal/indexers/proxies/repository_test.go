package proxies_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/proxies"
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
	repo := proxies.NewSQLiteRepository(raw)
	ctx := context.Background()

	cfg := json.RawMessage(`{"url":"http://proxy.example:8080"}`)
	created, err := repo.Create(ctx, proxies.Proxy{
		ID:      "http-test",
		Kind:    proxies.KindHTTP,
		Name:    "Test",
		Enabled: true,
		Config:  cfg,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "http-test" || created.Name != "Test" {
		t.Fatalf("unexpected create row: %+v", created)
	}

	got, err := repo.Get(ctx, "http-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != proxies.KindHTTP {
		t.Fatalf("kind mismatch: %s", got.Kind)
	}

	rows, err := repo.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("List: %v rows=%d", err, len(rows))
	}

	newName := "Renamed"
	enabled := false
	patched, err := repo.Patch(ctx, proxies.Patch{ID: "http-test", Name: &newName, Enabled: &enabled})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if patched.Name != "Renamed" || patched.Enabled {
		t.Fatalf("patch not applied: %+v", patched)
	}

	if err := repo.Delete(ctx, "http-test"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "http-test"); err == nil {
		t.Fatal("expected ErrNotFound after delete")
	}
}

func TestServiceCreateValidatesConfig(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := proxies.NewSQLiteRepository(raw)
	svc, err := proxies.NewService(proxies.ServiceOptions{Repository: repo, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Missing URL → invalid_config.
	if _, err := svc.Create(context.Background(), proxies.Proxy{
		Kind:   proxies.KindHTTP,
		Name:   "broken",
		Config: json.RawMessage(`{}`),
	}); err == nil {
		t.Fatal("expected error for missing url")
	}

	// Unknown kind.
	if _, err := svc.Create(context.Background(), proxies.Proxy{
		Kind:   "weird",
		Name:   "x",
		Config: json.RawMessage(`{}`),
	}); err == nil {
		t.Fatal("expected error for unknown kind")
	}

	// Happy path generates ID.
	out, err := svc.Create(context.Background(), proxies.Proxy{
		Kind:    proxies.KindHTTP,
		Name:    "Real Proxy",
		Enabled: true,
		Config:  json.RawMessage(`{"url":"http://p.example:8080"}`),
	})
	if err != nil {
		t.Fatalf("Create happy: %v", err)
	}
	if out.ID == "" {
		t.Fatal("expected generated id")
	}
}

func TestDeleteRefusesWhenInUse(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	proxyRepo := proxies.NewSQLiteRepository(raw)
	idxRepo := indexers.NewSQLiteRepository(raw)
	ctx := context.Background()

	p, err := proxyRepo.Create(ctx, proxies.Proxy{
		ID: "p1", Kind: proxies.KindHTTP, Name: "p1",
		Enabled: true,
		Config:  json.RawMessage(`{"url":"http://x:8080"}`),
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if _, err := idxRepo.Create(ctx, indexers.Definition{
		ID:      "idx1",
		Kind:    indexers.KindNull,
		Name:    "Test",
		Enabled: true,
		Config:  json.RawMessage(`{}`),
		ProxyID: p.ID,
	}); err != nil {
		t.Fatalf("create indexer: %v", err)
	}

	users, err := proxyRepo.IndexerIDsUsing(ctx, "p1")
	if err != nil {
		t.Fatalf("IndexerIDsUsing: %v", err)
	}
	if len(users) != 1 || users[0] != "idx1" {
		t.Fatalf("unexpected users: %v", users)
	}

	svc, _ := proxies.NewService(proxies.ServiceOptions{Repository: proxyRepo, Logger: quietLogger()})
	err = svc.Delete(ctx, "p1")
	var inUse *proxies.ErrInUse
	if err == nil {
		t.Fatal("expected ErrInUse")
	}
	if !errors.As(err, &inUse) {
		t.Fatalf("expected ErrInUse, got %T: %v", err, err)
	}
	if len(inUse.IndexerIDs) != 1 || inUse.IndexerIDs[0] != "idx1" {
		t.Fatalf("unexpected ErrInUse payload: %+v", inUse)
	}
}

// errorsAs is a tiny inline shim to avoid an extra import in the
// happy-path test file.
func errorsAs(err error, target any) bool {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if asTarget(err, target) {
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}

func asTarget(err error, target any) bool {
	if t, ok := target.(**proxies.ErrInUse); ok {
		if e, ok := err.(*proxies.ErrInUse); ok {
			*t = e
			return true
		}
	}
	return false
}
