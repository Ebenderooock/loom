package libraries

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store handles SQLite persistence for libraries and library files.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// List returns all libraries.
func (s *Store) List(ctx context.Context) ([]Library, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, path, media_type, monitor_on_add, quality_profile_id,
		       unmonitor_on_delete, auto_archive_watched, auto_archive_days_after_watch,
		       created_at, updated_at
		FROM libraries
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var libs []Library
	for rows.Next() {
		var l Library
		var mon, uod, aaw int
		if err := rows.Scan(&l.ID, &l.Name, &l.Path, &l.MediaType, &mon,
			&l.QualityProfileID, &uod, &aaw, &l.AutoArchiveDaysAfterWatch,
			&l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.MonitorOnAdd = mon != 0
		l.UnmonitorOnDelete = uod != 0
		l.AutoArchiveWatched = aaw != 0
		libs = append(libs, l)
	}
	return libs, rows.Err()
}

// Get returns a library by ID.
func (s *Store) Get(ctx context.Context, id string) (*Library, error) {
	var l Library
	var mon, uod, aaw int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, path, media_type, monitor_on_add, quality_profile_id,
		       unmonitor_on_delete, auto_archive_watched, auto_archive_days_after_watch,
		       created_at, updated_at
		FROM libraries WHERE id = ?`, id).Scan(
		&l.ID, &l.Name, &l.Path, &l.MediaType, &mon,
		&l.QualityProfileID, &uod, &aaw, &l.AutoArchiveDaysAfterWatch,
		&l.CreatedAt, &l.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("library %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	l.MonitorOnAdd = mon != 0
	l.UnmonitorOnDelete = uod != 0
	l.AutoArchiveWatched = aaw != 0
	return &l, nil
}

// Create inserts a new library, generating an ID if empty.
func (s *Store) Create(ctx context.Context, l *Library) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	l.CreatedAt = now
	l.UpdatedAt = now
	mon := 0
	if l.MonitorOnAdd {
		mon = 1
	}
	uod := 0
	if l.UnmonitorOnDelete {
		uod = 1
	}
	aaw := 0
	if l.AutoArchiveWatched {
		aaw = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO libraries (id, name, path, media_type, monitor_on_add, quality_profile_id,
		       unmonitor_on_delete, auto_archive_watched, auto_archive_days_after_watch,
		       created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.Name, l.Path, l.MediaType, mon, l.QualityProfileID,
		uod, aaw, l.AutoArchiveDaysAfterWatch, l.CreatedAt, l.UpdatedAt)
	return err
}

// Update modifies an existing library.
func (s *Store) Update(ctx context.Context, l *Library) error {
	l.UpdatedAt = time.Now().UTC()
	mon := 0
	if l.MonitorOnAdd {
		mon = 1
	}
	uod := 0
	if l.UnmonitorOnDelete {
		uod = 1
	}
	aaw := 0
	if l.AutoArchiveWatched {
		aaw = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE libraries SET name = ?, path = ?, media_type = ?, monitor_on_add = ?,
		       quality_profile_id = ?, unmonitor_on_delete = ?, auto_archive_watched = ?,
		       auto_archive_days_after_watch = ?, updated_at = ?
		WHERE id = ?`,
		l.Name, l.Path, l.MediaType, mon, l.QualityProfileID,
		uod, aaw, l.AutoArchiveDaysAfterWatch, l.UpdatedAt, l.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("library %q not found", l.ID)
	}
	return nil
}

// Delete removes a library (cascade deletes library_files via FK).
func (s *Store) Delete(ctx context.Context, id string) error {
	// Delete files first for SQLite FK enforcement.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM library_files WHERE library_id = ?`, id)
	res, err := s.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("library %q not found", id)
	}
	return nil
}

