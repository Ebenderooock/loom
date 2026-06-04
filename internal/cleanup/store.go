package cleanup

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Store persists cleanup orphans and settings.
type Store struct {
	db *sql.DB
}

// NewStore creates a cleanup store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const tsLayout = time.RFC3339

// parseTS tolerates both RFC3339 (our writes) and SQLite's CURRENT_TIMESTAMP
// "2006-01-02 15:04:05" form (seeded defaults).
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Upsert records (or refreshes) an orphan discovered during a scan. A newly
// discovered path is inserted as pending; an existing row only has its
// last_seen_at, size, root and client refreshed — its status and first_seen_at
// are preserved so retention timing and ignore decisions survive rescans.
func (s *Store) Upsert(ctx context.Context, o Orphan) error {
	now := time.Now().UTC().Format(tsLayout)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cleanup_orphans
			(id, path, client_id, root, size_bytes, status, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			client_id = excluded.client_id,
			root = excluded.root,
			size_bytes = excluded.size_bytes,
			last_seen_at = excluded.last_seen_at`,
		uuid.NewString(), o.Path, o.ClientID, o.Root, o.SizeBytes, now, now,
	)
	return err
}

// scanOrphan reads one row.
func scanOrphan(rows interface {
	Scan(...any) error
}) (Orphan, error) {
	var (
		o           Orphan
		status      string
		first, last string
		deleted     sql.NullString
	)
	if err := rows.Scan(&o.ID, &o.Path, &o.ClientID, &o.Root, &o.SizeBytes,
		&status, &o.Error, &first, &last, &deleted); err != nil {
		return Orphan{}, err
	}
	o.Status = OrphanStatus(status)
	o.FirstSeenAt = parseTS(first)
	o.LastSeenAt = parseTS(last)
	if deleted.Valid && deleted.String != "" {
		t := parseTS(deleted.String)
		o.DeletedAt = &t
	}
	return o, nil
}

const selectCols = `id, path, client_id, root, size_bytes, status, error, first_seen_at, last_seen_at, deleted_at`

// List returns orphans filtered by status. An empty status returns all rows.
func (s *Store) List(ctx context.Context, status OrphanStatus) ([]Orphan, error) {
	q := `SELECT ` + selectCols + ` FROM cleanup_orphans`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, string(status))
	}
	q += ` ORDER BY first_seen_at ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Orphan
	for rows.Next() {
		o, err := scanOrphan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Get returns a single orphan by id.
func (s *Store) Get(ctx context.Context, id string) (Orphan, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectCols+` FROM cleanup_orphans WHERE id = ?`, id)
	return scanOrphan(row)
}

// MarkDeleted flags an orphan as recycled.
func (s *Store) MarkDeleted(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(tsLayout)
	_, err := s.db.ExecContext(ctx,
		`UPDATE cleanup_orphans SET status='deleted', error='', deleted_at=? WHERE id=?`,
		now, id)
	return err
}

// MarkFailed records a failed deletion attempt with the error message.
func (s *Store) MarkFailed(ctx context.Context, id, msg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE cleanup_orphans SET status='delete_failed', error=? WHERE id=?`, msg, id)
	return err
}

// MarkIgnored flags an orphan to be kept and never auto-deleted or re-flagged.
func (s *Store) MarkIgnored(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE cleanup_orphans SET status='ignored', error='' WHERE id=?`, id)
	return err
}

// Delete removes a row entirely (used when an orphan resolves itself, e.g. it
// vanished from disk or became tracked again).
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cleanup_orphans WHERE id=?`, id)
	return err
}

// ResolveStalePending deletes pending/delete_failed rows whose path was not
// seen in the latest scan (gone from disk or now tracked). Ignored and deleted
// rows are preserved. seenPaths is the set of paths observed this scan.
func (s *Store) ResolveStalePending(ctx context.Context, seenPaths map[string]bool) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, path FROM cleanup_orphans WHERE status IN ('pending','delete_failed')`)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			rows.Close()
			return err
		}
		if !seenPaths[path] {
			stale = append(stale, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range stale {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM cleanup_orphans WHERE id=?`, id); err != nil {
			return err
		}
	}
	return nil
}

// GetSettings returns the cleanup settings, falling back to defaults if the
// row is somehow absent.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT auto_delete_enabled, retention_days FROM cleanup_settings WHERE id=1`)
	var st Settings
	if err := row.Scan(&st.AutoDeleteEnabled, &st.RetentionDays); err != nil {
		if err == sql.ErrNoRows {
			return Settings{AutoDeleteEnabled: true, RetentionDays: 7}, nil
		}
		return Settings{}, err
	}
	if st.RetentionDays < 1 {
		st.RetentionDays = 1
	}
	return st, nil
}

// SaveSettings upserts the global cleanup settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	if st.RetentionDays < 1 {
		st.RetentionDays = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cleanup_settings (id, auto_delete_enabled, retention_days, updated_at)
		VALUES (1, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			auto_delete_enabled = excluded.auto_delete_enabled,
			retention_days = excluded.retention_days,
			updated_at = CURRENT_TIMESTAMP`,
		st.AutoDeleteEnabled, st.RetentionDays,
	)
	return err
}
