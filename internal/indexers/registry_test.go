package indexers_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

// fakeIndexer is a minimal Indexer used by registry/search/health
// tests. Behaviour is configured via fields rather than ctor args so
// table tests can override only the bits they care about.
type fakeIndexer struct {
	id        string
	name      string
	caps      indexers.Caps
	delay     time.Duration
	results   []indexers.Result
	searchErr error
	testErr   error
	calls     atomic.Int32
}

func (f *fakeIndexer) ID() string          { return f.id }
func (f *fakeIndexer) Name() string        { return f.name }
func (f *fakeIndexer) Caps() indexers.Caps { return f.caps }
func (f *fakeIndexer) Search(ctx context.Context, _ indexers.Query) (*indexers.Results, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return &indexers.Results{IndexerID: f.id, Items: f.results, Total: len(f.results)}, nil
}
func (f *fakeIndexer) Test(ctx context.Context) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.testErr
}

func TestRegistryConcurrentRegisterGet(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			id := byte('a' + (i % 26))
			_ = r.Register(&fakeIndexer{id: string(id) + "-id", name: "n"})
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Get("a-id")
			_ = r.List()
		}()
	}
	wg.Wait()
	if r.Len() == 0 {
		t.Fatal("expected at least one registration")
	}
}

func TestRegistrySearchTimeoutGivesPartial(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	fast := &fakeIndexer{id: "fast", results: []indexers.Result{{IndexerID: "fast", Title: "ok", MagnetURI: "magnet:?xt=urn:btih:abc"}}}
	slow := &fakeIndexer{id: "slow", delay: 200 * time.Millisecond}
	if err := r.Register(fast); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(slow); err != nil {
		t.Fatal(err)
	}

	out := r.Search(context.Background(), indexers.Query{Term: "x"}, indexers.SearchOptions{
		PerIndexerTimeout: 25 * time.Millisecond,
	})
	if len(out.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(out.Results))
	}
	if _, ok := out.Errors["slow"]; !ok {
		t.Fatalf("want slow error, errors=%v", out.Errors)
	}
}

func TestRegistrySearchSelectsByID(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	a := &fakeIndexer{id: "a", results: []indexers.Result{{Title: "A", MagnetURI: "magnet:?xt=urn:btih:aaa"}}}
	b := &fakeIndexer{id: "b", results: []indexers.Result{{Title: "B", MagnetURI: "magnet:?xt=urn:btih:bbb"}}}
	_ = r.Register(a)
	_ = r.Register(b)
	out := r.Search(context.Background(), indexers.Query{}, indexers.SearchOptions{IndexerIDs: []string{"a"}})
	if len(out.Results) != 1 || out.Results[0].Title != "A" {
		t.Fatalf("unexpected results: %#v", out.Results)
	}
}

// --- p3: capability-based (category) indexer selection ---

func TestRegistrySearchSkipsIndexerByCategoryCaps(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	movies := &fakeIndexer{
		id:   "movies",
		name: "MoviesOnly",
		caps: indexers.Caps{Categories: []indexers.Category{indexers.CategoryMovies}},
		results: []indexers.Result{
			{IndexerID: "movies", Title: "M", MagnetURI: "magnet:?xt=urn:btih:aaa"},
		},
	}
	tv := &fakeIndexer{
		id:   "tv",
		name: "TVOnly",
		caps: indexers.Caps{Categories: []indexers.Category{indexers.CategoryTV}},
		results: []indexers.Result{
			{IndexerID: "tv", Title: "T", Category: []indexers.Category{indexers.CategoryTV},
				MagnetURI: "magnet:?xt=urn:btih:bbb"},
		},
	}
	_ = r.Register(movies)
	_ = r.Register(tv)

	q := indexers.Query{Term: "show", Categories: []indexers.Category{indexers.CategoryTV}}
	out := r.Search(context.Background(), q, indexers.SearchOptions{})

	if movies.calls.Load() != 0 {
		t.Errorf("movies-only indexer should be skipped for a TV search, got %d calls", movies.calls.Load())
	}
	if tv.calls.Load() != 1 {
		t.Errorf("tv indexer should be queried once, got %d", tv.calls.Load())
	}
	if _, ok := out.Errors["movies"]; ok {
		t.Error("a skipped indexer must not be reported as an error")
	}
	if out.Diagnostics == nil {
		t.Fatal("expected diagnostics")
	}
	var found bool
	for _, d := range out.Diagnostics.Indexers {
		if d.ID == "movies" {
			found = true
			if d.Status != "skipped" {
				t.Errorf("movies status = %q, want skipped", d.Status)
			}
		}
	}
	if !found {
		t.Error("expected a diagnostic entry for the skipped indexer")
	}
}

