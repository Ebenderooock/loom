package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestDefaultRollingSearchConfigEnabledByDefault(t *testing.T) {
	cfg := DefaultRollingSearchConfig()

	if !cfg.Enabled {
		t.Fatalf("expected rolling search to be enabled by default")
	}
	if cfg.IntervalHours != 12 {
		t.Fatalf("expected intervalHours default 12, got %d", cfg.IntervalHours)
	}
	if cfg.BatchSize != 5 {
		t.Fatalf("expected batchSize default 5, got %d", cfg.BatchSize)
	}
	if cfg.MinResearchDays != 7 {
		t.Fatalf("expected minResearchDays default 7, got %d", cfg.MinResearchDays)
	}
	if cfg.MaxSearchesPerDay != 100 {
		t.Fatalf("expected maxSearchesPerDay default 100, got %d", cfg.MaxSearchesPerDay)
	}
}

func TestUpdateConfigPreservesExplicitDisabledOverride(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rs := NewRollingSearcher(nil, nil, logger, DefaultRollingSearchConfig())

	cfg := rs.Config()
	cfg.Enabled = false
	rs.UpdateConfig(context.Background(), cfg)

	got := rs.Config()
	if got.Enabled {
		t.Fatalf("expected explicit enabled=false override to be preserved")
	}
}
