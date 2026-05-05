package downloads

import (
	"context"
	"database/sql"
	"fmt"
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
