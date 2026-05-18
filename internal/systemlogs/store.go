// Package systemlogs provides persistent storage and HTTP handlers for
// application-level log entries captured by the slog capture handler.
package systemlogs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/kernel/logging"
)

// Store persists log entries to the database and supports filtered queries.
type Store struct {
	db *sql.DB
}

// NewStore creates a new log store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ListFilter controls paginated retrieval of log entries.
type ListFilter struct {
	Level      string
	Search     string // substring match on message
	WorkflowID string
	Since      string // ISO-8601 timestamp
	Until      string // ISO-8601 timestamp
	Limit      int
	Offset     int
}

// ListResult is the paginated response envelope.
type ListResult struct {
	Items []logging.LogEntry `json:"items"`
	Total int                `json:"total"`
}

// Insert persists a single log entry.
func (s *Store) Insert(ctx context.Context, entry logging.LogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO system_logs (id, timestamp, level, message, source, attrs, workflow_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.Level, entry.Message,
		nullString(entry.Source), nullString(entry.Attrs), nullString(entry.WorkflowID),
	)
	return err
}

// List returns paginated log entries matching the filter.
func (s *Store) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	where, args := buildWhere(f)

	// Count total.
	var total int
	countQ := "SELECT COUNT(*) FROM system_logs" + where
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("system_logs count: %w", err)
	}

	// Fetch page.
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	selectQ := `SELECT id, timestamp, level, message,
	            COALESCE(source,''), COALESCE(attrs,''), COALESCE(workflow_id,'')
	            FROM system_logs` + where +
		` ORDER BY timestamp DESC LIMIT ? OFFSET ?`
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, selectQ, args...)
	if err != nil {
		return nil, fmt.Errorf("system_logs list: %w", err)
	}
	defer rows.Close()

	var items []logging.LogEntry
	for rows.Next() {
		var e logging.LogEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Level, &e.Message,
			&e.Source, &e.Attrs, &e.WorkflowID); err != nil {
			return nil, fmt.Errorf("system_logs scan: %w", err)
		}
		items = append(items, e)
	}
	if items == nil {
		items = []logging.LogEntry{}
	}
	return &ListResult{Items: items, Total: total}, nil
}

// Prune deletes log entries older than the given time.
func (s *Store) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	ts := olderThan.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM system_logs WHERE timestamp < ?`, ts)
	if err != nil {
		return 0, fmt.Errorf("system_logs prune: %w", err)
	}
	return res.RowsAffected()
}

// Clear deletes all log entries.
func (s *Store) Clear(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM system_logs`)
	return err
}

func buildWhere(f ListFilter) (string, []any) {
	var clauses []string
	var args []any

	if f.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, strings.ToLower(f.Level))
	}
	if f.Search != "" {
		clauses = append(clauses, "message LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}
	if f.WorkflowID != "" {
		clauses = append(clauses, "workflow_id = ?")
		args = append(args, f.WorkflowID)
	}
	if f.Since != "" {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, f.Since)
	}
	if f.Until != "" {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, f.Until)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// BatchWriter receives log entries from a channel and inserts them in batches.
type BatchWriter struct {
	store     *Store
	ch        chan logging.LogEntry
	batchSize int
	flushMS   int
	done      chan struct{}
}

// NewBatchWriter starts a goroutine that consumes from a channel and
// batch-inserts entries. The caller should use Sink() to get the
// write-end channel, and Close() to shut down.
func (s *Store) NewBatchWriter(_ context.Context) *BatchWriter {
	ch := make(chan logging.LogEntry, 1024)
	bw := &BatchWriter{
		store:     s,
		ch:        ch,
		batchSize: 100,
		flushMS:   500,
		done:      make(chan struct{}),
	}
	go bw.run()
	return bw
}

// Sink returns the channel the capture handler should write to.
func (bw *BatchWriter) Sink() chan<- logging.LogEntry {
	return bw.ch
}

// Close shuts down the batch writer, flushing remaining entries.
func (bw *BatchWriter) Close() {
	close(bw.ch)
	<-bw.done
}

func (bw *BatchWriter) run() {
	defer close(bw.done)
	ticker := time.NewTicker(time.Duration(bw.flushMS) * time.Millisecond)
	defer ticker.Stop()

	var batch []logging.LogEntry

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		for _, e := range batch {
			_ = bw.store.Insert(ctx, e)
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-bw.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= bw.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Wait blocks until the batch writer has finished processing.
func (bw *BatchWriter) Wait() {
	<-bw.done
}
