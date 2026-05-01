package indexers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Registry is a concurrency-safe map from indexer ID to live Indexer
// instances. It is a pure in-memory structure; persistence is owned
// by Repository and the two are stitched together by Service.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Indexer
}

// NewRegistry returns an empty Registry ready for use.
func NewRegistry() *Registry {
	return &Registry{items: make(map[string]Indexer)}
}

// Register inserts ix. It returns an error if an indexer with the same
// ID is already registered. Use Replace to swap an existing entry.
func (r *Registry) Register(ix Indexer) error {
	if ix == nil {
		return errors.New("indexers: register nil")
	}
	id := ix.ID()
	if id == "" {
		return errors.New("indexers: register with empty id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[id]; exists {
		return fmt.Errorf("indexers: %q already registered", id)
	}
	r.items[id] = ix
	return nil
}

// Replace inserts or overwrites the entry under ix.ID(). Useful when
// the operator updated configuration and we need to swap the live
// instance without dropping in-flight requests on other indexers.
func (r *Registry) Replace(ix Indexer) error {
	if ix == nil {
		return errors.New("indexers: replace with nil")
	}
	if ix.ID() == "" {
		return errors.New("indexers: replace with empty id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[ix.ID()] = ix
	return nil
}

// Remove deletes the entry by ID. Removing an unknown ID is a no-op.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
}

// Get returns the indexer registered under id. The bool is false when
// no such entry exists.
func (r *Registry) Get(id string) (Indexer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ix, ok := r.items[id]
	return ix, ok
}

// List returns a snapshot of every registered indexer, sorted by ID
// for deterministic output.
func (r *Registry) List() []Indexer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Indexer, 0, len(r.items))
	for _, ix := range r.items {
		out = append(out, ix)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Len returns how many indexers are currently registered.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// SearchOptions tunes a fan-out search.
type SearchOptions struct {
	// IndexerIDs restricts the fan-out to the given IDs. Empty means
	// "every registered indexer".
	IndexerIDs []string

	// PerIndexerTimeout bounds how long a single indexer may take
	// before its result is dropped and an error is recorded. Zero
	// means "use the parent context as-is".
	PerIndexerTimeout time.Duration

	// MaxParallel caps the number of concurrent in-flight searches.
	// Zero or negative means "no cap".
	MaxParallel int
}

// AggregatedResults is what Registry.Search returns: the merged result
// list plus a per-source error map for indexers that failed or timed
// out. Errors keyed by indexer ID never appear in Results — the two
// maps are disjoint.
type AggregatedResults struct {
	Results []Result          `json:"results"`
	Errors  map[string]string `json:"errors"`
}

// Search fans q out across the indexers selected by opts. It returns
// partial results: an indexer that errors or times out contributes an
// entry in Errors but does not abort the whole call.
func (r *Registry) Search(ctx context.Context, q Query, opts SearchOptions) AggregatedResults {
	targets := r.selectTargets(opts.IndexerIDs)
	if len(targets) == 0 {
		return AggregatedResults{Results: []Result{}, Errors: map[string]string{}}
	}

	limit := opts.MaxParallel
	if limit <= 0 || limit > len(targets) {
		limit = len(targets)
	}
	sem := make(chan struct{}, limit)

	type partial struct {
		id  string
		out *Results
		err error
	}
	ch := make(chan partial, len(targets))
	var wg sync.WaitGroup

	for _, ix := range targets {
		wg.Add(1)
		ix := ix
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := runOne(ctx, ix, q, opts.PerIndexerTimeout)
			ch <- partial{id: ix.ID(), out: res, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	agg := AggregatedResults{Results: []Result{}, Errors: map[string]string{}}
	for p := range ch {
		if p.err != nil {
			agg.Errors[p.id] = p.err.Error()
			continue
		}
		if p.out == nil {
			continue
		}
		agg.Results = append(agg.Results, p.out.Items...)
	}
	return agg
}

// runOne wraps Search with a per-indexer deadline so a slow source
// can't hold up the fan-out.
func runOne(ctx context.Context, ix Indexer, q Query, timeout time.Duration) (*Results, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return ix.Search(ctx, q)
}

func (r *Registry) selectTargets(ids []string) []Indexer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(ids) == 0 {
		out := make([]Indexer, 0, len(r.items))
		for _, ix := range r.items {
			out = append(out, ix)
		}
		return out
	}
	out := make([]Indexer, 0, len(ids))
	for _, id := range ids {
		if ix, ok := r.items[id]; ok {
			out = append(out, ix)
		}
	}
	return out
}
