package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store provides persistence for workflows.
type Store struct {
	db *sql.DB
}

// NewStore creates the workflow store and ensures tables exist.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("workflows: migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			state TEXT NOT NULL,
			media_type TEXT NOT NULL,
			grab_title TEXT,
			download_client_id TEXT,
			download_id TEXT,
			quality_profile_id TEXT,
			retry_count INTEGER DEFAULT 0,
			max_retries INTEGER DEFAULT 3,
			last_error TEXT,
			metadata TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS workflow_items (
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			media_type TEXT NOT NULL,
			media_id TEXT NOT NULL,
			PRIMARY KEY (workflow_id, media_id)
		);

		CREATE TABLE IF NOT EXISTS workflow_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			from_state TEXT NOT NULL,
			to_state TEXT NOT NULL,
			message TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_workflows_state ON workflows(state);
		CREATE INDEX IF NOT EXISTS idx_workflow_items_media ON workflow_items(media_type, media_id);

		CREATE TABLE IF NOT EXISTS workflow_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			message TEXT,
			metadata TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_workflow_events_wf ON workflow_events(workflow_id);
	`)
	if err != nil {
		return err
	}

	// Partial unique index for active download constraints — SQLite supports this
	_, _ = s.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflows_active_download
		ON workflows(download_client_id, download_id)
		WHERE download_client_id IS NOT NULL AND download_id IS NOT NULL
		AND state NOT IN ('completed', 'failed', 'cancelled');
	`)
	return nil
}

// Create persists a new workflow with its items.
func (s *Store) Create(ctx context.Context, w *Workflow) error {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	now := time.Now()
	w.CreatedAt = now
	w.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO workflows (id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Type, w.State, w.MediaType, w.GrabTitle,
		nullStr(w.DownloadClientID), nullStr(w.DownloadID), nullStr(w.QualityProfileID),
		w.RetryCount, w.MaxRetries, nullStr(w.LastError), nullStr(w.Metadata),
		w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert workflow: %w", err)
	}

	for _, item := range w.Items {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO workflow_items (workflow_id, media_type, media_id)
			VALUES (?, ?, ?)`,
			w.ID, item.MediaType, item.MediaID,
		)
		if err != nil {
			return fmt.Errorf("insert workflow item: %w", err)
		}
	}

	// Record initial history event
	_, err = tx.ExecContext(ctx, `
		INSERT INTO workflow_history (workflow_id, from_state, to_state, message, created_at)
		VALUES (?, '', ?, 'Workflow created', ?)`,
		w.ID, w.State, now,
	)
	if err != nil {
		return fmt.Errorf("insert history: %w", err)
	}

	return tx.Commit()
}

// Transition atomically moves a workflow to a new state (idempotent guard).
// Returns false if the workflow was not in the expected current state.
func (s *Store) Transition(ctx context.Context, id, fromState, toState, message string) (bool, error) {
	now := time.Now()
	var completedAt *time.Time
	if toState == StateCompleted || toState == StateFailed || toState == StateCancelled {
		completedAt = &now
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE workflows SET state = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND state = ?`,
		toState, now, completedAt, id, fromState,
	)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return false, nil // already transitioned by another worker
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO workflow_history (workflow_id, from_state, to_state, message, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, fromState, toState, message, now,
	)
	if err != nil {
		return false, err
	}

	return true, tx.Commit()
}

// SetError records an error on the workflow.
func (s *Store) SetError(ctx context.Context, id, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows SET last_error = ?, updated_at = ? WHERE id = ?`,
		errMsg, time.Now(), id,
	)
	return err
}

// IncrementRetry bumps retry count and records the error.
func (s *Store) IncrementRetry(ctx context.Context, id, errMsg string) (int, error) {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows SET retry_count = retry_count + 1, last_error = ?, updated_at = ?
		WHERE id = ?`, errMsg, now, id,
	)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT retry_count FROM workflows WHERE id = ?`, id).Scan(&count)
	return count, err
}

// SetDownload sets download client info on a workflow.
func (s *Store) SetDownload(ctx context.Context, id, clientID, downloadID, title string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows SET download_client_id = ?, download_id = ?, grab_title = ?, updated_at = ?
		WHERE id = ?`, clientID, downloadID, title, time.Now(), id,
	)
	return err
}

