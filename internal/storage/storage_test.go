package storage_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/storage"
	dbpg "github.com/loomctl/loom/internal/storage/db/postgres"
	dbsqlite "github.com/loomctl/loom/internal/storage/db/sqlite"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func openSQLiteForTest(t *testing.T) storage.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "loom_test.db")
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: path},
	}
	db, err := storage.Open(context.Background(), cfg, quietLogger())
	if err != nil {
		t.Fatalf("Open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSQLiteOpenAndMigrate(t *testing.T) {
	db := openSQLiteForTest(t)
	if got := db.Engine(); got != storage.EngineSQLite {
		t.Errorf("Engine() = %q, want sqlite", got)
	}
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	want := []string{"api_keys", "scheduled_jobs", "schema_meta", "users"}
	got := listSQLiteTables(t, db.DB())
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("expected table %q in %v", w, got)
		}
	}

	// Idempotency: a second Migrate must succeed with no error.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestSQLiteRoundTrip(t *testing.T) {
	db := openSQLiteForTest(t)
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	q := dbsqlite.New(db.DB())
	created, err := q.CreateUser(ctx, dbsqlite.CreateUserParams{
		Username:     "alice",
		PasswordHash: "hash",
		Email:        sql.NullString{String: "a@example.com", Valid: true},
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Username != "alice" || created.Role != "admin" {
		t.Errorf("created user mismatch: %+v", created)
	}

	got, err := q.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("round-trip ID mismatch: got %d, created %d", got.ID, created.ID)
	}
}

func TestPostgresOpenAndMigrate(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run")
	}
	cfg := config.StorageConfig{
		Engine:   "postgres",
		Postgres: config.PostgresConfig{DSN: dsn},
	}
	ctx := context.Background()
	db, err := storage.Open(ctx, cfg, quietLogger())
	if err != nil {
		t.Fatalf("Open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("idempotent Migrate: %v", err)
	}

	// Smoke a round-trip.
	q := dbpg.New(db.DB())
	created, err := q.CreateUser(ctx, dbpg.CreateUserParams{
		Username:     "alice_pg_" + t.Name(),
		PasswordHash: "hash",
		Email:        sql.NullString{String: "a@example.com", Valid: true},
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	got, err := q.GetUserByUsername(ctx, created.Username)
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("round-trip ID mismatch")
	}
}

func TestPostgresRequiresDSN(t *testing.T) {
	cfg := config.StorageConfig{Engine: "postgres"}
	if _, err := storage.Open(context.Background(), cfg, quietLogger()); err == nil {
		t.Errorf("expected error for empty DSN, got nil")
	}
}

func listSQLiteTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	sort.Strings(out)
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
