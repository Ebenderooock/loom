package indexers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	// TimeoutOverrides supplies a per-indexer timeout keyed by indexer
	// ID. When an indexer has an entry here it takes precedence over
	// PerIndexerTimeout. Used to grant FlareSolverr-proxied indexers a
	// longer budget (a real Cloudflare solve can take tens of seconds)
	// while keeping direct indexers fail-fast.
	TimeoutOverrides map[string]time.Duration

	// MaxParallel caps the number of concurrent in-flight searches.
	// Zero or negative means "no cap".
	MaxParallel int
}

// timeoutFor returns the effective per-indexer timeout for the given
// indexer ID, preferring a TimeoutOverrides entry over the default.
func (o SearchOptions) timeoutFor(id string) time.Duration {
	if o.TimeoutOverrides != nil {
		if t, ok := o.TimeoutOverrides[id]; ok && t > 0 {
			return t
		}
	}
	return o.PerIndexerTimeout
}

// IndexerDiagnostic records timing and status for a single indexer's
// contribution to a fan-out search. Surfaced via the diagnostics
// field of the search response.
type IndexerDiagnostic struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"` // "ok", "error", "timeout"
	ResponseTimeMS int64  `json:"response_time_ms"`
	ResultCount    int    `json:"result_count"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

// SearchDiagnostics carries observability metadata for a search
// request — per-indexer breakdowns and overall timing.
type SearchDiagnostics struct {
	Indexers         []IndexerDiagnostic `json:"indexers"`
	TotalResults     int                 `json:"total_results"`
	SearchDurationMS int64               `json:"search_duration_ms"`
}

// AggregatedResults is what Registry.Search returns: the merged result
// list plus a per-source error map for indexers that failed or timed
// out. Errors keyed by indexer ID never appear in Results — the two
// maps are disjoint.
type AggregatedResults struct {
	Results     []Result          `json:"results"`
	Errors      map[string]string `json:"errors"`
	Diagnostics *SearchDiagnostics `json:"diagnostics,omitempty"`
}

// StreamEventType identifies the kind of SSE event emitted during a
// streaming search.
type StreamEventType string

const (
	EventSearchStart  StreamEventType = "search-start"
	EventIndexerStart StreamEventType = "indexer-start"
	EventIndexerResult StreamEventType = "indexer-result"
	EventIndexerError  StreamEventType = "indexer-error"
	EventDone          StreamEventType = "done"
)

// StreamEvent is a single SSE event emitted during a streaming search.
type StreamEvent struct {
	Type         StreamEventType `json:"type"`
	IndexerID    string          `json:"indexer_id,omitempty"`
	IndexerName  string          `json:"indexer_name,omitempty"`
	Results      []Result        `json:"results,omitempty"`
	ResultCount  int             `json:"result_count,omitempty"`
	ElapsedMS    int64           `json:"elapsed_ms,omitempty"`
	Error        string          `json:"error,omitempty"`
	Status       string          `json:"status,omitempty"`
	Indexers     []IndexerInfo   `json:"indexers,omitempty"`
	TotalResults int             `json:"total_results,omitempty"`
	TotalErrors  int             `json:"total_errors,omitempty"`
	SearchDurationMS int64       `json:"search_duration_ms,omitempty"`
}

// IndexerInfo is a lightweight summary of an indexer for the search-start event.
type IndexerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
		id         string
		name       string
		out        *Results
		err        error
		skipped    bool
		skipReason string
		elapsed    time.Duration
	}
	ch := make(chan partial, len(targets))
	var wg sync.WaitGroup
	searchStart := time.Now()

	for _, ix := range targets {
		// Skip indexers that can't handle the query's IDs when there's no
		// text fallback. Without this, indexers like TPB return generic
		// results when they receive ID params they don't understand.
		if q.Term == "" && queryHasIDs(q) && !indexerSupportsAnyQueryID(ix, q) {
			slog.Debug("registry: skipping indexer (no supported IDs, no text fallback)",
				"indexer", ix.Name(), "query_ids", queryIDSummary(q))
			ch <- partial{id: ix.ID(), name: ix.Name(), skipped: true,
				skipReason: "indexer does not support any of the query's ID types"}
			continue
		}

		// Skip indexers whose advertised categories cannot serve this
		// query (e.g. a movies-only indexer for a TV search). This avoids
		// wasted requests/timeouts and the false "indexer failed"
		// diagnostics they produce.
		if !indexerServesCategories(ix, q) {
			slog.Debug("registry: skipping indexer (categories not served)",
				"indexer", ix.Name(), "query_categories", queryCategorySummary(q))
			ch <- partial{id: ix.ID(), name: ix.Name(), skipped: true,
				skipReason: "indexer does not serve the requested categories"}
			continue
		}

		wg.Add(1)
		ix := ix
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			start := time.Now()
			res, err := runOne(ctx, ix, q, opts.timeoutFor(ix.ID()))
			ch <- partial{id: ix.ID(), name: ix.Name(), out: res, err: err, elapsed: time.Since(start)}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	agg := AggregatedResults{Results: []Result{}, Errors: map[string]string{}}
	diags := make([]IndexerDiagnostic, 0, len(targets))
	for p := range ch {
		d := IndexerDiagnostic{
			ID:             p.id,
			Name:           p.name,
			ResponseTimeMS: p.elapsed.Milliseconds(),
		}
		if p.skipped {
			d.Status = "skipped"
			d.ErrorMessage = p.skipReason
			if d.ErrorMessage == "" {
				d.ErrorMessage = "indexer skipped"
			}
		} else if p.err != nil {
			agg.Errors[p.id] = p.err.Error()
			d.ErrorMessage = p.err.Error()
			if errors.Is(p.err, context.DeadlineExceeded) {
				d.Status = "timeout"
			} else {
				d.Status = "error"
			}
		} else if p.out != nil {
			d.Status = "ok"
			d.ResultCount = len(p.out.Items)
			agg.Results = append(agg.Results, p.out.Items...)
		} else {
			d.Status = "ok"
			slog.Warn("indexer returned nil results without error", "indexer", p.id, "name", p.name)
		}
		diags = append(diags, d)
	}

	sort.Slice(diags, func(i, j int) bool { return diags[i].Name < diags[j].Name })

	// Validate: reject structurally invalid results (empty title, no
	// download path). Applied before category filter.
	agg.Results = validateResults(agg.Results)

	// Deduplicate: when the same release (same GUID) appears from
	// multiple indexers, keep only the first occurrence. Results
	// are already ordered by indexer completion time, which naturally
	// favours faster/higher-priority indexers.
	agg.Results = deduplicateResults(agg.Results)

	// Post-filter: reject results whose categories don't match the
	// requested family. Indexers (especially Cardigann scrapers) may
	// return cross-category results even when cat= is sent. This is
	// the same safety net Radarr/Sonarr apply client-side.
	if len(q.Categories) > 0 {
		preCount := len(agg.Results)
		agg.Results = filterByCategory(agg.Results, q.Categories)
		postCount := len(agg.Results)
		if preCount > 0 && postCount == 0 {
			slog.Warn("category filter removed all results",
				"query", q.Term,
				"requested_categories", q.Categories,
				"pre_filter_count", preCount,
			)
		} else if preCount != postCount {
			slog.Debug("category filter applied",
				"pre_filter_count", preCount,
				"post_filter_count", postCount,
				"requested_categories", q.Categories,
			)
		}
	}

	agg.Diagnostics = &SearchDiagnostics{
		Indexers:         diags,
		TotalResults:     len(agg.Results),
		SearchDurationMS: time.Since(searchStart).Milliseconds(),
	}
	return agg
}

// SearchStream fans q out like Search but emits incremental StreamEvent
// values on the events channel instead of collecting everything up-front.
// The channel is closed when the search completes or ctx is cancelled.
func (r *Registry) SearchStream(ctx context.Context, q Query, opts SearchOptions, events chan<- StreamEvent) {
	defer close(events)

	targets := r.selectTargets(opts.IndexerIDs)
	if len(targets) == 0 {
		select {
		case events <- StreamEvent{Type: EventDone}:
		case <-ctx.Done():
		}
		return
	}

	infos := make([]IndexerInfo, len(targets))
	for i, ix := range targets {
		infos[i] = IndexerInfo{ID: ix.ID(), Name: ix.Name()}
	}
	select {
	case events <- StreamEvent{Type: EventSearchStart, Indexers: infos}:
	case <-ctx.Done():
		return
	}

	limit := opts.MaxParallel
	if limit <= 0 || limit > len(targets) {
		limit = len(targets)
	}
	sem := make(chan struct{}, limit)
	searchStart := time.Now()

	var wg sync.WaitGroup
	var totalResults, totalErrors int32

	for _, ix := range targets {
		// Skip indexers that can't handle the query's IDs when there's no
		// text fallback (same check as Search).
		if q.Term == "" && queryHasIDs(q) && !indexerSupportsAnyQueryID(ix, q) {
			slog.Debug("registry: stream skipping indexer (no supported IDs, no text fallback)",
				"indexer", ix.Name(), "query_ids", queryIDSummary(q))
			select {
			case events <- StreamEvent{
				Type: EventIndexerError, IndexerID: ix.ID(), IndexerName: ix.Name(),
				Error: "skipped: indexer does not support any of the query's ID types", Status: "skipped",
			}:
			case <-ctx.Done():
			}
			continue
		}

		// Skip indexers whose categories cannot serve this query (same
		// check as Search).
		if !indexerServesCategories(ix, q) {
			slog.Debug("registry: stream skipping indexer (categories not served)",
				"indexer", ix.Name(), "query_categories", queryCategorySummary(q))
			select {
			case events <- StreamEvent{
				Type: EventIndexerError, IndexerID: ix.ID(), IndexerName: ix.Name(),
				Error: "skipped: indexer does not serve the requested categories", Status: "skipped",
			}:
			case <-ctx.Done():
			}
			continue
		}

		wg.Add(1)
		go func(ix Indexer) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			select {
			case events <- StreamEvent{Type: EventIndexerStart, IndexerID: ix.ID(), IndexerName: ix.Name()}:
			case <-ctx.Done():
				return
			}

			start := time.Now()
			res, err := runOne(ctx, ix, q, opts.timeoutFor(ix.ID()))
			elapsed := time.Since(start).Milliseconds()

			if err != nil {
				atomic.AddInt32(&totalErrors, 1)
				status := "error"
				if errors.Is(err, context.DeadlineExceeded) {
					status = "timeout"
				}
				select {
				case events <- StreamEvent{
					Type: EventIndexerError, IndexerID: ix.ID(), IndexerName: ix.Name(),
					Error: err.Error(), Status: status, ElapsedMS: elapsed,
				}:
				case <-ctx.Done():
				}
				return
			}

			items := res.Items
			items = validateResults(items)
			if len(q.Categories) > 0 {
				items = filterByCategory(items, q.Categories)
			}

			atomic.AddInt32(&totalResults, int32(len(items)))
			select {
			case events <- StreamEvent{
				Type: EventIndexerResult, IndexerID: ix.ID(), IndexerName: ix.Name(),
				Results: items, ResultCount: len(items), ElapsedMS: elapsed, Status: "ok",
			}:
			case <-ctx.Done():
			}
		}(ix)
	}

	wg.Wait()

	select {
	case events <- StreamEvent{
		Type:             EventDone,
		TotalResults:     int(atomic.LoadInt32(&totalResults)),
		TotalErrors:      int(atomic.LoadInt32(&totalErrors)),
		SearchDurationMS: time.Since(searchStart).Milliseconds(),
	}:
	case <-ctx.Done():
	}
}

// runOne wraps Search with a per-indexer deadline so a slow source
// can't hold up the fan-out.
func runOne(ctx context.Context, ix Indexer, q Query, timeout time.Duration) (*Results, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if dl, ok := ctx.Deadline(); ok {
		slog.Debug("runOne: context for indexer", "indexer", ix.Name(), "per_timeout", timeout.String(), "deadline_in", time.Until(dl).String())
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

// validateResults removes results that are structurally invalid —
// specifically those with an empty Title or no download path (neither
// Link nor MagnetURI). This mirrors Sonarr's IsValidRelease() check
// and catches upstream garbage before it enters scoring/streaming.
func validateResults(results []Result) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if strings.TrimSpace(r.Title) == "" {
			slog.Debug("validate: dropping result with empty title",
				"indexer", r.IndexerID, "guid", r.GUID)
			continue
		}
		if strings.TrimSpace(r.Link) == "" && strings.TrimSpace(r.MagnetURI) == "" && strings.TrimSpace(r.Infohash) == "" {
			slog.Debug("validate: dropping result with no download path",
				"indexer", r.IndexerID, "title", r.Title, "guid", r.GUID)
			continue
		}
		out = append(out, r)
	}
	return out
}

// filterByCategory keeps only results that share at least one
// top-level category family with the requested categories. Results
// with no categories at all are kept (we can't prove they're wrong).
func filterByCategory(results []Result, wanted []Category) []Result {
	families := make(map[Category]bool, len(wanted))
	for _, c := range wanted {
		families[c.Family()] = true
	}
	filtered := make([]Result, 0, len(results))
	for _, r := range results {
		if len(r.Category) == 0 {
			filtered = append(filtered, r)
			continue
		}
		matched := false
		for _, rc := range r.Category {
			if families[rc.Family()] {
				matched = true
				break
			}
		}
		if matched {
			filtered = append(filtered, r)
		} else {
			slog.Debug("category filter: dropping result",
				"indexer", r.IndexerID,
				"title", r.Title,
				"result_categories", r.Category,
				"wanted_families", wanted,
			)
		}
	}
	return filtered
}

// deduplicateResults removes duplicate releases across indexers.
// Duplicates are identified by GUID; the first occurrence wins (which
// naturally favours faster or higher-priority indexers since results
// are appended in completion order). Results without a GUID are always
// kept — they can't be matched.
func deduplicateResults(results []Result) []Result {
	seen := make(map[string]bool, len(results))
	out := make([]Result, 0, len(results))
	dupes := 0
	for _, r := range results {
		guid := strings.TrimSpace(r.GUID)
		if guid == "" {
			out = append(out, r)
			continue
		}
		if seen[guid] {
			dupes++
			continue
		}
		seen[guid] = true
		out = append(out, r)
	}
	if dupes > 0 {
		slog.Debug("dedup: removed duplicate results", "removed", dupes, "remaining", len(out))
	}
	return out
}

// queryHasIDs returns true when the query carries at least one external ID.
func queryHasIDs(q Query) bool {
	return q.IMDBID != "" || q.TVDBID != "" || q.TMDBID != ""
}

// indexerSupportsAnyQueryID checks whether the indexer advertises support
// for at least one of the ID types present in the query.
func indexerSupportsAnyQueryID(ix Indexer, q Query) bool {
	caps := ix.Caps()
	if len(caps.SupportedIDs) == 0 {
		return true // unknown caps → allow (don't break existing indexers)
	}
	supported := make(map[string]bool, len(caps.SupportedIDs))
	for _, id := range caps.SupportedIDs {
		supported[strings.ToLower(id)] = true
	}
	if q.IMDBID != "" && (supported["imdbid"] || supported["imdb"]) {
		return true
	}
	if q.TVDBID != "" && (supported["tvdbid"] || supported["tvdb"]) {
		return true
	}
	if q.TMDBID != "" && (supported["tmdbid"] || supported["tmdb"]) {
		return true
	}
	return false
}

// indexerServesCategories reports whether the indexer can serve the
// query's requested categories. It compares at the top-level family
// granularity (e.g. 2040 → 2000) so a movies-only indexer is skipped
// for a TV search and vice-versa. An indexer that advertises no
// categories is allowed (unknown caps → don't break it); a query that
// requests no categories matches every indexer.
func indexerServesCategories(ix Indexer, q Query) bool {
	if len(q.Categories) == 0 {
		return true
	}
	caps := ix.Caps()
	if len(caps.Categories) == 0 {
		return true // unknown caps → allow
	}
	families := make(map[Category]bool, len(caps.Categories))
	for _, c := range caps.Categories {
		families[c.Family()] = true
	}
	for _, qc := range q.Categories {
		if families[qc.Family()] {
			return true
		}
	}
	return false
}

// queryCategorySummary returns a human-readable summary of requested
// category families for diagnostics.
func queryCategorySummary(q Query) string {
	if len(q.Categories) == 0 {
		return ""
	}
	parts := make([]string, 0, len(q.Categories))
	seen := make(map[Category]bool, len(q.Categories))
	for _, c := range q.Categories {
		f := c.Family()
		if seen[f] {
			continue
		}
		seen[f] = true
		parts = append(parts, strconv.Itoa(int(f)))
	}
	return strings.Join(parts, ",")
}

// queryIDSummary returns a human-readable summary of which IDs are set.
func queryIDSummary(q Query) string {
	var parts []string
	if q.IMDBID != "" {
		parts = append(parts, "imdb="+q.IMDBID)
	}
	if q.TVDBID != "" {
		parts = append(parts, "tvdb="+q.TVDBID)
	}
	if q.TMDBID != "" {
		parts = append(parts, "tmdb="+q.TMDBID)
	}
	return strings.Join(parts, ",")
}