// ResetAttempt clears retry/error/completion fields when a workflow starts a
// fresh grab attempt (for example, redownloading the same episode).
func (s *Store) ResetAttempt(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows
		SET retry_count = 0, last_error = NULL, completed_at = NULL, updated_at = ?
		WHERE id = ?`, time.Now(), id,
	)
	return err
}

// SetMetadata updates the workflow's metadata JSON blob.
func (s *Store) SetMetadata(ctx context.Context, id, metadata string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows SET metadata = ?, updated_at = ?
		WHERE id = ?`, nullStr(metadata), time.Now(), id,
	)
	return err
}

// Get retrieves a workflow by ID with items and history.
func (s *Store) Get(ctx context.Context, id string) (*Workflow, error) {
	w := &Workflow{}
	var completedAt sql.NullTime
	var grabTitle, dlClient, dlID, qpID, lastErr, metadata sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata,
			created_at, updated_at, completed_at
		FROM workflows WHERE id = ?`, id,
	).Scan(&w.ID, &w.Type, &w.State, &w.MediaType, &grabTitle,
		&dlClient, &dlID, &qpID,
		&w.RetryCount, &w.MaxRetries, &lastErr, &metadata,
		&w.CreatedAt, &w.UpdatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	w.GrabTitle = grabTitle.String
	w.DownloadClientID = dlClient.String
	w.DownloadID = dlID.String
	w.QualityProfileID = qpID.String
	w.LastError = lastErr.String
	w.Metadata = metadata.String
	if completedAt.Valid {
		w.CompletedAt = &completedAt.Time
	}

	// Load items
	w.Items, err = s.getItems(ctx, id)
	if err != nil {
		return nil, err
	}

	// Load history
	w.History, err = s.getHistory(ctx, id)
	if err != nil {
		return nil, err
	}

	return w, nil
}

// ListActive returns all non-terminal workflows.
func (s *Store) ListActive(ctx context.Context) ([]*Workflow, error) {
	return s.list(ctx, `
		SELECT id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata,
			created_at, updated_at, completed_at
		FROM workflows
		WHERE state NOT IN ('completed', 'failed', 'cancelled')
		ORDER BY created_at DESC`)
}

// ListRecent returns recent workflows (active + recently completed).
func (s *Store) ListRecent(ctx context.Context, limit int) ([]*Workflow, error) {
	return s.listArgs(ctx, `
		SELECT id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata,
			created_at, updated_at, completed_at
		FROM workflows
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
}

