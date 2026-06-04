package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrInviteNotFound is returned when an invite token does not exist.
var ErrInviteNotFound = errors.New("auth: invite not found")

// inviteTSLayout is the canonical timestamp format written by the invite store.
const inviteTSLayout = time.RFC3339

// parseInviteTS tolerates both RFC3339 (our writes) and SQLite's
// CURRENT_TIMESTAMP form, mirroring the requests store.
func parseInviteTS(s string) time.Time {
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

// Invite is a persisted, single-use account invitation.
type Invite struct {
	ID        string
	Token     string
	Email     string
	Role      string
	CreatedBy int64
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    time.Time
	UsedBy    int64
	// UsedByName is populated by list queries when the redeeming user still
	// exists; it is empty for pending invites.
	UsedByName string
}

// InviteStore persists account invites using a raw *sql.DB, mirroring the
// lighter store pattern used by the requests package.
type InviteStore struct {
	db *sql.DB
}

// NewInviteStore creates an invite store backed by the given database.
func NewInviteStore(db *sql.DB) *InviteStore {
	return &InviteStore{db: db}
}

// Create persists a new invite.
func (s *InviteStore) Create(ctx context.Context, inv Invite) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_invites (id, token, email, role, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.Token, inv.Email, inv.Role, inv.CreatedBy,
		inv.CreatedAt.UTC().Format(inviteTSLayout),
		inv.ExpiresAt.UTC().Format(inviteTSLayout),
	)
	return err
}

const inviteSelectCols = `i.id, i.token, i.email, i.role, i.created_by,
	i.created_at, i.expires_at, i.used_at, i.used_by,
	COALESCE(u.username, '')`

func scanInvite(row interface{ Scan(...any) error }) (Invite, error) {
	var (
		inv               Invite
		created, expires  string
		usedAt            sql.NullString
		usedBy            sql.NullInt64
		usedByName        string
	)
	if err := row.Scan(&inv.ID, &inv.Token, &inv.Email, &inv.Role, &inv.CreatedBy,
		&created, &expires, &usedAt, &usedBy, &usedByName); err != nil {
		return Invite{}, err
	}
	inv.CreatedAt = parseInviteTS(created)
	inv.ExpiresAt = parseInviteTS(expires)
	if usedAt.Valid {
		inv.UsedAt = parseInviteTS(usedAt.String)
	}
	if usedBy.Valid {
		inv.UsedBy = usedBy.Int64
	}
	inv.UsedByName = usedByName
	return inv, nil
}

// GetByToken returns the invite for the given token, or ErrInviteNotFound.
func (s *InviteStore) GetByToken(ctx context.Context, token string) (Invite, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+inviteSelectCols+`
		FROM user_invites i LEFT JOIN users u ON u.id = i.used_by
		WHERE i.token = ?`, token)
	inv, err := scanInvite(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrInviteNotFound
	}
	return inv, err
}

// List returns all invites, newest first.
func (s *InviteStore) List(ctx context.Context) ([]Invite, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+inviteSelectCols+`
		FROM user_invites i LEFT JOIN users u ON u.id = i.used_by
		ORDER BY i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Delete removes an invite by id. It reports whether a row was removed.
func (s *InviteStore) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_invites WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Claim atomically marks an unused, unexpired invite as consumed. It returns
// true only if the invite was claimable at the instant of the update,
// preventing a concurrent or expired redemption from succeeding. used_by is set
// later by Finalize once the account it provisions has an id.
func (s *InviteStore) Claim(ctx context.Context, token string, at time.Time) (bool, error) {
	ts := at.UTC().Format(inviteTSLayout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE user_invites SET used_at = ?
		WHERE token = ? AND used_at IS NULL AND expires_at > ?`,
		ts, token, ts,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Finalize records which user a claimed invite provisioned.
func (s *InviteStore) Finalize(ctx context.Context, token string, usedBy int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_invites SET used_by = ? WHERE token = ?`, usedBy, token)
	return err
}

// Release reverts a claim, making the invite redeemable again. Used to roll
// back when account creation fails after a successful claim.
func (s *InviteStore) Release(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_invites SET used_at = NULL, used_by = NULL WHERE token = ?`, token)
	return err
}
