package scheduler

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

type stubPlayHistoryPruner struct {
	deleted int64
	err     error
	cutoff  time.Time
}

func (s *stubPlayHistoryPruner) PruneHistory(_ context.Context, olderThan time.Time) (int64, error) {
	s.cutoff = olderThan
	if s.err != nil {
		return 0, s.err
	}
	return s.deleted, nil
}

func TestRegisterPlayHistoryPrune_Validation(t *testing.T) {
	s, err := New(Config{Enabled: true}, newMemStore(), slog.Default(), SystemClock{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pruner := &stubPlayHistoryPruner{}
	if err := RegisterPlayHistoryPrune(context.Background(), nil, pruner, 30, slog.Default()); err == nil {
		t.Fatalf("expected error for nil scheduler")
	}
	if err := RegisterPlayHistoryPrune(context.Background(), s, nil, 30, slog.Default()); err == nil {
		t.Fatalf("expected error for nil pruner")
	}
}

func TestRegisterPlayHistoryPrune_RegistersJob(t *testing.T) {
	store := newMemStore()
	s, err := New(Config{Enabled: true}, store, slog.Default(), SystemClock{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pruner := &stubPlayHistoryPruner{}
	if err := RegisterPlayHistoryPrune(context.Background(), s, pruner, 0, slog.Default()); err != nil {
		t.Fatalf("RegisterPlayHistoryPrune: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	upsert, ok := store.upserts[PlayHistoryPruneJobName]
	if !ok {
		t.Fatalf("expected %s to be registered", PlayHistoryPruneJobName)
	}
	if upsert.schedule != PlayHistoryPruneSchedule {
		t.Fatalf("schedule = %q, want %q", upsert.schedule, PlayHistoryPruneSchedule)
	}
}

func TestRegisterAuditPrune_StillValidatesScheduler(t *testing.T) {
	if err := RegisterAuditPrune(context.Background(), nil, nil, 30, slog.Default()); err == nil {
		t.Fatalf("expected error for nil scheduler")
	}
}
