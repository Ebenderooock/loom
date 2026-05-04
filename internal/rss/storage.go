package rss

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Storage handles RSS item persistence and retrieval.
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new RSS storage layer.
func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

// StoreItems saves RSS items to the database, handling deduplication by GUID+SourceID.
// Returns count of newly stored items and count of duplicates skipped.
func (s *Storage) StoreItems(ctx context.Context, items []*Item) (stored, deduped int, err error) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	for _, item := range items {
		// Check if this GUID+SourceID combo already exists
		var existsID string
		checkErr := s.db.QueryRowContext(ctx,
			`SELECT id FROM rss_items WHERE guid = ? AND source_id = ? LIMIT 1`,
			item.GUID, item.SourceID,
		).Scan(&existsID)

		if checkErr == nil {
			// Item already exists, skip it
			deduped++
			continue
		} else if checkErr != sql.ErrNoRows {
			return stored, deduped, fmt.Errorf("check duplicate: %w", checkErr)
		}

		// New item, insert it
		if item.ID == "" {
			item.ID = uuid.New().String()
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now().UTC()
		}

		_, err := s.db.ExecContext(ctx,
			`INSERT INTO rss_items (id, title, link, published_at, source_id, guid, created_at, raw)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.Title, item.Link, item.PublishedAt,
			item.SourceID, item.GUID, item.CreatedAt, item.Raw,
		)
		if err != nil {
			return stored, deduped, fmt.Errorf("insert item: %w", err)
		}
		stored++
	}

	return stored, deduped, nil
}

// GetRecentItems retrieves the most recent RSS items, optionally filtered by source.
func (s *Storage) GetRecentItems(ctx context.Context, limit int, sourceID string) ([]*Item, error) {
	var rows *sql.Rows
	var err error

	if sourceID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, title, link, published_at, source_id, guid, created_at, raw
			 FROM rss_items WHERE source_id = ? ORDER BY created_at DESC LIMIT ?`,
			sourceID, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, title, link, published_at, source_id, guid, created_at, raw
			 FROM rss_items ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		item := &Item{}
		err := rows.Scan(
			&item.ID, &item.Title, &item.Link, &item.PublishedAt,
			&item.SourceID, &item.GUID, &item.CreatedAt, &item.Raw,
		)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return items, nil
}

// GetItemsByDateRange retrieves items published within a date range.
func (s *Storage) GetItemsByDateRange(ctx context.Context, from, to time.Time, limit int) ([]*Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, link, published_at, source_id, guid, created_at, raw
		 FROM rss_items
		 WHERE published_at BETWEEN ? AND ?
		 ORDER BY published_at DESC
		 LIMIT ?`,
		from, to, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query by date range: %w", err)
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		item := &Item{}
		err := rows.Scan(
			&item.ID, &item.Title, &item.Link, &item.PublishedAt,
			&item.SourceID, &item.GUID, &item.CreatedAt, &item.Raw,
		)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return items, nil
}

// DeleteOldItems removes items older than the specified duration (retention policy).
func (s *Storage) DeleteOldItems(ctx context.Context, olderThan time.Duration) (deleted int64, err error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM rss_items WHERE created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old items: %w", err)
	}
	return result.RowsAffected()
}

// ClearSource removes all items from a specific source (useful when re-enabling a source).
func (s *Storage) ClearSource(ctx context.Context, sourceID string) (deleted int64, err error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM rss_items WHERE source_id = ?`,
		sourceID,
	)
	if err != nil {
		return 0, fmt.Errorf("clear source: %w", err)
	}
	return result.RowsAffected()
}

// GetSourceStats returns statistics about items from a specific source.
func (s *Storage) GetSourceStats(ctx context.Context, sourceID string) (count int64, oldestItem time.Time, newestItem time.Time, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), MIN(published_at), MAX(published_at)
		 FROM rss_items WHERE source_id = ?`,
		sourceID,
	).Scan(&count, &oldestItem, &newestItem)
	if err != nil && err != sql.ErrNoRows {
		return 0, oldestItem, newestItem, fmt.Errorf("get stats: %w", err)
	}
	return count, oldestItem, newestItem, nil
}
