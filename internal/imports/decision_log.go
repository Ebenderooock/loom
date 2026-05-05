package imports

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ImportDecision records every import decision with full context.
type ImportDecision struct {
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"`
	SourcePath     string `json:"source_path"`
	DestPath       string `json:"dest_path"`
	MediaType      string `json:"media_type"`
	MediaID        string `json:"media_id"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	ConflictPolicy string `json:"conflict_policy"`
	FileSize       int64  `json:"file_size"`
	FileQuality    string `json:"file_quality"`
	CreatedAt      string `json:"created_at"`
}

// DecisionLogger records import decisions to the database.
type DecisionLogger struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewDecisionLogger creates a new DecisionLogger.
func NewDecisionLogger(db *sql.DB, logger *slog.Logger) *DecisionLogger {
	return &DecisionLogger{db: db, logger: logger}
}

// Log records a single import decision.
func (dl *DecisionLogger) Log(ctx context.Context, d ImportDecision) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if d.Timestamp == "" {
		d.Timestamp = now
	}
	if d.CreatedAt == "" {
		d.CreatedAt = now
	}

	_, err := dl.db.ExecContext(ctx,
		`INSERT INTO import_decisions (id, timestamp, source_path, dest_path, media_type, media_id, action, reason, conflict_policy, file_size, file_quality, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.Timestamp, d.SourcePath, d.DestPath,
		d.MediaType, d.MediaID, d.Action, d.Reason,
		d.ConflictPolicy, d.FileSize, d.FileQuality, d.CreatedAt,
	)
	if err != nil {
		dl.logger.Error("failed to log import decision", "error", err, "action", d.Action, "source", d.SourcePath)
		return fmt.Errorf("log import decision: %w", err)
	}

	dl.logger.Info("import decision logged",
		"action", d.Action,
		"source", d.SourcePath,
		"dest", d.DestPath,
		"reason", d.Reason,
	)
	return nil
}

// ListDecisions returns paginated import decisions, optionally filtered by media_id.
func (dl *DecisionLogger) ListDecisions(ctx context.Context, mediaID string, limit, offset int) ([]*ImportDecision, error) {
	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if mediaID != "" {
		rows, err = dl.db.QueryContext(ctx,
			`SELECT id, timestamp, source_path, dest_path, media_type, media_id, action, reason, conflict_policy, file_size, file_quality, created_at
			 FROM import_decisions
			 WHERE media_id = ?
			 ORDER BY created_at DESC
			 LIMIT ? OFFSET ?`,
			mediaID, limit, offset,
		)
	} else {
		rows, err = dl.db.QueryContext(ctx,
			`SELECT id, timestamp, source_path, dest_path, media_type, media_id, action, reason, conflict_policy, file_size, file_quality, created_at
			 FROM import_decisions
			 ORDER BY created_at DESC
			 LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query import decisions: %w", err)
	}
	defer rows.Close()

	var decisions []*ImportDecision
	for rows.Next() {
		var d ImportDecision
		if err := rows.Scan(
			&d.ID, &d.Timestamp, &d.SourcePath, &d.DestPath,
			&d.MediaType, &d.MediaID, &d.Action, &d.Reason,
			&d.ConflictPolicy, &d.FileSize, &d.FileQuality, &d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan import decision: %w", err)
		}
		decisions = append(decisions, &d)
	}
	return decisions, rows.Err()
}
