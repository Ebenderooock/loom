package downloads

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// HistoryEntry represents a single row in the download_history table.
type HistoryEntry struct {
	ID          string  `json:"id"`
	DownloadID  string  `json:"download_id"`
	ClientID    string  `json:"client_id"`
	Title       string  `json:"title"`
	Category    string  `json:"category"`
	Status      string  `json:"status"`
	GrabbedAt   *string `json:"grabbed_at,omitempty"`
	CompletedAt string  `json:"completed_at"`
}

// HistoryStore persists download completion/failure events to SQLite.
type HistoryStore struct {
	db *sql.DB
}

// NewHistoryStore creates a HistoryStore backed by the provided database.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// RecordCompletion inserts a new history row from a DownloadCompletedEvent.
func (h *HistoryStore) RecordCompletion(ctx context.Context, event *DownloadCompletedEvent) error {
	id := uuid.New().String()
	completedAt := event.CompletedAt.UTC().Format(time.RFC3339)

	_, err := h.db.ExecContext(ctx,
		`INSERT INTO download_history (id, download_id, client_id, title, category, status, completed_at)
		 VALUES (?, ?, ?, ?, ?, 'completed', ?)`,
		id, event.DownloadID, event.ClientID, event.Title, event.Category, completedAt,
	)
	return err
}

// WasCompleted checks whether a download was already emitted as completed.
// Used for idempotency across process restarts — prevents re-importing
// a download that was already handled in a previous session.
func (h *HistoryStore) WasCompleted(ctx context.Context, clientID, downloadID string) bool {
	var count int
	err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM download_history WHERE client_id = ? AND download_id = ? AND status = 'completed'`,
		clientID, downloadID,
	).Scan(&count)
	return err == nil && count > 0
}

// List returns history entries ordered by completed_at descending.
func (h *HistoryStore) List(ctx context.Context, limit, offset int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := h.db.QueryContext(ctx,
		`SELECT id, download_id, client_id, title, category, status, grabbed_at, completed_at
		 FROM download_history
		 ORDER BY completed_at DESC
		 LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.DownloadID, &e.ClientID, &e.Title, &e.Category, &e.Status, &e.GrabbedAt, &e.CompletedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
