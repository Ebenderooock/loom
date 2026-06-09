package requests

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a request id does not exist.
var ErrNotFound = errors.New("requests: not found")

// ErrDuplicate is returned when an open request already exists for the media.
var ErrDuplicate = errors.New("requests: an open request already exists for this title")

// ErrConflict is returned when a state transition no longer applies because the
// request's status changed concurrently (e.g. approved while being rejected).
var ErrConflict = errors.New("requests: request state changed concurrently")

// ErrQuotaExceeded is returned when a user has reached their request limit for a
// media type within the configured rolling window.
var ErrQuotaExceeded = errors.New("requests: request quota exceeded")

// Store persists media requests.
type Store struct {
	db *sql.DB
}

// NewStore creates a request store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const tsLayout = time.RFC3339

// parseTS tolerates both RFC3339 (our writes) and SQLite's CURRENT_TIMESTAMP form.
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

const selectCols = `id, user_id, username, media_type, tmdb_id, title, year, poster_path,
	overview, status, reason, media_id, decided_by, decided_at, created_at, updated_at`

func scanRequest(row interface{ Scan(...any) error }) (Request, error) {
	var (
		r                 Request
		mediaType, status string
		created, updated  string
		decidedNull       sql.NullString
	)
	if err := row.Scan(&r.ID, &r.UserID, &r.Username, &mediaType, &r.TMDBID, &r.Title,
		&r.Year, &r.PosterPath, &r.Overview, &status, &r.Reason, &r.MediaID,
		&r.DecidedBy, &decidedNull, &created, &updated); err != nil {
		return Request{}, err
	}
	r.MediaType = MediaType(mediaType)
	r.Status = Status(status)
	r.CreatedAt = parseTS(created)
	r.UpdatedAt = parseTS(updated)
	if decidedNull.Valid && decidedNull.String != "" {
		t := parseTS(decidedNull.String)
		r.DecidedAt = &t
	}
	return r, nil
}

// isUniqueViolation reports whether err is a SQLite/Postgres unique-constraint
// violation (used to map the open-request partial index to ErrDuplicate).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "constraint failed")
}

// Create inserts a new pending request. The unique partial index on open
// requests enforces dedupe at the DB level; a conflict maps to ErrDuplicate.
func (s *Store) Create(ctx context.Context, r Request) (Request, error) {
	r.ID = uuid.NewString()
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	r.Status = StatusPending
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO media_requests
			(id, user_id, username, media_type, tmdb_id, title, year, poster_path,
			 overview, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		r.ID, r.UserID, r.Username, string(r.MediaType), r.TMDBID, r.Title, r.Year,
		r.PosterPath, r.Overview, now.Format(tsLayout), now.Format(tsLayout),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Request{}, ErrDuplicate
		}
		return Request{}, err
	}
	return r, nil
}

// List returns requests filtered by status (empty = all), newest first.
func (s *Store) List(ctx context.Context, status Status) ([]Request, error) {
	q := `SELECT ` + selectCols + ` FROM media_requests`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, string(status))
	}
	q += ` ORDER BY created_at DESC`
	return s.query(ctx, q, args...)
}

// ListByUser returns a single user's requests, newest first.
func (s *Store) ListByUser(ctx context.Context, userID string) ([]Request, error) {
	return s.query(ctx,
		`SELECT `+selectCols+` FROM media_requests WHERE user_id = ? ORDER BY created_at DESC`,
		userID)
}

func (s *Store) query(ctx context.Context, q string, args ...any) ([]Request, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns a single request by id.
func (s *Store) Get(ctx context.Context, id string) (Request, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+selectCols+` FROM media_requests WHERE id = ?`, id)
	r, err := scanRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	return r, err
}

// ClaimForApproval atomically transitions a pending request to approving so it
// can only be fulfilled once. It returns true if this caller won the claim.
func (s *Store) ClaimForApproval(ctx context.Context, id, decidedBy string) (bool, error) {
	now := time.Now().UTC().Format(tsLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE media_requests
		SET status='approving', decided_by=?, decided_at=?, updated_at=?
		WHERE id=? AND status='pending'`,
		decidedBy, now, now, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// MarkApproved finalizes a claimed (approving) request as approved with its
// created media id. It is a no-op (ErrConflict) if the request is no longer
// being approved.
func (s *Store) MarkApproved(ctx context.Context, id, mediaID string) error {
	now := time.Now().UTC().Format(tsLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE media_requests SET status='approved', media_id=?, reason='', updated_at=?
		WHERE id=? AND status='approving'`, mediaID, now, id)
	return affectedOne(res, err)
}

// MarkAvailable marks a pending request as available, recording its media id
// (used when the requested media already exists in the library). It is a no-op
// (ErrConflict) if the request is no longer pending.
func (s *Store) MarkAvailable(ctx context.Context, id, mediaID string) error {
	now := time.Now().UTC().Format(tsLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE media_requests SET status='available', media_id=?, reason='', updated_at=?
		WHERE id=? AND status='pending'`, mediaID, now, id)
	return affectedOne(res, err)
}

// MarkFailed returns a claimed (approving) request to a re-requestable failed
// state with a reason. decided_at/by are cleared so a fresh attempt is
// unambiguous.
func (s *Store) MarkFailed(ctx context.Context, id, reason string) error {
	now := time.Now().UTC().Format(tsLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE media_requests SET status='failed', reason=?, decided_by='', decided_at=NULL, updated_at=?
		WHERE id=? AND status='approving'`, reason, now, id)
	return affectedOne(res, err)
}

