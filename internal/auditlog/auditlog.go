package auditlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TimestampFormat is the canonical ISO-8601 layout used for all audit
// log timestamps. Using a single constant avoids drift between writers.
const TimestampFormat = "2006-01-02T15:04:05.000Z"

// FormatTime converts t to the canonical audit log timestamp string.
// A zero time returns "" to avoid misleading year-0001 timestamps.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(TimestampFormat)
}

// Entry is a single audit log record.
type Entry struct {
	ID         string  `json:"id"`
	Timestamp  string  `json:"timestamp"`
	OccurredAt string  `json:"occurred_at,omitempty"`
	Category   string  `json:"category"`
	EventType  string  `json:"event_type"`
	Message    string  `json:"message"`
	Detail     *string `json:"detail,omitempty"`
	EntityType *string `json:"entity_type,omitempty"`
	EntityID   *string `json:"entity_id,omitempty"`
	EntityName *string `json:"entity_name,omitempty"`
	Level      string  `json:"level"`
	Source     *string `json:"source,omitempty"`
}

// ListFilter controls paginated retrieval.
type ListFilter struct {
	Category   string
	EventType  string
	Level      string
	EntityType string
	EntityID   string
	Limit      int
	Offset     int
	Since      string // ISO-8601 timestamp
	Until      string // ISO-8601 timestamp
}

// ListResult is the paginated response envelope.
type ListResult struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
	Limit   int     `json:"limit"`
	Offset  int     `json:"offset"`
}

// Logger reads and writes audit log entries.
type Logger struct {
	db     *sql.DB
	logger *slog.Logger
}

// New creates a Logger backed by db.
func New(db *sql.DB, logger *slog.Logger) *Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return &Logger{db: db, logger: logger.With("module", "auditlog")}
}

// Log inserts an audit log entry. The ID and Timestamp fields are
// auto-populated if empty. Safe to call on a nil receiver (no-op).
func (l *Logger) Log(ctx context.Context, e Entry) {
	if l == nil {
		return
	}
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.Timestamp == "" {
		e.Timestamp = FormatTime(time.Now())
	}
	if e.Level == "" {
		e.Level = "info"
	}

	_, err := l.db.ExecContext(ctx,
		`INSERT INTO audit_log (id, timestamp, occurred_at, category, event_type, message, detail, entity_type, entity_id, entity_name, level, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Timestamp, e.OccurredAt, e.Category, e.EventType, e.Message,
		e.Detail, e.EntityType, e.EntityID, e.EntityName, e.Level, e.Source,
	)
	if err != nil {
		l.logger.Warn("audit log insert failed", "err", err)
	}
}

// LogBackground writes an audit entry using a detached context so that
// HTTP request cancellation cannot prevent the write. Use this from
// event bus subscribers or background goroutines. Safe to call on nil.
func (l *Logger) LogBackground(e Entry) {
	if l == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l.Log(ctx, e)
}

// List returns paginated audit log entries matching filter.
func (l *Logger) List(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}

	var where []string
	var args []any

	if f.Category != "" {
		where = append(where, "category = ?")
		args = append(args, f.Category)
	}
	if f.EventType != "" {
		where = append(where, "event_type = ?")
		args = append(args, f.EventType)
	}
	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, f.Level)
	}
	if f.EntityType != "" {
		where = append(where, "entity_type = ?")
		args = append(args, f.EntityType)
	}
	if f.EntityID != "" {
		where = append(where, "entity_id = ?")
		args = append(args, f.EntityID)
	}
	if f.Since != "" {
		where = append(where, "timestamp >= ?")
		args = append(args, f.Since)
	}
	if f.Until != "" {
		where = append(where, "timestamp <= ?")
		args = append(args, f.Until)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	// Total count.
	var total int
	countQ := "SELECT COUNT(*) FROM audit_log" + whereClause
	if err := l.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("audit log count: %w", err)
	}

	// Rows.
	dataQ := "SELECT id, timestamp, occurred_at, category, event_type, message, detail, entity_type, entity_id, entity_name, level, source FROM audit_log" +
		whereClause + " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	dataArgs := append(append([]any{}, args...), f.Limit, f.Offset)

	rows, err := l.db.QueryContext(ctx, dataQ, dataArgs...)
	if err != nil {
		return ListResult{}, fmt.Errorf("audit log query: %w", err)
	}
	defer rows.Close()

	entries := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.OccurredAt, &e.Category, &e.EventType, &e.Message,
			&e.Detail, &e.EntityType, &e.EntityID, &e.EntityName, &e.Level, &e.Source); err != nil {
			return ListResult{}, fmt.Errorf("audit log scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("audit log rows: %w", err)
	}

	return ListResult{
		Entries: entries,
		Total:   total,
		Limit:   f.Limit,
		Offset:  f.Offset,
	}, nil
}

// Prune deletes entries older than age.
func (l *Logger) Prune(ctx context.Context, age time.Duration) (int64, error) {
	cutoff := FormatTime(time.Now().Add(-age))
	res, err := l.db.ExecContext(ctx,
		`DELETE FROM audit_log WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("audit log prune: %w", err)
	}
	return res.RowsAffected()
}

// ── HTTP handlers ──────────────────────────────────────────────────────

// Mount registers audit log routes onto r.
func (l *Logger) Mount(r chi.Router) {
	r.Get("/api/v1/system/audit-log", l.handleList)
}

func (l *Logger) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	result, err := l.List(r.Context(), ListFilter{
		Category:   q.Get("category"),
		EventType:  q.Get("event_type"),
		Level:      q.Get("level"),
		EntityType: q.Get("entity_type"),
		EntityID:   q.Get("entity_id"),
		Limit:      limit,
		Offset:     offset,
		Since:      q.Get("since"),
		Until:      q.Get("until"),
	})
	if err != nil {
		l.logger.Error("audit log list failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// ── Convenience helpers ────────────────────────────────────────────────

// StrPtr returns a pointer to s, or nil if s is empty.
func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// DetailJSON marshals v to a JSON string pointer for use in Entry.Detail.
func DetailJSON(v any) *string {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}
