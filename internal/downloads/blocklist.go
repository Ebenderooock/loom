package downloads

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// BlocklistEntry is a single blocklisted release.
type BlocklistEntry struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	IndexerID   string    `json:"indexer_id"`
	ReleaseHash string    `json:"release_hash"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}

// BlocklistStore manages the blocklist table.
type BlocklistStore struct {
	db *sql.DB
}

// NewBlocklistStore returns a store backed by db.
func NewBlocklistStore(db *sql.DB) *BlocklistStore {
	return &BlocklistStore{db: db}
}

// Add inserts or replaces a blocklist entry.
func (s *BlocklistStore) Add(ctx context.Context, entry BlocklistEntry) error {
	if entry.ID == "" {
		return fmt.Errorf("blocklist: id required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO blocklist (id, title, indexer_id, release_hash, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Title, entry.IndexerID, entry.ReleaseHash, entry.Reason, entry.CreatedAt,
	)
	return err
}

// List returns all blocklist entries ordered by created_at desc.
func (s *BlocklistStore) List(ctx context.Context) ([]BlocklistEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, indexer_id, release_hash, reason, created_at
		 FROM blocklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BlocklistEntry
	for rows.Next() {
		var e BlocklistEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.IndexerID, &e.ReleaseHash, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Remove deletes a single entry by ID.
func (s *BlocklistStore) Remove(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blocklist WHERE id = ?`, id)
	return err
}

// Clear deletes all blocklist entries.
func (s *BlocklistStore) Clear(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blocklist`)
	return err
}

// IsBlocked returns true if a release with the given hash is blocklisted.
func (s *BlocklistStore) IsBlocked(ctx context.Context, releaseHash string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blocklist WHERE release_hash = ?`, releaseHash).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

var infoHashRe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// NormalizeInfoHash returns the lowercase hex infohash if s is a v1 torrent
// infohash, or "" otherwise.
func NormalizeInfoHash(s string) string {
	s = strings.TrimSpace(s)
	if infoHashRe.MatchString(s) {
		return strings.ToLower(s)
	}
	return ""
}

// TitleHash returns the hex sha256 of a release title.
func TitleHash(title string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(title)))
}

// ReleaseHash returns the blocklist key for a release: its infohash when the
// download ID is one, otherwise a hash of the title.
func ReleaseHash(title, downloadID string) string {
	if h := NormalizeInfoHash(downloadID); h != "" {
		return h
	}
	return TitleHash(title)
}

// IsReleaseBlocked reports whether a candidate release matches a blocklist
// entry by infohash, by title hash, or by case-insensitive title.
func (s *BlocklistStore) IsReleaseBlocked(ctx context.Context, title, infoHash string) (bool, error) {
	title = strings.TrimSpace(title)
	hashes := []any{}
	if title != "" {
		hashes = append(hashes, TitleHash(title))
	}
	if h := NormalizeInfoHash(infoHash); h != "" {
		hashes = append(hashes, h)
	}
	if len(hashes) == 0 {
		return false, nil
	}
	query := `SELECT COUNT(*) FROM blocklist WHERE release_hash IN (?` + strings.Repeat(", ?", len(hashes)-1) + `)`
	args := hashes
	if title != "" {
		query += ` OR LOWER(title) = LOWER(?)`
		args = append(args, title)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