// MarkRejected records an admin rejection with a reason. Only pending or failed
// requests may be rejected; otherwise it is a no-op (ErrConflict).
func (s *Store) MarkRejected(ctx context.Context, id, reason, decidedBy string) error {
	now := time.Now().UTC().Format(tsLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE media_requests
		SET status='rejected', reason=?, decided_by=?, decided_at=?, updated_at=?
		WHERE id=? AND status IN ('pending','failed')`,
		reason, decidedBy, now, now, id)
	return affectedOne(res, err)
}

// affectedOne maps a conditional UPDATE that changed no rows to ErrConflict.
func affectedOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// statusPlaceholders renders the quota-counted statuses as a SQL list and args.
func statusPlaceholders() (string, []any) {
	parts := make([]string, len(quotaCountedStatuses))
	args := make([]any, len(quotaCountedStatuses))
	for i, st := range quotaCountedStatuses {
		parts[i] = "?"
		args[i] = string(st)
	}
	return strings.Join(parts, ", "), args
}

// GetQuotaConfig returns the global per-user request quota.
func (s *Store) GetQuotaConfig(ctx context.Context) (QuotaConfig, error) {
	var c QuotaConfig
	err := s.db.QueryRowContext(ctx,
		`SELECT movie_limit, series_limit, music_limit, window_days FROM request_quota_config WHERE id = 1`,
	).Scan(&c.MovieLimit, &c.SeriesLimit, &c.MusicLimit, &c.WindowDays)
	if errors.Is(err, sql.ErrNoRows) {
		// Defensive: row should exist from migration; treat as unlimited.
		return QuotaConfig{WindowDays: DefaultWindowDays}, nil
	}
	return c, err
}

// SetQuotaConfig persists the global per-user request quota.
func (s *Store) SetQuotaConfig(ctx context.Context, c QuotaConfig) error {
	now := time.Now().UTC().Format(tsLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE request_quota_config
		SET movie_limit = ?, series_limit = ?, music_limit = ?, window_days = ?, updated_at = ?
		WHERE id = 1`,
		c.MovieLimit, c.SeriesLimit, c.MusicLimit, c.WindowDays, now)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO request_quota_config (id, movie_limit, series_limit, music_limit, window_days, updated_at)
			VALUES (1, ?, ?, ?, ?, ?)`,
			c.MovieLimit, c.SeriesLimit, c.MusicLimit, c.WindowDays, now)
	}
	return err
}

// CountUserRequests returns how many of a user's requests for the given media
// type currently consume a quota slot within the rolling window. A zero `since`
// counts across all time.
func (s *Store) CountUserRequests(ctx context.Context, userID string, mt MediaType, since time.Time) (int, error) {
	statusList, statusArgs := statusPlaceholders()
	q := `SELECT COUNT(*) FROM media_requests
		WHERE user_id = ? AND media_type = ? AND status IN (` + statusList + `)`
	args := append([]any{userID, string(mt)}, statusArgs...)
	if !since.IsZero() {
		q += ` AND created_at >= ?`
		args = append(args, since.UTC().Format(tsLayout))
	}
	var n int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// CreateWithinQuota atomically enforces a per-user quota and inserts a new
// pending request. It serializes against concurrent inserts (BEGIN IMMEDIATE)
// so a user cannot exceed `limit` by racing. It returns ErrQuotaExceeded when
// the user already has `limit` counted requests of r.MediaType in the window.
func (s *Store) CreateWithinQuota(ctx context.Context, r Request, limit int, since time.Time) (Request, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Request{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Acquire a write lock up front so the count below cannot be invalidated by
	// a concurrent insert before we commit.
	if _, err := tx.ExecContext(ctx, `UPDATE request_quota_config SET id = id WHERE id = 1`); err != nil {
		return Request{}, err
	}

	statusList, statusArgs := statusPlaceholders()
	countQ := `SELECT COUNT(*) FROM media_requests
		WHERE user_id = ? AND media_type = ? AND status IN (` + statusList + `)`
	countArgs := append([]any{r.UserID, string(r.MediaType)}, statusArgs...)
	if !since.IsZero() {
		countQ += ` AND created_at >= ?`
		countArgs = append(countArgs, since.UTC().Format(tsLayout))
	}
	var used int
	if err := tx.QueryRowContext(ctx, countQ, countArgs...).Scan(&used); err != nil {
		return Request{}, err
	}
	if used >= limit {
		return Request{}, ErrQuotaExceeded
	}

	r.ID = uuid.NewString()
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	r.Status = StatusPending
	_, err = tx.ExecContext(ctx, `
		INSERT INTO media_requests
			(id, user_id, username, media_type, tmdb_id, title, year, poster_path,
			 overview, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		r.ID, r.UserID, r.Username, string(r.MediaType), r.TMDBID, r.Title, r.Year,
		r.PosterPath, r.Overview, now.Format(tsLayout), now.Format(tsLayout),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Request{}, ErrDuplicate
		}
		return Request{}, err
	}
	if err := tx.Commit(); err != nil {
		return Request{}, err
	}
	return r, nil
}
