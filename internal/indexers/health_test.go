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
