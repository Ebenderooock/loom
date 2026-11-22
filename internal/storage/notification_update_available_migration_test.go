package storage

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/pressly/goose/v3"
)

func TestNotificationUpdateAvailableMigration(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := openSQLite(ctx, config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "notifications.db")}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrations, dialect, err := migrationsFor(EngineSQLite)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.Dialect(dialect), db.DB(), migrations, goose.WithLogger(gooseSlogLogger{logger: logger}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 106); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO notification_connections
		(id, name, type, on_application_update, created_at, updated_at)
		VALUES ('existing', 'Existing', 'webhook', 1, '2026-01-01', '2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 107); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		var available, updated bool
		if err := db.DB().QueryRowContext(ctx, `SELECT on_update_available, on_application_update FROM notification_connections WHERE id = 'existing'`).Scan(&available, &updated); err != nil {
			t.Fatal(err)
		}
		if available || !updated {
			t.Fatalf("migration changed existing subscriptions: available=%v updated=%v", available, updated)
		}
	}
	check()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO notification_connections
		(id, name, type, created_at, updated_at) VALUES ('new', 'New', 'webhook', '2026-01-01', '2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	var available bool
	if err := db.DB().QueryRowContext(ctx, `SELECT on_update_available FROM notification_connections WHERE id = 'new'`).Scan(&available); err != nil || available {
		t.Fatalf("new connection default: available=%v err=%v", available, err)
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 107); err != nil {
		t.Fatal(err)
	}
	check()
}