func TestRegistrySearchUnknownCategoryCapsNotSkipped(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	// No advertised categories → unknown caps → must not be skipped.
	any := &fakeIndexer{
		id:   "any",
		name: "AnyCat",
		results: []indexers.Result{
			{IndexerID: "any", Title: "X", Category: []indexers.Category{indexers.CategoryTV},
				MagnetURI: "magnet:?xt=urn:btih:ccc"},
		},
	}
	_ = r.Register(any)

	q := indexers.Query{Term: "show", Categories: []indexers.Category{indexers.CategoryTV}}
	out := r.Search(context.Background(), q, indexers.SearchOptions{})
	if any.calls.Load() != 1 {
		t.Errorf("indexer with unknown caps should be queried, got %d calls", any.calls.Load())
	}
	if len(out.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(out.Results))
	}
}

func TestRegistrySearchSurfacesIndexerError(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	bad := &fakeIndexer{id: "bad", searchErr: errors.New("upstream 500")}
	_ = r.Register(bad)
	out := r.Search(context.Background(), indexers.Query{}, indexers.SearchOptions{})
	if len(out.Results) != 0 {
		t.Fatalf("expected no results, got %d", len(out.Results))
	}
	if msg, ok := out.Errors["bad"]; !ok || msg == "" {
		t.Fatalf("missing error for bad indexer: %v", out.Errors)
	}
}

func TestRegistryReplaceAndRemove(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()
	first := &fakeIndexer{id: "x", name: "first"}
	second := &fakeIndexer{id: "x", name: "second"}
	if err := r.Register(first); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(second); err == nil {
		t.Fatal("expected duplicate register error")
	}
	if err := r.Replace(second); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, ok := r.Get("x")
	if !ok || got.Name() != "second" {
		t.Fatalf("Replace did not swap entry: got=%#v ok=%v", got, ok)
	}
	r.Remove("x")
	if _, ok := r.Get("x"); ok {
		t.Fatal("Remove did not delete")
	}
}

func TestRegistrySearchFiltersByCategory(t *testing.T) {
	t.Parallel()
	r := indexers.NewRegistry()

	seeders := func(n int) *int { return &n }
	_ = r.Register(&fakeIndexer{
		id:   "mixed",
		name: "mixed-indexer",
		results: []indexers.Result{
			{Title: "Avengers 2012 1080p", Category: []indexers.Category{2040}, Seeders: seeders(50), MagnetURI: "magnet:?xt=urn:btih:m1"},
			{Title: "Avengers S01E01", Category: []indexers.Category{5030}, Seeders: seeders(30), MagnetURI: "magnet:?xt=urn:btih:m2"},
			{Title: "Avengers OST", Category: []indexers.Category{3000}, Seeders: seeders(10), MagnetURI: "magnet:?xt=urn:btih:m3"},
			{Title: "Avengers Unknown", Category: nil, Seeders: seeders(5), MagnetURI: "magnet:?xt=urn:btih:m4"}, // no category → kept
		},
	})

	// Movie search: should drop TV and Audio results, keep movie + unknown
	out := r.Search(context.Background(), indexers.Query{
		Term:       "Avengers",
		Categories: []indexers.Category{2000},
	}, indexers.SearchOptions{})
	if len(out.Results) != 2 {
		t.Fatalf("movie filter: got %d results, want 2", len(out.Results))
	}
	for _, res := range out.Results {
		if res.Title == "Avengers S01E01" || res.Title == "Avengers OST" {
			t.Errorf("movie filter: unexpected result %q", res.Title)
		}
	}

	// TV search: should drop Movie and Audio results, keep TV + unknown
	out = r.Search(context.Background(), indexers.Query{
		Term:       "Avengers",
		Categories: []indexers.Category{5000, 5030},
	}, indexers.SearchOptions{})
	if len(out.Results) != 2 {
		t.Fatalf("tv filter: got %d results, want 2", len(out.Results))
	}
	for _, res := range out.Results {
		if res.Title == "Avengers 2012 1080p" || res.Title == "Avengers OST" {
			t.Errorf("tv filter: unexpected result %q", res.Title)
		}
	}

	// No category filter: should keep all 4
	out = r.Search(context.Background(), indexers.Query{
		Term: "Avengers",
	}, indexers.SearchOptions{})
	if len(out.Results) != 4 {
		t.Fatalf("no filter: got %d results, want 4", len(out.Results))
	}
}