// FindByDownload looks up active workflow by download client + download ID.
func (s *Store) FindByDownload(ctx context.Context, clientID, downloadID string) (*Workflow, error) {
	rows, err := s.listArgs(ctx, `
		SELECT id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata,
			created_at, updated_at, completed_at
		FROM workflows
		WHERE download_client_id = ? AND download_id = ?
		AND state NOT IN ('completed', 'failed', 'cancelled')
		LIMIT 1`, clientID, downloadID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	// Load items
	rows[0].Items, _ = s.getItems(ctx, rows[0].ID)
	return rows[0], nil
}

// FindActiveForMedia checks if there's an active workflow for a given media item.
func (s *Store) FindActiveForMedia(ctx context.Context, mediaType, mediaID string) (*Workflow, error) {
	var wfID string
	err := s.db.QueryRowContext(ctx, `
		SELECT wi.workflow_id FROM workflow_items wi
		JOIN workflows w ON w.id = wi.workflow_id
		WHERE wi.media_type = ? AND wi.media_id = ?
		AND w.state NOT IN ('completed', 'failed', 'cancelled')
		LIMIT 1`, mediaType, mediaID,
	).Scan(&wfID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, wfID)
}

// PruneCompleted removes terminal workflows older than maxAge.
func (s *Store) PruneCompleted(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM workflows
		WHERE state IN ('completed', 'failed', 'cancelled')
		AND completed_at < ?`, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ActiveMediaIDs returns the set of media IDs that have active workflows.
func (s *Store) ActiveMediaIDs(ctx context.Context, mediaType string, ids []string) (map[string]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `
		SELECT DISTINCT wi.media_id FROM workflow_items wi
		JOIN workflows w ON w.id = wi.workflow_id
		WHERE wi.media_type = ? AND w.state NOT IN ('completed', 'failed', 'cancelled')
		AND wi.media_id IN (`
	args := []any{mediaType}
	for i, id := range ids {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, id)
	}
	query += ")"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}

// StaleWorkflows returns workflows stuck in a state longer than expected.
// Downloading workflows are included separately using DownloadingStaleThreshold
// so that large files on slow connections are not falsely marked stale.
func (s *Store) StaleWorkflows(ctx context.Context) ([]*Workflow, error) {
	now := time.Now()
	var results []*Workflow

	for state, timeout := range StateTimeouts {
		cutoff := now.Add(-timeout)
		wfs, err := s.listArgs(ctx, `
			SELECT id, type, state, media_type, grab_title,
				download_client_id, download_id, quality_profile_id,
				retry_count, max_retries, last_error, metadata,
				created_at, updated_at, completed_at
			FROM workflows
			WHERE state = ? AND updated_at < ?`,
			string(state), cutoff)
		if err != nil {
			return nil, err
		}
		results = append(results, wfs...)
	}

	// Include downloading workflows that have been silent beyond the generous
	// threshold. The orchestrator will apply further progress-aware logic.
	dlCutoff := now.Add(-DownloadingStaleThreshold)
	dlWfs, err := s.listArgs(ctx, `
		SELECT id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata,
			created_at, updated_at, completed_at
		FROM workflows
		WHERE state = ? AND updated_at < ?`,
		StateDownloading, dlCutoff)
	if err != nil {
		return nil, err
	}
	results = append(results, dlWfs...)

	return results, nil
}

// ResetRetry clears retry count and last error for a workflow (used on manual retry).
func (s *Store) ResetRetry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows SET retry_count = 0, last_error = NULL, updated_at = ?
		WHERE id = ?`, time.Now(), id,
	)
	return err
}

// ListRecentlyFailed returns workflows in failed state updated since the given time.
func (s *Store) ListRecentlyFailed(ctx context.Context, since time.Time) ([]*Workflow, error) {
	return s.listArgs(ctx, `
		SELECT id, type, state, media_type, grab_title,
			download_client_id, download_id, quality_profile_id,
			retry_count, max_retries, last_error, metadata,
			created_at, updated_at, completed_at
		FROM workflows
		WHERE state = 'failed' AND updated_at >= ?
		ORDER BY updated_at DESC`, since)
}

// Delete removes a workflow and its items/history (cascade).
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.ExecContext(ctx, `DELETE FROM workflow_history WHERE workflow_id = ?`, id)
	tx.ExecContext(ctx, `DELETE FROM workflow_items WHERE workflow_id = ?`, id)
	_, err = tx.ExecContext(ctx, `DELETE FROM workflows WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// MergeItems adds items to an existing workflow, ignoring duplicates.
func (s *Store) MergeItems(ctx context.Context, workflowID string, items []Item) error {
	for _, item := range items {
		_, _ = s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO workflow_items (workflow_id, media_type, media_id)
			VALUES (?, ?, ?)`,
			workflowID, item.MediaType, item.MediaID,
		)
	}
	return nil
}

// --- Helpers ---

func (s *Store) list(ctx context.Context, query string) ([]*Workflow, error) {
	return s.listArgs(ctx, query)
}

