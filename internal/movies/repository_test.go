package movies

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/storage"
)

func openRepositoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "movies_repo_test.db")},
	}
	db, err := storage.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db.DB()
}

func seedLibraryAndMovie(t *testing.T, ctx context.Context, raw *sql.DB, repo Repository, libraryID, movieID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := raw.ExecContext(ctx,
		`INSERT INTO libraries (id, name, path, media_type, quality_profile_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		libraryID, "Movies", "/media/movies", "movie", "qp-1", now, now,
	)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if err := repo.AddMovie(ctx, &Movie{
		ID:               movieID,
		Title:            "Test Movie",
		Status:           MovieStatusMissing,
		MonitoringStatus: MonitoringStatusMonitored,
		QualityProfileID: "qp-1",
		LibraryID:        libraryID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("add movie: %v", err)
	}
}

func TestAddMovieFileUpdatesLibraryFileMapping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	raw := openRepositoryTestDB(t)
	repo := NewRepository(raw)
	store := libraries.NewStore(raw)
	seedLibraryAndMovie(t, ctx, raw, repo, "lib-1", "movie-1")

	now := time.Now().UTC()
	path := "/media/movies/Test Movie (2025)/Test Movie (2025).mkv"
	_, err := raw.ExecContext(ctx,
		`INSERT INTO library_files (id, library_id, path, size_bytes, media_id, last_scanned, created_at)
		 VALUES (?, ?, ?, ?, NULL, ?, ?)`,
		"lf-1", "lib-1", path, 12345, now, now,
	)
	if err != nil {
		t.Fatalf("insert library file: %v", err)
	}

	before, err := store.UnmappedCount(ctx, "lib-1")
	if err != nil {
		t.Fatalf("unmapped before import: %v", err)
	}
	if before != 1 {
		t.Fatalf("unmapped before import = %d, want 1", before)
	}

	err = repo.AddMovieFile(ctx, &MovieFile{
		ID:        "mf-1",
		MovieID:   "movie-1",
		FilePath:  path,
		Size:      12345,
		Quality:   "WEB-DL",
		Format:    "x265",
		DateAdded: now,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("add movie file: %v", err)
	}

	var mediaID sql.NullString
	if err := raw.QueryRowContext(ctx, `SELECT media_id FROM library_files WHERE path = ?`, path).Scan(&mediaID); err != nil {
		t.Fatalf("query library mapping: %v", err)
	}
	if !mediaID.Valid || mediaID.String != "movie-1" {
		t.Fatalf("library_files.media_id = %q (valid=%v), want movie-1", mediaID.String, mediaID.Valid)
	}

	after, err := store.UnmappedCount(ctx, "lib-1")
	if err != nil {
		t.Fatalf("unmapped after import: %v", err)
	}
	if after != 0 {
		t.Fatalf("unmapped after import = %d, want 0", after)
	}
}

func TestUpdateMovieFileUpdatesLibraryFileMapping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	raw := openRepositoryTestDB(t)
	repo := NewRepository(raw)
	store := libraries.NewStore(raw)
	seedLibraryAndMovie(t, ctx, raw, repo, "lib-2", "movie-2")

	now := time.Now().UTC()
	path := "/media/movies/Another Movie (2024)/Another Movie (2024).mkv"
	_, err := raw.ExecContext(ctx,
		`INSERT INTO library_files (id, library_id, path, size_bytes, media_id, last_scanned, created_at)
		 VALUES (?, ?, ?, ?, NULL, ?, ?)`,
		"lf-2", "lib-2", path, 9999, now, now,
	)
	if err != nil {
		t.Fatalf("insert library file: %v", err)
	}
	_, err = raw.ExecContext(ctx,
		`INSERT INTO movie_files (id, movie_id, file_path, size, quality, format, media_info, date_added, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"mf-2", "movie-2", path, 9999, "WEBDL-1080p", "x264", `{}`, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert movie file: %v", err)
	}

	if err := repo.UpdateMovieFile(ctx, &MovieFile{
		ID:        "mf-2",
		MovieID:   "movie-2",
		FilePath:  path,
		Size:      11111,
		Quality:   "Bluray-1080p",
		Format:    "x265",
		DateAdded: now,
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("update movie file: %v", err)
	}

	var mediaID sql.NullString
	if err := raw.QueryRowContext(ctx, `SELECT media_id FROM library_files WHERE path = ?`, path).Scan(&mediaID); err != nil {
		t.Fatalf("query library mapping: %v", err)
	}
	if !mediaID.Valid || mediaID.String != "movie-2" {
		t.Fatalf("library_files.media_id = %q (valid=%v), want movie-2", mediaID.String, mediaID.Valid)
	}

	after, err := store.UnmappedCount(ctx, "lib-2")
	if err != nil {
		t.Fatalf("unmapped after update: %v", err)
	}
	if after != 0 {
		t.Fatalf("unmapped after update = %d, want 0", after)
	}
}
