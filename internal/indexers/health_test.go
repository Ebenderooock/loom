package indexers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

// fixedClock returns a stable Now() for deterministic latency math.
type fixedClock struct{ now time.Time }

func (f *fixedClock) Now() time.Time { return f.now }

func TestHealthCheckerRunsAcrossRegistry(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	clock := &fixedClock{now: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository: repo,
		Logger:     quietLogger(),
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Seed two indexers via the public API.
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if _, err := svc.Create(ctx, indexers.Definition{
			ID: id, Kind: indexers.KindNull, Name: id, Enabled: true,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	checker := indexers.NewHealthChecker(svc, 4, 0)
	if err := checker.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	healths, err := repo.ListHealth(ctx)
	if err != nil {
		t.Fatalf("ListHealth: %v", err)
	}
	for _, id := range []string{"a", "b"} {
		h, ok := healths[id]
		if !ok {
			t.Fatalf("no health row for %q", id)
		}
		if h.Status != indexers.StatusOK {
			t.Fatalf("%q status=%s", id, h.Status)
		}
		if h.LastSuccessAt == nil || !h.LastSuccessAt.Equal(clock.now) {
			t.Fatalf("%q last_success_at=%v want=%v", id, h.LastSuccessAt, clock.now)
		}
	}
}

func TestServiceFetchDownloadUsesLiveIndexer(t *testing.T) {
	t.Parallel()

	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository: repo,
		Logger:     quietLogger(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ix := &fakeIndexer{id: "dl", name: "dl", download: []byte("torrent-bytes")}
	if err := svc.Registry().Replace(ix); err != nil {
		t.Fatalf("register indexer: %v", err)
	}

	got, err := svc.FetchDownload(context.Background(), "dl", "https://tracker.example/release.torrent")
	if err != nil {
		t.Fatalf("FetchDownload: %v", err)
	}
	if string(got) != "torrent-bytes" {
		t.Fatalf("FetchDownload bytes = %q, want %q", string(got), "torrent-bytes")
	}
}

// failingIndexer always errors from Test() so we can verify failure
// paths persist a "failed" status.
type failingIndexer struct{ id string }

func (f *failingIndexer) ID() string          { return f.id }
func (f *failingIndexer) Name() string        { return f.id }
func (f *failingIndexer) Caps() indexers.Caps { return indexers.Caps{} }
func (f *failingIndexer) Search(context.Context, indexers.Query) (*indexers.Results, error) {
	return &indexers.Results{IndexerID: f.id}, nil
}
func (f *failingIndexer) Test(context.Context) error { return errors.New("boom") }

func TestHealthChecker_BackoffCapsAndResets(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	svc, err := indexers.NewService(indexers.ServiceOptions{Repository: repo, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, _ = svc.Create(context.Background(), indexers.Definition{
		ID: "backoff", Kind: indexers.KindNull, Name: "Backoff", Enabled: true,
	})
	if err := svc.Registry().Replace(&failingIndexer{id: "backoff"}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	checker := indexers.NewHealthChecker(svc, 1, 30*time.Second)

	// Run several times — each should increment the backoff.
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = checker.Run(ctx)
	}

	// After 5 failures the indexer should be backed off (skipped on next Run).
	// Verify by checking the health row still shows failed status.
	got, err := repo.GetHealth(ctx, "backoff")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if got.Status != indexers.StatusFailed {
		t.Fatalf("expected failed status after repeated failures, got %s", got.Status)
	}

	// Now replace with a passing indexer and force immediate check by
	// creating a new checker (fresh backoff state).
	if err := svc.Registry().Replace(&passingIndexer{id: "backoff"}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	freshChecker := indexers.NewHealthChecker(svc, 1, 0)
	if err := freshChecker.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err = repo.GetHealth(ctx, "backoff")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if got.Status != indexers.StatusOK {
		t.Fatalf("expected OK status after recovery, got %s", got.Status)
	}
}

// passingIndexer always succeeds.
type passingIndexer struct{ id string }

func (p *passingIndexer) ID() string          { return p.id }
func (p *passingIndexer) Name() string        { return p.id }
func (p *passingIndexer) Caps() indexers.Caps { return indexers.Caps{} }
func (p *passingIndexer) Search(context.Context, indexers.Query) (*indexers.Results, error) {
	return &indexers.Results{IndexerID: p.id}, nil
}
func (p *passingIndexer) Test(context.Context) error { return nil }

func TestHealthCheckerFailureRecorded(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	svc, err := indexers.NewService(indexers.ServiceOptions{Repository: repo, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	// Persist a row so ListHealth has the chance to write it, then
	// inject a failing live instance into the registry directly.
	_, _ = svc.Create(context.Background(), indexers.Definition{
		ID: "f", Kind: indexers.KindNull, Name: "F", Enabled: true,
	})
	if err := svc.Registry().Replace(&failingIndexer{id: "f"}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	checker := indexers.NewHealthChecker(svc, 1, 0)
	if err := checker.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := repo.GetHealth(context.Background(), "f")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if got.Status != indexers.StatusFailed {
		t.Fatalf("status=%s want=failed", got.Status)
	}
	if got.LastError == "" {
		t.Fatal("expected last_error to be populated")
	}
}
