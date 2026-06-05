package bots

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store errors.
var (
	// ErrInvalidCode indicates a link code does not exist.
	ErrInvalidCode = errors.New("bots: invalid link code")
	// ErrCodeExpired indicates a link code has passed its expiry.
	ErrCodeExpired = errors.New("bots: link code expired")
	// ErrLinkedToOther indicates the chat identity is already bound to a
	// different Loom account; it must be unlinked before re-binding.
	ErrLinkedToOther = errors.New("bots: chat account is already linked to a different user")
	// ErrLinkNotFound indicates an account link id does not exist.
	ErrLinkNotFound = errors.New("bots: account link not found")
)

const tsLayout = time.RFC3339

func parseTS(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// codeAlphabet is Crockford base32 (no I, L, O, U) for unambiguous human entry.
const codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// codeLength yields ~60 bits of entropy (12 * 5 bits).
const codeLength = 12

// generateCode returns a cryptographically random Crockford-base32 code.
func generateCode() (string, error) {
	buf := make([]byte, codeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = codeAlphabet[int(buf[i])%len(codeAlphabet)]
	}
	return string(buf), nil
}

// normalizeCode canonicalizes user-entered codes: uppercase, strip spaces and
// dashes, and fold the Crockford-ambiguous letters (I/L→1, O→0).
func normalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch r {
		case ' ', '-', '\t':
			continue
		case 'I', 'L':
			b.WriteRune('1')
		case 'O':
			b.WriteRune('0')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Store persists bot configuration, account links, and link codes.
type Store struct {
	db *sql.DB
}

// NewStore creates a bots store backed by the given database.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// GetConfig returns the singleton bot configuration, creating defaults if the
// row is somehow absent.
func (s *Store) GetConfig(ctx context.Context) (Config, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT telegram_enabled, telegram_bot_token, discord_enabled, discord_bot_token,
		       default_movie_quality_profile_id, default_movie_library_id,
		       default_series_quality_profile_id, default_series_library_id, updated_at
		FROM bot_config WHERE id = 1`)
	var (
		c       Config
		updated string
	)
	err := row.Scan(&c.TelegramEnabled, &c.TelegramBotToken, &c.DiscordEnabled, &c.DiscordBotToken,
		&c.DefaultMovieQualityProfileID, &c.DefaultMovieLibraryID,
		&c.DefaultSeriesQualityProfileID, &c.DefaultSeriesLibraryID, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("bots: get config: %w", err)
	}
	c.UpdatedAt = parseTS(updated)
	return c, nil
}

// SetConfig persists the singleton bot configuration.
func (s *Store) SetConfig(ctx context.Context, c Config) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE bot_config SET
			telegram_enabled = ?, telegram_bot_token = ?,
			discord_enabled = ?, discord_bot_token = ?,
			default_movie_quality_profile_id = ?, default_movie_library_id = ?,
			default_series_quality_profile_id = ?, default_series_library_id = ?,
			updated_at = ?
		WHERE id = 1`,
		c.TelegramEnabled, c.TelegramBotToken, c.DiscordEnabled, c.DiscordBotToken,
		c.DefaultMovieQualityProfileID, c.DefaultMovieLibraryID,
		c.DefaultSeriesQualityProfileID, c.DefaultSeriesLibraryID,
		time.Now().UTC().Format(tsLayout))
	if err != nil {
		return fmt.Errorf("bots: set config: %w", err)
	}
	return nil
}

// CreateLinkCode generates and stores a fresh single-use code for the given chat
// identity, invalidating any prior outstanding codes for that identity.
func (s *Store) CreateLinkCode(ctx context.Context, platform Platform, externalID, externalUsername string) (LinkCode, error) {
	code, err := generateCode()
	if err != nil {
		return LinkCode{}, fmt.Errorf("bots: generate code: %w", err)
	}
	now := time.Now().UTC()
	lc := LinkCode{
		Code:             code,
		Platform:         platform,
		ExternalID:       externalID,
		ExternalUsername: externalUsername,
		ExpiresAt:        now.Add(LinkCodeTTL),
		CreatedAt:        now,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LinkCode{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bot_link_codes WHERE platform = ? AND external_id = ?`,
		platform, externalID); err != nil {
		return LinkCode{}, fmt.Errorf("bots: clear prior codes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bot_link_codes (code, platform, external_id, external_username, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		lc.Code, platform, externalID, externalUsername,
		lc.ExpiresAt.Format(tsLayout), lc.CreatedAt.Format(tsLayout)); err != nil {
		return LinkCode{}, fmt.Errorf("bots: insert code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LinkCode{}, err
	}
	return lc, nil
}

// PreviewLinkCode returns the chat identity behind a code without consuming it,
// so the web UI can confirm who is being linked before binding. Expired codes
// return ErrCodeExpired.
func (s *Store) PreviewLinkCode(ctx context.Context, code string) (LinkCode, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT code, platform, external_id, external_username, expires_at, created_at
		FROM bot_link_codes WHERE code = ?`, code)
	lc, err := scanLinkCode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LinkCode{}, ErrInvalidCode
	}
	if err != nil {
		return LinkCode{}, err
	}
	if time.Now().UTC().After(lc.ExpiresAt) {
		return LinkCode{}, ErrCodeExpired
	}
	return lc, nil
}

// RedeemLinkCode binds the chat identity behind code to userID and consumes the
// code. Re-redeeming an identity already linked to the same user is idempotent;
// an identity linked to a different user returns ErrLinkedToOther.
func (s *Store) RedeemLinkCode(ctx context.Context, code string, userID int64) (AccountLink, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountLink{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT code, platform, external_id, external_username, expires_at, created_at
		FROM bot_link_codes WHERE code = ?`, code)
	lc, err := scanLinkCode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountLink{}, ErrInvalidCode
	}
	if err != nil {
		return AccountLink{}, err
	}
	if time.Now().UTC().After(lc.ExpiresAt) {
		_, _ = tx.ExecContext(ctx, `DELETE FROM bot_link_codes WHERE code = ?`, code)
		_ = tx.Commit()
		return AccountLink{}, ErrCodeExpired
	}

	// Guard against silently reassigning a chat identity already bound elsewhere.
	var existingUser int64
	var existingID string
	err = tx.QueryRowContext(ctx,
		`SELECT id, user_id FROM bot_account_links WHERE platform = ? AND external_id = ?`,
		lc.Platform, lc.ExternalID).Scan(&existingID, &existingUser)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// no existing link — create below
	case err != nil:
		return AccountLink{}, err
	case existingUser != userID:
		return AccountLink{}, ErrLinkedToOther
	default:
		// already linked to this same user: refresh handle, consume code.
		if _, err := tx.ExecContext(ctx,
			`UPDATE bot_account_links SET external_username = ? WHERE id = ?`,
			lc.ExternalUsername, existingID); err != nil {
			return AccountLink{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM bot_link_codes WHERE code = ?`, code); err != nil {
			return AccountLink{}, err
		}
		if err := tx.Commit(); err != nil {
			return AccountLink{}, err
		}
		return AccountLink{ID: existingID, Platform: lc.Platform, ExternalID: lc.ExternalID,
			ExternalUsername: lc.ExternalUsername, UserID: userID}, nil
	}

	link := AccountLink{
		ID:               uuid.New().String(),
		Platform:         lc.Platform,
		ExternalID:       lc.ExternalID,
		ExternalUsername: lc.ExternalUsername,
		UserID:           userID,
		CreatedAt:        time.Now().UTC(),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bot_account_links (id, platform, external_id, external_username, user_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		link.ID, link.Platform, link.ExternalID, link.ExternalUsername, link.UserID,
		link.CreatedAt.Format(tsLayout)); err != nil {
		return AccountLink{}, fmt.Errorf("bots: insert link: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bot_link_codes WHERE code = ?`, code); err != nil {
		return AccountLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return AccountLink{}, err
	}
	return link, nil
}

// GetLink returns the account link for a chat identity, or nil if none exists.
func (s *Store) GetLink(ctx context.Context, platform Platform, externalID string) (*AccountLink, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, platform, external_id, external_username, user_id, created_at
		FROM bot_account_links WHERE platform = ? AND external_id = ?`, platform, externalID)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &link, nil
}

// ListLinks returns all account links, newest first.
func (s *Store) ListLinks(ctx context.Context) ([]AccountLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, platform, external_id, external_username, user_id, created_at
		FROM bot_account_links ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountLink
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// DeleteLink removes an account link by id.
func (s *Store) DeleteLink(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bot_account_links WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLinkNotFound
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanLink(row scanner) (AccountLink, error) {
	var (
		l       AccountLink
		created string
	)
	if err := row.Scan(&l.ID, &l.Platform, &l.ExternalID, &l.ExternalUsername, &l.UserID, &created); err != nil {
		return AccountLink{}, err
	}
	l.CreatedAt = parseTS(created)
	return l, nil
}

func scanLinkCode(row scanner) (LinkCode, error) {
	var (
		lc               lc2
		expires, created string
	)
	if err := row.Scan(&lc.Code, &lc.Platform, &lc.ExternalID, &lc.ExternalUsername, &expires, &created); err != nil {
		return LinkCode{}, err
	}
	return LinkCode{
		Code:             lc.Code,
		Platform:         lc.Platform,
		ExternalID:       lc.ExternalID,
		ExternalUsername: lc.ExternalUsername,
		ExpiresAt:        parseTS(expires),
		CreatedAt:        parseTS(created),
	}, nil
}

// lc2 is a scan helper holding the string-typed columns of a link code.
type lc2 struct {
	Code             string
	Platform         Platform
	ExternalID       string
	ExternalUsername string
}
