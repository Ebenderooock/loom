package downloads_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/storage"
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