func (s *Store) listArgs(ctx context.Context, query string, args ...any) ([]*Workflow, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Workflow
	for rows.Next() {
		w := &Workflow{}
		var completedAt sql.NullTime
		var grabTitle, dlClient, dlID, qpID, lastErr, metadata sql.NullString
		if err := rows.Scan(&w.ID, &w.Type, &w.State, &w.MediaType, &grabTitle,
			&dlClient, &dlID, &qpID,
			&w.RetryCount, &w.MaxRetries, &lastErr, &metadata,
			&w.CreatedAt, &w.UpdatedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		w.GrabTitle = grabTitle.String
		w.DownloadClientID = dlClient.String
		w.DownloadID = dlID.String
		w.QualityProfileID = qpID.String
		w.LastError = lastErr.String
		w.Metadata = metadata.String
		if completedAt.Valid {
			w.CompletedAt = &completedAt.Time
		}
		results = append(results, w)
	}
	return results, rows.Err()
}

// GetItems returns the media items linked to a workflow.
func (s *Store) GetItems(ctx context.Context, workflowID string) ([]Item, error) {
	return s.getItems(ctx, workflowID)
}

func (s *Store) getItems(ctx context.Context, workflowID string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT workflow_id, media_type, media_id
		FROM workflow_items WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.WorkflowID, &item.MediaType, &item.MediaID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) getHistory(ctx context.Context, workflowID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_id, from_state, to_state, message, created_at
		FROM workflow_history WHERE workflow_id = ?
		ORDER BY created_at ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var ev Event
		var msg sql.NullString
		if err := rows.Scan(&ev.ID, &ev.WorkflowID, &ev.FromState, &ev.ToState,
			&msg, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Message = msg.String
		events = append(events, ev)
	}
	return events, rows.Err()
}

// LogEvent writes a rich audit event to the workflow_events table.
func (s *Store) LogEvent(ctx context.Context, workflowID, eventType, message, metadata string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_events (workflow_id, event_type, message, metadata, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		workflowID, eventType, nullStr(message), nullStr(metadata), time.Now(),
	)
	return err
}

// ListEvents returns all audit events for a workflow, ordered chronologically.
func (s *Store) ListEvents(ctx context.Context, workflowID string) ([]WorkflowEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_id, event_type, message, metadata, created_at
		FROM workflow_events WHERE workflow_id = ?
		ORDER BY created_at ASC, id ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []WorkflowEvent
	for rows.Next() {
		var ev WorkflowEvent
		var msg, meta sql.NullString
		if err := rows.Scan(&ev.ID, &ev.WorkflowID, &ev.EventType, &msg, &meta, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Message = msg.String
		ev.Metadata = meta.String
		events = append(events, ev)
	}
	return events, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// MetadataFromMap converts a map to JSON metadata string.
func MetadataFromMap(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// PostDownloadPolicy holds the seed requirements and settling config
// persisted in workflow metadata under the "post_download" key.
type PostDownloadPolicy struct {
	SeedRatioLimit       *float64  `json:"seed_ratio_limit,omitempty"`
	SeedTimeLimitMinutes *int      `json:"seed_time_limit_minutes,omitempty"`
	SettlingDelay        int       `json:"settling_delay_seconds,omitempty"` // default 5
	StartedAt            time.Time `json:"started_at,omitempty"`
}

// Default settling delay when no explicit value is configured.
const DefaultSettlingDelaySec = 5

// SetPostDownloadPolicy merges seed policy into workflow metadata without
// overwriting other keys.
func (s *Store) SetPostDownloadPolicy(ctx context.Context, id string, policy PostDownloadPolicy) error {
	wf, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	m := make(map[string]any)
	if wf.Metadata != "" {
		_ = json.Unmarshal([]byte(wf.Metadata), &m)
	}
	m["post_download"] = policy
	return s.SetMetadata(ctx, id, MetadataFromMap(m))
}

// GetPostDownloadPolicy reads the seed policy from workflow metadata.
func GetPostDownloadPolicy(metadata string) *PostDownloadPolicy {
	if metadata == "" {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return nil
	}
	raw, ok := m["post_download"]
	if !ok {
		return nil
	}
	var p PostDownloadPolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	return &p
}

// MergeMetadata updates specific keys in the workflow metadata JSON without
// overwriting other keys.
func (s *Store) MergeMetadata(ctx context.Context, id string, patch map[string]any) error {
	wf, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	m := make(map[string]any)
	if wf.Metadata != "" {
		_ = json.Unmarshal([]byte(wf.Metadata), &m)
	}
	for k, v := range patch {
		m[k] = v
	}
	return s.SetMetadata(ctx, id, MetadataFromMap(m))
}
