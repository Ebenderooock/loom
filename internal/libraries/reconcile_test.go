package libraries

import (
	"context"
	"testing"
	"time"
)

// seedMovieFile inserts a minimal movie + movie_file row into the DB.
func seedMovieFile(t *testing.T, store *Store, movieID, filePath string) {
	t.Helper()
	rawDB := store.db
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := rawDB.ExecContext(ctx,
		`INSERT OR IGNORE INTO movies
		 (id, title, year, status, monitoring_status, library_id, quality_profile_id, created_at, updated_at)
		 VALUES (?, ?, 2024, 'available', 'monitored', 'lib1', 'default', ?, ?)`,
		movieID, movieID, now, now)
	if err != nil {
		t.Fatalf("seed movie: %v", err)
	}

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO movie_files
		 (id, movie_id, file_path, size, quality, format, date_added, created_at, updated_at)
		 VALUES (?, ?, ?, 1000000, 'HD-1080p', 'BluRay', ?, ?, ?)`,
		movieID+"-file", movieID, filePath, now, now, now)
	if err != nil {
		t.Fatalf("seed movie_file: %v", err)
	}
}

// seedEpisodeFile inserts a series + season + episode + episode_file row.
func seedEpisodeFile(t *testing.T, store *Store, seriesID, episodeID, filePath string) {
	t.Helper()
	rawDB := store.db
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := rawDB.ExecContext(ctx,
		`INSERT OR IGNORE INTO series
		 (id, title, year, status, monitoring_status, library_id, quality_profile_id, created_at, updated_at)
		 VALUES (?, ?, 2024, 'continuing', 'monitored', 'lib1', 'default', ?, ?)`,
		seriesID, seriesID, now, now)
	if err != nil {
		t.Fatalf("seed series: %v", err)
	}

	seasonID := seriesID + "-s1"
	_, err = rawDB.ExecContext(ctx,
		`INSERT OR IGNORE INTO seasons
		 (id, series_id, season_number, monitored, created_at, updated_at)
		 VALUES (?, ?, 1, 1, ?, ?)`,
		seasonID, seriesID, now, now)
	if err != nil {
		t.Fatalf("seed season: %v", err)
	}

	_, err = rawDB.ExecContext(ctx,
		`INSERT OR IGNORE INTO episodes
		 (id, series_id, season_id, episode_number, title, has_file, monitored, created_at, updated_at)
		 VALUES (?, ?, ?, 1, 'Episode 1', true, 1, ?, ?)`,
		episodeID, seriesID, seasonID, now, now)
	if err != nil {
		t.Fatalf("seed episode: %v", err)
	}

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO episode_files
		 (id, episode_id, series_id, file_path, file_size, quality, source, resolution, codec, media_info, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 500000, 'HD-1080p', 'BluRay', '1080p', 'H264', '{}', ?, ?)`,
		episodeID+"-file", episodeID, seriesID, filePath, now, now)
	if err != nil {
		t.Fatalf("seed episode_file: %v", err)
	}
}

func TestReconcileRemovedPaths_MovieSetsMissing(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	ctx := context.Background()

	seedMovieFile(t, store, "movie-1", "/lib/movie-1/movie.mkv")

	// Verify initial state: status = 'available'
	var status string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM movies WHERE id = ?`, "movie-1").Scan(&status); err != nil {
		t.Fatalf("query initial status: %v", err)
	}
	if status != "available" {
		t.Fatalf("expected initial status 'available', got %q", status)
	}

	// Reconcile as if the file was removed.
	if err := store.ReconcileRemovedPaths(ctx, []string{"/lib/movie-1/movie.mkv"}); err != nil {
		t.Fatalf("ReconcileRemovedPaths: %v", err)
	}

	// Movie file should be soft-deleted.
	var deletedAt *string
	if err := store.db.QueryRowContext(ctx,
		`SELECT deleted_at FROM movie_files WHERE movie_id = ?`, "movie-1").Scan(&deletedAt); err != nil {
		t.Fatalf("query movie_file: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("expected movie_file to be soft-deleted, but deleted_at is NULL")
	}

	// Movie status should now be 'missing'.
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM movies WHERE id = ?`, "movie-1").Scan(&status); err != nil {
		t.Fatalf("query status after reconcile: %v", err)
	}
	if status != "missing" {
		t.Fatalf("expected status 'missing', got %q", status)
	}
}

