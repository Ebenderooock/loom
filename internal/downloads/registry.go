package downloads

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Registry is a concurrency-safe map from client ID to live
// DownloadClient. It is a pure in-memory structure; persistence lives
// in Repository and the two are stitched together by Service.
type Registry struct {
	mu    sync.RWMutex
	items map[string]DownloadClient
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{items: make(map[string]DownloadClient)}
}

// Register inserts c. Duplicate IDs are an error.
func (r *Registry) Register(c DownloadClient) error {
	if c == nil {
		return errors.New("downloads: register nil")
	}
	id := c.ID()
	if id == "" {
		return errors.New("downloads: register with empty id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[id]; exists {
		return fmt.Errorf("downloads: %q already registered", id)
	}
	r.items[id] = c
	return nil
}

// Replace inserts or overwrites the entry under c.ID(). Used after
// CRUD updates so live calls against other clients keep flowing.
func (r *Registry) Replace(c DownloadClient) error {
	if c == nil {
		return errors.New("downloads: replace nil")
	}
	if c.ID() == "" {
		return errors.New("downloads: replace empty id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[c.ID()] = c
	return nil
}

// Remove deletes the entry by ID; missing IDs are a no-op.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
}

// Get returns the client registered under id.
func (r *Registry) Get(id string) (DownloadClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.items[id]
	return c, ok
}

// List returns a snapshot of every registered client, sorted by ID.
func (r *Registry) List() []DownloadClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]DownloadClient, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Len returns the number of registered clients.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// FanOutOptions tunes a multi-client fan-out. Zero values disable the
// per-client timeout and the parallelism cap.
type FanOutOptions struct {
	// ClientIDs restricts the fan-out. Empty = every registered
	// client.
	ClientIDs []string

	// PerClientTimeout bounds a single client call.
	PerClientTimeout time.Duration

	// MaxParallel caps concurrent in-flight calls. <=0 = no cap.
	MaxParallel int
}

// AggregatedStatus is the merged result of a Status fan-out.
type AggregatedStatus struct {
	Items  []Item            `json:"items"`
	Errors map[string]string `json:"errors"`
}

// AggregatedFreeSpace is the merged result of a FreeSpace fan-out.
type AggregatedFreeSpace struct {
	BytesByClient map[string]int64  `json:"bytes_by_client"`
	Errors        map[string]string `json:"errors"`
}

// AggregatedTest is the merged result of a Test fan-out.
type AggregatedTest struct {
	OK     []string          `json:"ok"`
	Errors map[string]string `json:"errors"`
}

// Status fans Status() across the selected clients.
func (r *Registry) Status(ctx context.Context, ids []string, opts FanOutOptions) AggregatedStatus {
	targets := r.selectTargets(opts.ClientIDs)
	out := AggregatedStatus{Items: []Item{}, Errors: map[string]string{}}
	if len(targets) == 0 {
		return out
	}
	type partial struct {
		id    string
		items []Item
		err   error
	}
	ch := make(chan partial, len(targets))
	r.runFanOut(ctx, targets, opts, func(c DownloadClient, cctx context.Context) {
		items, err := c.Status(cctx, ids...)
		ch <- partial{id: c.ID(), items: items, err: err}
	}, len(targets))
	close(ch)
	for p := range ch {
		if p.err != nil {
			out.Errors[p.id] = p.err.Error()
			continue
		}
		out.Items = append(out.Items, p.items...)
	}
	return out
}

// FreeSpace fans FreeSpace() across the selected clients.
func (r *Registry) FreeSpace(ctx context.Context, opts FanOutOptions) AggregatedFreeSpace {
	targets := r.selectTargets(opts.ClientIDs)
	out := AggregatedFreeSpace{BytesByClient: map[string]int64{}, Errors: map[string]string{}}
	if len(targets) == 0 {
		return out
	}
	type partial struct {
		id    string
		bytes int64
		err   error
	}
	ch := make(chan partial, len(targets))
	r.runFanOut(ctx, targets, opts, func(c DownloadClient, cctx context.Context) {
		b, err := c.FreeSpace(cctx)
		ch <- partial{id: c.ID(), bytes: b, err: err}
	}, len(targets))
	close(ch)
	for p := range ch {
		if p.err != nil {
			out.Errors[p.id] = p.err.Error()
			continue
		}
		out.BytesByClient[p.id] = p.bytes
	}
	return out
}

// Test fans Test() across the selected clients.
func (r *Registry) Test(ctx context.Context, opts FanOutOptions) AggregatedTest {
	targets := r.selectTargets(opts.ClientIDs)
	out := AggregatedTest{OK: []string{}, Errors: map[string]string{}}
	if len(targets) == 0 {
		return out
	}
	type partial struct {
		id  string
		err error
	}
	ch := make(chan partial, len(targets))
	r.runFanOut(ctx, targets, opts, func(c DownloadClient, cctx context.Context) {
		err := c.Test(cctx)
		ch <- partial{id: c.ID(), err: err}
	}, len(targets))
	close(ch)
	for p := range ch {
		if p.err != nil {
			out.Errors[p.id] = p.err.Error()
			continue
		}
		out.OK = append(out.OK, p.id)
	}
	sort.Strings(out.OK)
	return out
}

// runFanOut launches do(c) for every c in targets, capping concurrency
// at opts.MaxParallel and applying opts.PerClientTimeout per call.
// The caller is responsible for sizing/closing its result channel.
// expected is unused beyond clarifying intent for the reader.
func (r *Registry) runFanOut(parent context.Context, targets []DownloadClient, opts FanOutOptions, do func(DownloadClient, context.Context), expected int) {
	_ = expected
	limit := opts.MaxParallel
	if limit <= 0 || limit > len(targets) {
		limit = len(targets)
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, c := range targets {
		c := c
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			cctx := parent
			if opts.PerClientTimeout > 0 {
				var cancel context.CancelFunc
				cctx, cancel = context.WithTimeout(parent, opts.PerClientTimeout)
				defer cancel()
			}
			do(c, cctx)
		}()
	}
	wg.Wait()
}

func (r *Registry) selectTargets(ids []string) []DownloadClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(ids) == 0 {
		out := make([]DownloadClient, 0, len(r.items))
		for _, c := range r.items {
			out = append(out, c)
		}
		return out
	}
	out := make([]DownloadClient, 0, len(ids))
	for _, id := range ids {
		if c, ok := r.items[id]; ok {
			out = append(out, c)
		}
	}
	return out
}
