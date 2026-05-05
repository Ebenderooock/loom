package commands

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Queue manages a persistent command queue with a background worker.
type Queue struct {
	db     *sql.DB
	logger *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewQueue creates a command queue with a worker goroutine.
func NewQueue(db *sql.DB, logger *slog.Logger) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{db: db, logger: logger, cancel: cancel}
	q.wg.Add(1)
	go q.worker(ctx)
	return q
}

// Stop signals the worker to exit and waits for it to finish.
func (q *Queue) Stop() {
	q.cancel()
	q.wg.Wait()
}

// Enqueue inserts a command into the queue.
func (q *Queue) Enqueue(ctx context.Context, name CommandName, body string, priority int) (*Command, error) {
	id := generateID()
	if body == "" {
		body = "{}"
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO command_queue (id, name, body, status, priority)
		 VALUES (?, ?, ?, 'queued', ?)`,
		id, string(name), body, priority)
	if err != nil {
		return nil, fmt.Errorf("enqueue command: %w", err)
	}
	return q.Get(ctx, id)
}

// Get returns a command by ID.
func (q *Queue) Get(ctx context.Context, id string) (*Command, error) {
	var c Command
	err := q.db.QueryRowContext(ctx,
		`SELECT id, name, body, status, priority, started_at, completed_at, result, created_at
		 FROM command_queue WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Body, &c.Status, &c.Priority, &c.StartedAt, &c.CompletedAt, &c.Result, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns the most recent commands.
func (q *Queue) List(ctx context.Context, limit int) ([]Command, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, name, body, status, priority, started_at, completed_at, result, created_at
		 FROM command_queue ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}
	defer rows.Close()
	var out []Command
	for rows.Next() {
		var c Command
		if err := rows.Scan(&c.ID, &c.Name, &c.Body, &c.Status, &c.Priority, &c.StartedAt, &c.CompletedAt, &c.Result, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Cancel marks a queued command as failed with a cancellation message.
func (q *Queue) Cancel(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE command_queue SET status = 'failed', result = 'cancelled', completed_at = ?
		 WHERE id = ? AND status = 'queued'`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("cancel command: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (q *Queue) worker(ctx context.Context) {
	defer q.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.processNext(ctx)
		}
	}
}

func (q *Queue) processNext(ctx context.Context) {
	var c Command
	err := q.db.QueryRowContext(ctx,
		`SELECT id, name, body, status, priority, started_at, completed_at, result, created_at
		 FROM command_queue WHERE status = 'queued' ORDER BY priority DESC, created_at ASC LIMIT 1`).
		Scan(&c.ID, &c.Name, &c.Body, &c.Status, &c.Priority, &c.StartedAt, &c.CompletedAt, &c.Result, &c.CreatedAt)
	if err != nil {
		return // no work or error
	}

	now := time.Now().UTC()
	_, _ = q.db.ExecContext(ctx,
		`UPDATE command_queue SET status = 'running', started_at = ? WHERE id = ?`, now, c.ID)

	q.logger.Info("command started", "id", c.ID, "name", c.Name)

	// Execute the command (stub — logs and completes).
	result := q.executeCommand(ctx, c)

	done := time.Now().UTC()
	status := StatusCompleted
	if result != "" && result != "ok" {
		status = StatusFailed
	} else {
		result = "ok"
	}
	_, _ = q.db.ExecContext(ctx,
		`UPDATE command_queue SET status = ?, completed_at = ?, result = ? WHERE id = ?`,
		string(status), done, result, c.ID)

	q.logger.Info("command completed", "id", c.ID, "name", c.Name, "status", status)
}

func (q *Queue) executeCommand(_ context.Context, c Command) string {
	// Placeholder — real implementations would dispatch to movie/series services.
	switch c.Name {
	case CmdRescanMovie, CmdRescanSeries, CmdRefreshMovie, CmdRefreshSeries,
		CmdMissingSearch, CmdCutoffSearch, CmdBackup, CmdRssSync:
		return "ok"
	default:
		return fmt.Sprintf("unknown command: %s", c.Name)
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
