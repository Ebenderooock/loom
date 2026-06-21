package imports

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE import_history (
		id TEXT PRIMARY KEY,
		media_type TEXT,
		media_id TEXT,
		source_path TEXT,
		dest_path TEXT,
		import_mode TEXT,
		status TEXT,
		error TEXT,
		imported_at DATETIME
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAlreadyImported_RejectsKnownFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)

	// Insert a previous successful import.
	_, err := db.Exec(
		`INSERT INTO import_history (id, source_path, status) VALUES (?, ?, ?)`,
		"test-id", "/downloads/movie.mkv", string(StatusImported),
	)
	if err != nil {
		t.Fatal(err)
	}

	spec := NewAlreadyImportedSpec(db)
	candidate := &ImportCandidate{SourcePath: "/downloads/movie.mkv"}
	rejection := spec.IsSatisfiedBy(context.Background(), candidate)
	if rejection == nil {
		t.Fatal("expected rejection for already-imported file")
	}
	if rejection.Reason != RejectionAlreadyImported {
		t.Fatalf("expected reason %q, got %q", RejectionAlreadyImported, rejection.Reason)
	}
}

func TestAlreadyImported_AllowsNewFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)

	spec := NewAlreadyImportedSpec(db)
	candidate := &ImportCandidate{SourcePath: "/downloads/new-movie.mkv"}
	rejection := spec.IsSatisfiedBy(context.Background(), candidate)
	if rejection != nil {
		t.Fatalf("expected no rejection for new file, got: %s", rejection.Message)
	}
}

func TestAlreadyImported_AllowsFailedImport(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)

	// Insert a FAILED import — should not block re-import.
	_, err := db.Exec(
		`INSERT INTO import_history (id, source_path, status) VALUES (?, ?, ?)`,
		"test-id", "/downloads/movie.mkv", string(StatusFailed),
	)
	if err != nil {
		t.Fatal(err)
	}

	spec := NewAlreadyImportedSpec(db)
	candidate := &ImportCandidate{SourcePath: "/downloads/movie.mkv"}
	rejection := spec.IsSatisfiedBy(context.Background(), candidate)
	if rejection != nil {
		t.Fatalf("expected no rejection for previously-failed import, got: %s", rejection.Message)
	}
}

func TestAlreadyImported_NilDB(t *testing.T) {
	t.Parallel()
	spec := NewAlreadyImportedSpec(nil)
	candidate := &ImportCandidate{SourcePath: "/downloads/movie.mkv"}
	rejection := spec.IsSatisfiedBy(context.Background(), candidate)
	if rejection != nil {
		t.Fatal("expected no rejection when DB is nil")
	}
}
