package indexers_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/scheduler"
)

func TestRegistryAdmissionSkipsWithoutExpandingTargets(t *testing.T) {
	r := indexers.NewRegistry()
	denied := &fakeIndexer{id: "denied", name: "Denied"}
	allowed := &fakeIndexer{id: "allowed", name: "Allowed"}
	for _, ix := range []*fakeIndexer{denied, allowed} {
		if err := r.Register(ix); err != nil {
			t.Fatal(err)
		}
	}
	ctx := indexers.WithSearchAdmission(context.Background(), func(id string) bool {
		return id == "allowed"
	})
	agg := r.Search(ctx, indexers.Query{Term: "test"}, indexers.SearchOptions{})
	if denied.calls.Load() != 0 || allowed.calls.Load() != 1 {
		t.Fatalf("denied provider queried: denied=%d allowed=%d", denied.calls.Load(), allowed.calls.Load())
	}
	for _, d := range agg.Diagnostics.Indexers {
		if d.ID == "denied" && d.Status != "skipped" {
			t.Fatalf("quota denial must not trip circuit breaker: %+v", d)
		}
	}
	r.Search(ctx, indexers.Query{Term: "test"}, indexers.SearchOptions{IndexerIDs: []string{"denied"}})
	if allowed.calls.Load() != 1 || denied.calls.Load() != 0 {
		t.Fatal("all-denied explicit selection expanded to all providers")
	}
	r.Search(context.Background(), indexers.Query{Term: "test"}, indexers.SearchOptions{})
	if denied.calls.Load() != 1 || allowed.calls.Load() != 2 {
		t.Fatal("manual search inherited admission policy")
	}
}

func TestRegistryAdmissionAfterEligibilityAndBeforeAttempt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		caps      indexers.Caps
		q         indexers.Query
		cancelled bool
	}{
		{"unsupported ID", indexers.Caps{SupportedIDs: []string{"tvdb"}}, indexers.Query{IMDBID: "tt-test"}, false},
		{"wrong category", indexers.Caps{Categories: []indexers.Category{indexers.CategoryTV}}, indexers.Query{Term: "test", Categories: []indexers.Category{indexers.CategoryMovies}}, false},
		{"cancelled", indexers.Caps{}, indexers.Query{Term: "test"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := indexers.NewRegistry()
			p := &fakeIndexer{id: "provider", caps: tc.caps}
			if err := r.Register(p); err != nil {
				t.Fatal(err)
			}
			var reservations atomic.Int64
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			ctx = indexers.WithSearchAdmission(ctx, func(string) bool {
				reservations.Add(1)
				return true
			})
			r.Search(ctx, tc.q, indexers.SearchOptions{})
			if reservations.Load() != 0 || p.calls.Load() != 0 {
				t.Fatalf("ineligible query consumed quota: reservations=%d calls=%d", reservations.Load(), p.calls.Load())
			}
		})
	}
}

func TestRegistryConcurrentAdmissionCountsFailedQueries(t *testing.T) {
	r := indexers.NewRegistry()
	p := &fakeIndexer{id: "provider", searchErr: errors.New("provider failed")}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	quota := scheduler.NewQuotaTracker(3)
	ctx := indexers.WithSearchAdmission(context.Background(), quota.TryReserve)
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			r.Search(ctx, indexers.Query{Term: "test"}, indexers.SearchOptions{})
		})
	}
	wg.Wait()
	if p.calls.Load() != 3 || quota.Usage()["provider"] != 3 {
		t.Fatalf("failed concurrent attempts not capped: calls=%d usage=%v", p.calls.Load(), quota.Usage())
	}
}
