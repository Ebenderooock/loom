package systemlogs

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/logging"
	_ "modernc.org/sqlite"
)

func TestBatchWriter_InsertFailureLoggedAsError(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE system_logs (
		id TEXT PRIMARY KEY,
		timestamp TEXT NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		source TEXT,
		attrs TEXT,
		workflow_id TEXT
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	store := NewStore(db)
	var out bytes.Buffer
	store.logger = slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	bw := store.NewBatchWriter(context.Background())

	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	bw.Sink() <- logging.LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     "error",
		Message:   "test message",
	}
	bw.Close()

	logged := out.String()
	if !strings.Contains(logged, "level=ERROR") {
		t.Fatalf("expected error-level log, got %q", logged)
	}
	if !strings.Contains(logged, "system log insert failed") {
		t.Fatalf("expected failure message, got %q", logged)
	}
}
