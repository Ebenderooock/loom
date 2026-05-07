package libraries

import (
	"context"
	"database/sql"
	"fmt"
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
	defer rows.Close()

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

// UnmappedCount returns the number of files without a media_id.
func (s *Store) UnmappedCount(ctx context.Context, libraryID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_files WHERE library_id = ? AND media_id IS NULL`, libraryID).Scan(&count)
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
	defer rows.Close()

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

// ShouldUnmonitorOnDelete returns true if the library has unmonitor-on-delete enabled.
func (s *Store) ShouldUnmonitorOnDelete(ctx context.Context, libraryID string) bool {
	var val int
	err := s.db.QueryRowContext(ctx,
		`SELECT unmonitor_on_delete FROM libraries WHERE id = ?`, libraryID).Scan(&val)
	return err == nil && val == 1
}