// FileCount returns the number of files in a library.
func (s *Store) FileCount(ctx context.Context, libraryID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_files WHERE library_id = ?`, libraryID).Scan(&count)
	return count, err
}

// ListFiles returns all files for a library.
func (s *Store) ListFiles(ctx context.Context, libraryID string) ([]LibraryFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, library_id, path, size_bytes, media_id, last_scanned, created_at
		FROM library_files
		WHERE library_id = ?
		ORDER BY path`, libraryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var files []LibraryFile
	for rows.Next() {
		var f LibraryFile
		if err := rows.Scan(&f.ID, &f.LibraryID, &f.Path, &f.SizeBytes,
			&f.MediaID, &f.LastScanned, &f.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// UpsertFile inserts or updates a library file by path.
func (s *Store) UpsertFile(ctx context.Context, f *LibraryFile) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	f.LastScanned = &now
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO library_files (id, library_id, path, size_bytes, media_id, last_scanned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			size_bytes = excluded.size_bytes,
			media_id = excluded.media_id,
			last_scanned = excluded.last_scanned`,
		f.ID, f.LibraryID, f.Path, f.SizeBytes, f.MediaID, f.LastScanned, f.CreatedAt)
	return err
}

// StaleFilePaths returns the paths of library files that were not touched since the given time.
// Call this before DeleteStaleFiles to capture paths for reconciliation.
func (s *Store) StaleFilePaths(ctx context.Context, libraryID string, since time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT path FROM library_files
		WHERE library_id = ? AND (last_scanned IS NULL OR last_scanned < ?)`,
		libraryID, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// DeleteStaleFiles removes files for a library that were not scanned since the given time.
func (s *Store) DeleteStaleFiles(ctx context.Context, libraryID string, since time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM library_files WHERE library_id = ? AND (last_scanned IS NULL OR last_scanned < ?)`,
		libraryID, since)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ReconcileRemovedPaths updates movie and episode state for paths that were
// removed from disk (i.e. no longer present in library_files).
//
// For movies: soft-deletes movie_files rows and sets the movie status to
// "missing" when no valid file remains.
// For episodes: deletes episode_files rows and sets has_file=false when no
// valid file remains for that episode.
//
// The operation is idempotent — repeated calls with the same paths are safe.
func (s *Store) ReconcileRemovedPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	// Build (?,?,…) placeholder for IN clauses.
	ph := strings.Repeat("?,", len(paths))
	ph = "(" + ph[:len(ph)-1] + ")"
	args := make([]any, len(paths))
	for i, p := range paths {
		args[i] = p
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reconcile tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()

	// 1. Soft-delete movie_files for the removed paths.
	if _, err := tx.ExecContext(ctx,
		`UPDATE movie_files SET deleted_at = ?, updated_at = ? WHERE file_path IN `+ph+` AND deleted_at IS NULL`,
		append([]any{now, now}, args...)...); err != nil {
		return fmt.Errorf("soft-delete movie_files: %w", err)
	}

	// 2. Set movies to "missing" where no valid file remains after the deletion
	//    above, but only for movies that were associated with one of the removed paths.
	if _, err := tx.ExecContext(ctx,
		`UPDATE movies SET status = 'missing', updated_at = ?
		 WHERE deleted_at IS NULL
		   AND id IN (SELECT movie_id FROM movie_files WHERE file_path IN `+ph+`)
		   AND NOT EXISTS (
		       SELECT 1 FROM movie_files mf
		       WHERE mf.movie_id = movies.id AND mf.deleted_at IS NULL
		   )`,
		append([]any{now}, args...)...); err != nil {
		return fmt.Errorf("update movie status: %w", err)
	}

	// 3. Capture affected episode IDs before deleting episode_files, so we
	//    can reset has_file on episodes that now have zero remaining files.
	epRows, err := tx.QueryContext(ctx,
		`SELECT DISTINCT episode_id FROM episode_files WHERE file_path IN `+ph,
		args...)
	if err != nil {
		return fmt.Errorf("query affected episodes: %w", err)
	}
	var affectedEpisodeIDs []string
	for epRows.Next() {
		var id string
		if scanErr := epRows.Scan(&id); scanErr != nil {
			_ = epRows.Close()
			return fmt.Errorf("scan episode id: %w", scanErr)
		}
		affectedEpisodeIDs = append(affectedEpisodeIDs, id)
	}
	_ = epRows.Close()
	if err := epRows.Err(); err != nil {
		return fmt.Errorf("iterate affected episodes: %w", err)
	}

	// 4. Delete episode_files for the removed paths.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM episode_files WHERE file_path IN `+ph,
		args...); err != nil {
		return fmt.Errorf("delete episode_files: %w", err)
	}

	// 5. Reset has_file=false for episodes with no remaining files.
	if len(affectedEpisodeIDs) > 0 {
		epPH := strings.Repeat("?,", len(affectedEpisodeIDs))
		epPH = "(" + epPH[:len(epPH)-1] + ")"
		epArgs := make([]any, len(affectedEpisodeIDs))
		for i, id := range affectedEpisodeIDs {
			epArgs[i] = id
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE episodes SET has_file = false, updated_at = ?
			 WHERE id IN `+epPH+`
			   AND NOT EXISTS (
			       SELECT 1 FROM episode_files ef WHERE ef.episode_id = episodes.id
			   )`,
			append([]any{now.Format(time.RFC3339)}, epArgs...)...); err != nil {
			return fmt.Errorf("reset episode has_file: %w", err)
		}
	}

	return tx.Commit()
}

// ShouldUnmonitorOnDelete returns true if the library has unmonitor-on-delete enabled.
func (s *Store) ShouldUnmonitorOnDelete(ctx context.Context, libraryID string) bool {
	var val int
	err := s.db.QueryRowContext(ctx,
		`SELECT unmonitor_on_delete FROM libraries WHERE id = ?`, libraryID).Scan(&val)
	return err == nil && val == 1
}
