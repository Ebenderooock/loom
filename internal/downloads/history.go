package downloads

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// HistoryEntry represents a single row in the download_history table.
type HistoryEntry struct {
	ID             string  `json:"id"`
	DownloadID     string  `json:"download_id"`
	ClientID       string  `json:"client_id"`
	Title          string  `json:"title"`
	Category       string  `json:"category"`
	Status         string  `json:"status"`
	GrabbedAt      *string `json:"grabbed_at,omitempty"`
	CompletedAt    string  `json:"completed_at"`
	AcquisitionKey string  `json:"acquisition_key,omitempty"`
	MediaType      string  `json:"media_type,omitempty"`
	MediaID        string  `json:"media_id,omitempty"`
	IndexerID      string  `json:"indexer_id,omitempty"`
	ReleaseHash    string  `json:"release_hash,omitempty"`
	Stage          string  `json:"stage,omitempty"`
	Reason         string  `json:"reason,omitempty"`
	Attempt        int     `json:"attempt,omitempty"`
	RetryCount     int     `json:"retry_count,omitempty"`
	RetryEligible  bool    `json:"retry_eligible"`
	NextRetryAt    string  `json:"next_retry_at,omitempty"`
	IsLatest       bool    `json:"is_latest"`
}

// HistoryStore persists download completion/failure events to SQLite.
type HistoryStore struct {
	db *sql.DB
}

// NewHistoryStore creates a HistoryStore backed by the provided database.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// RecordCompletion inserts a new completed-history row from a DownloadCompletedEvent.
func (h *HistoryStore) RecordCompletion(ctx context.Context, event *DownloadCompletedEvent) error {
	return h.record(ctx, event.DownloadID, event.ClientID, event.Title, event.Category, "completed", event.CompletedAt)
}

// RecordFailure inserts a new failed-history row for a failed download.
func (h *HistoryStore) RecordFailure(
	ctx context.Context,
	clientID, downloadID, title, category string,
	failedAt time.Time,
) error {
	return h.record(ctx, downloadID, clientID, title, category, "failed", failedAt)
}

// WasCompleted checks whether a download was already emitted as completed.
// Used for idempotency across process restarts — prevents re-importing
// a download that was already handled in a previous session.
func (h *HistoryStore) WasCompleted(ctx context.Context, clientID, downloadID string) bool {
	return h.WasRecorded(ctx, clientID, downloadID, "completed")
}

// WasRecorded checks whether a download has already been persisted with
// the given history status.
func (h *HistoryStore) WasRecorded(ctx context.Context, clientID, downloadID, status string) bool {
	var count int
	err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM download_history WHERE client_id = ? AND download_id = ? AND status = ?`,
		clientID, downloadID, status,
	).Scan(&count)
	return err == nil && count > 0
}

func (h *HistoryStore) record(
	ctx context.Context,
	downloadID, clientID, title, category, status string,
	at time.Time,
) error {
	id := uuid.New().String()
	completedAt := at.UTC().Format(time.RFC3339)
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO download_history (id, download_id, client_id, title, category, status, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, downloadID, clientID, title, category, status, completedAt,
	)
	return err
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
		`SELECT id, download_id, client_id, title, category, status, grabbed_at, completed_at,
		 acquisition_key, media_type, media_id, indexer_id, release_hash, stage, reason,
		 attempt, retry_count, retry_eligible, next_retry_at,
		 (acquisition_key <> '' AND attempt = (SELECT MAX(d.attempt) FROM download_history d
		 WHERE d.acquisition_key = download_history.acquisition_key))
		 FROM download_history
		 WHERE status <> 'search_completed'
		 ORDER BY completed_at DESC
		 LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.DownloadID, &e.ClientID, &e.Title, &e.Category, &e.Status, &e.GrabbedAt, &e.CompletedAt,
			&e.AcquisitionKey, &e.MediaType, &e.MediaID, &e.IndexerID, &e.ReleaseHash, &e.Stage, &e.Reason,
			&e.Attempt, &e.RetryCount, &e.RetryEligible, &e.NextRetryAt, &e.IsLatest); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Clear removes completion history while retaining acquisition admission state.
func (h *HistoryStore) Clear(ctx context.Context) error {
	// Admission records must survive clearing the visible completion history.
	_, err := h.db.ExecContext(ctx, `DELETE FROM download_history WHERE acquisition_key = ''`)
	return err
}
