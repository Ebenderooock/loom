package healthmonitor

import (
	"context"
	"testing"
	"time"
)

// --- test doubles ---

type fakeIndexerChecker struct {
	indexers []IndexerInfo
	err      error
}

func (f *fakeIndexerChecker) List(_ context.Context) ([]IndexerInfo, error) {
	return f.indexers, f.err
}

type fakeDownloadChecker struct {
	clients []ClientInfo
	err     error
}

func (f *fakeDownloadChecker) ListClients(_ context.Context) ([]ClientInfo, error) {
	return f.clients, f.err
}

// --- tests ---

func TestRunChecks_NoServices(t *testing.T) {
	m := New(Options{Interval: time.Minute, Cooldown: time.Minute})
	results := m.RunChecks(context.Background())
	// With no lib paths and nil services, no results.
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestRunChecks_DownloadClients(t *testing.T) {
	dl := &fakeDownloadChecker{
		clients: []ClientInfo{
			{ID: "1", Name: "qbit", Enabled: true, Status: "ok"},
			{ID: "2", Name: "sab", Enabled: true, Status: "failed"},
			{ID: "3", Name: "disabled", Enabled: false, Status: "unknown"},
		},
	}
	m := New(Options{Downloads: dl, Interval: time.Minute, Cooldown: time.Minute})
	results := m.RunChecks(context.Background())

	// Should have 2 results (disabled is skipped).
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}
	if results[0].Status != "ok" {
		t.Errorf("expected ok for qbit, got %s", results[0].Status)
	}
	if results[1].Status != "critical" {
		t.Errorf("expected critical for sab, got %s", results[1].Status)
	}
}

func TestRunChecks_Indexers_AllFailing(t *testing.T) {
	idx := &fakeIndexerChecker{
		indexers: []IndexerInfo{
			{ID: "1", Name: "nzb1", Enabled: true, Status: "failed"},
			{ID: "2", Name: "nzb2", Enabled: true, Status: "failed"},
		},
	}
	m := New(Options{Indexers: idx, Interval: time.Minute, Cooldown: time.Minute})
	results := m.RunChecks(context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "critical" {
		t.Errorf("expected critical, got %s", results[0].Status)
	}
}

func TestRunChecks_Indexers_SomeFailing(t *testing.T) {
	idx := &fakeIndexerChecker{
		indexers: []IndexerInfo{
			{ID: "1", Name: "nzb1", Enabled: true, Status: "ok"},
			{ID: "2", Name: "nzb2", Enabled: true, Status: "failed"},
		},
	}
	m := New(Options{Indexers: idx, Interval: time.Minute, Cooldown: time.Minute})
	results := m.RunChecks(context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "warning" {
		t.Errorf("expected warning, got %s", results[0].Status)
	}
}

func TestAlertCooldown(t *testing.T) {
	var sent int
	notifier := func(_ context.Context, _, _ string) error {
		sent++
		return nil
	}

	idx := &fakeIndexerChecker{
		indexers: []IndexerInfo{
			{ID: "1", Name: "nzb1", Enabled: true, Status: "failed"},
		},
	}
	m := New(Options{
		Indexers: idx,
		Notifier: notifier,
		Interval: time.Minute,
		Cooldown: time.Hour,
	})

	ctx := context.Background()
	results := m.RunChecks(ctx)
	m.processAlerts(ctx, results)
	if sent != 1 {
		t.Fatalf("expected 1 alert, got %d", sent)
	}

	// Second run — should be suppressed by cooldown.
	results = m.RunChecks(ctx)
	m.processAlerts(ctx, results)
	if sent != 1 {
		t.Fatalf("expected alert to be suppressed, got %d", sent)
	}
}

func TestLastResults(t *testing.T) {
	m := New(Options{Interval: time.Minute, Cooldown: time.Minute})
	// Initially empty.
	if len(m.LastResults()) != 0 {
		t.Fatal("expected empty initial results")
	}

	dl := &fakeDownloadChecker{
		clients: []ClientInfo{
			{ID: "1", Name: "qbit", Enabled: true, Status: "ok"},
		},
	}
	m.downloadSvc = dl
	m.RunChecks(context.Background())

	results := m.LastResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