func TestReconcileRemovedPaths_MovieKeepsStatusWhenFileRemains(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	ctx := context.Background()

	seedMovieFile(t, store, "movie-2", "/lib/movie-2/movie.mkv")

	// Seed a second file for the same movie (multi-file scenario).
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := store.db.ExecContext(ctx,
		`INSERT INTO movie_files
		 (id, movie_id, file_path, size, quality, format, date_added, created_at, updated_at)
		 VALUES ('movie-2-file2', 'movie-2', '/lib/movie-2/extras.mkv', 200000, 'HD-1080p', 'BluRay', ?, ?, ?)`,
		now, now, now)
	if err != nil {
		t.Fatalf("seed second movie_file: %v", err)
	}

	// Remove only the first file.
	if err := store.ReconcileRemovedPaths(ctx, []string{"/lib/movie-2/movie.mkv"}); err != nil {
		t.Fatalf("ReconcileRemovedPaths: %v", err)
	}

	// Movie still has a valid file → status must stay 'available'.
	var status string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM movies WHERE id = ?`, "movie-2").Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "available" {
		t.Fatalf("expected status 'available' (file remains), got %q", status)
	}
}

func TestReconcileRemovedPaths_EpisodeSetsHasFileFalse(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	ctx := context.Background()

	seedEpisodeFile(t, store, "series-1", "ep-1", "/lib/series-1/s01e01.mkv")

	// Verify initial state.
	var hasFile bool
	if err := store.db.QueryRowContext(ctx, `SELECT has_file FROM episodes WHERE id = ?`, "ep-1").Scan(&hasFile); err != nil {
		t.Fatalf("query initial has_file: %v", err)
	}
	if !hasFile {
		t.Fatal("expected initial has_file=true")
	}

	// Reconcile the removed file.
	if err := store.ReconcileRemovedPaths(ctx, []string{"/lib/series-1/s01e01.mkv"}); err != nil {
		t.Fatalf("ReconcileRemovedPaths: %v", err)
	}

	// Episode file row should be gone.
	var count int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM episode_files WHERE episode_id = ?`, "ep-1").Scan(&count); err != nil {
		t.Fatalf("query episode_files count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected episode_file row deleted, got count=%d", count)
	}

	// Episode has_file should be false.
	if err := store.db.QueryRowContext(ctx, `SELECT has_file FROM episodes WHERE id = ?`, "ep-1").Scan(&hasFile); err != nil {
		t.Fatalf("query has_file after reconcile: %v", err)
	}
	if hasFile {
		t.Fatal("expected has_file=false after file removal")
	}
}

func TestReconcileRemovedPaths_EpisodeKeepsHasFileWhenFileRemains(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	ctx := context.Background()

	seedEpisodeFile(t, store, "series-2", "ep-2", "/lib/series-2/s01e01.mkv")

	// Seed a second file for the same episode.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := store.db.ExecContext(ctx,
		`INSERT INTO episode_files
		 (id, episode_id, series_id, file_path, file_size, quality, source, resolution, codec, media_info, created_at, updated_at)
		 VALUES ('ep-2-file2', 'ep-2', 'series-2', '/lib/series-2/s01e01-alt.mkv', 400000, 'HD-1080p', 'BluRay', '1080p', 'H264', '{}', ?, ?)`,
		now, now)
	if err != nil {
		t.Fatalf("seed second episode_file: %v", err)
	}

	// Remove only the first file.
	if err := store.ReconcileRemovedPaths(ctx, []string{"/lib/series-2/s01e01.mkv"}); err != nil {
		t.Fatalf("ReconcileRemovedPaths: %v", err)
	}

	// Episode still has another file → has_file must remain true.
	var hasFile bool
	if err := store.db.QueryRowContext(ctx, `SELECT has_file FROM episodes WHERE id = ?`, "ep-2").Scan(&hasFile); err != nil {
		t.Fatalf("query has_file: %v", err)
	}
	if !hasFile {
		t.Fatal("expected has_file=true (second file still present)")
	}
}

func TestReconcileRemovedPaths_EmptyPathsIsNoop(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)

	// Should not error on empty input.
	if err := store.ReconcileRemovedPaths(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error for nil paths: %v", err)
	}
	if err := store.ReconcileRemovedPaths(context.Background(), []string{}); err != nil {
		t.Fatalf("unexpected error for empty paths: %v", err)
	}
}

func TestReconcileRemovedPaths_Idempotent(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	ctx := context.Background()

	seedMovieFile(t, store, "movie-idem", "/lib/idem/movie.mkv")

	paths := []string{"/lib/idem/movie.mkv"}

	// First reconcile.
	if err := store.ReconcileRemovedPaths(ctx, paths); err != nil {
		t.Fatalf("first ReconcileRemovedPaths: %v", err)
	}

	// Second reconcile should not error.
	if err := store.ReconcileRemovedPaths(ctx, paths); err != nil {
		t.Fatalf("second ReconcileRemovedPaths (idempotent): %v", err)
	}

	var status string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM movies WHERE id = ?`, "movie-idem").Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "missing" {
		t.Fatalf("expected 'missing' after idempotent reconcile, got %q", status)
	}
}
