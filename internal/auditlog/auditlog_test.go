package auditlog

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestFormatTime_Normal(t *testing.T) {
	ts := time.Date(2025, 6, 15, 14, 30, 45, 123000000, time.UTC)
	got := FormatTime(ts)
	want := "2025-06-15T14:30:45.123Z"
	if got != want {
		t.Errorf("FormatTime(%v) = %q, want %q", ts, got, want)
	}
}

func TestFormatTime_ConvertsToUTC(t *testing.T) {
	loc := time.FixedZone("UTC+5", 5*3600)
	ts := time.Date(2025, 6, 15, 19, 0, 0, 0, loc) // 19:00 UTC+5 = 14:00 UTC
	got := FormatTime(ts)
	want := "2025-06-15T14:00:00.000Z"
	if got != want {
		t.Errorf("FormatTime(%v) = %q, want %q", ts, got, want)
	}
}

func TestFormatTime_Zero(t *testing.T) {
	got := FormatTime(time.Time{})
	if got != "" {
		t.Errorf("FormatTime(zero) = %q, want empty string", got)
	}
}

func TestLogger_NilReceiver_Log(t *testing.T) {
	var l *Logger
	// Must not panic
	l.Log(context.Background(), Entry{
		Category:  "test",
		EventType: "test.event",
		Message:   "should not panic",
		Level:     "info",
	})
}

func TestLogger_NilReceiver_LogBackground(t *testing.T) {
	var l *Logger
	// Must not panic
	l.LogBackground(Entry{
		Category:  "test",
		EventType: "test.event",
		Message:   "should not panic",
		Level:     "info",
	})
}

func TestLogger_Prune(t *testing.T) {
	db := setupTestDB(t)
	logger := New(db, slog.Default())

	// Insert an old entry
	old := FormatTime(time.Now().Add(-60 * 24 * time.Hour))
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO audit_log (id, timestamp, category, event_type, message, level)
		 VALUES (?, ?, 'test', 'test.old', 'old entry', 'info')`,
		"old-1", old)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a recent entry
	recent := FormatTime(time.Now())
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO audit_log (id, timestamp, category, event_type, message, level)
		 VALUES (?, ?, 'test', 'test.recent', 'recent entry', 'info')`,
		"recent-1", recent)
	if err != nil {
		t.Fatal(err)
	}

	n, err := logger.Prune(context.Background(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Errorf("Prune deleted %d rows, want 1", n)
	}

	// Verify recent entry still exists
	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_log WHERE id = 'recent-1'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("recent entry count = %d, want 1", count)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE audit_log (
		id          TEXT PRIMARY KEY,
		timestamp   TEXT NOT NULL,
		occurred_at TEXT,
		category    TEXT NOT NULL,
		event_type  TEXT NOT NULL,
		message     TEXT NOT NULL,
		detail      TEXT,
		entity_type TEXT,
		entity_id   TEXT,
		entity_name TEXT,
		level       TEXT NOT NULL DEFAULT 'info',
		source      TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
