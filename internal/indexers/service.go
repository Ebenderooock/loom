package indexers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/indexers/throttle"
)

// Clock is the small time abstraction the package uses so tests can
// pin "now". Production callers pass SystemClock{}.
type Clock interface {
	Now() time.Time
}

// SystemClock is the wall-clock implementation of Clock.
type SystemClock struct{}

// Now returns the current local time.
func (SystemClock) Now() time.Time { return time.Now() }

// ServiceOptions wires Service dependencies.
type ServiceOptions struct {
	Repository         Repository
	Registry           *Registry
	Logger             *slog.Logger
	Clock              Clock
	SearchTimeout      time.Duration
	MaxParallel        int
	HealthCheckTimeout time.Duration
	// RouteExtensions are additional sub-mounters that Mount calls
	// inside the same auth-protected /api/v1 group as the indexer
	// routes. The proxies handler is wired here so cmd/loom can
	// attach the proxy CRUD endpoints without editing server.go.
	RouteExtensions []RouteMounter

	// DefinitionLister, when non-nil, provides the list of available
	// Cardigann definitions for the catalogue API endpoint.
	DefinitionLister DefinitionLister

	// SearchHealthTracker, when non-nil, is used for search metrics.
	// If nil, NewService creates one automatically.
	SearchHealthTracker *SearchHealthTracker

	// IndexerAvailability, when non-nil, tracks per-indexer circuit
	// breaker state. If nil, NewService creates one automatically.
	IndexerAvailability *IndexerAvailability

	// QueryLog, when non-nil, enables persistent per-query logging.
	QueryLog *QueryLog

	// AuditLog, when non-nil, records notable indexer events.
	AuditLog *auditlog.Logger
}

// RouteMounter mounts additional routes onto the Service router. The
// chi.Router argument is already inside the project's /api/v1 group;
// implementations should call r.Route("/api/v1/<thing>", ...) just
// like Service.Mount does.
type RouteMounter func(chi.Router)

// Service is the orchestrator that the HTTP layer depends on. It owns
// the lifecycle that links a persisted Definition to a live Indexer
// in the Registry.
type Service struct {
	repo               Repository
	registry           *Registry
	logger             *slog.Logger
	clock              Clock
	searchTimeout      time.Duration
	maxParallel        int
	healthCheckTimeout time.Duration
	routeExtensions      []RouteMounter
	definitionLister     DefinitionLister
	searchHealthTracker  *SearchHealthTracker
	indexerAvailability  *IndexerAvailability
	queryLog             *QueryLog
	auditLog             *auditlog.Logger

	mu sync.Mutex // serialises CRUD against the registry
}

// NewService validates opts and returns a wired Service.
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Repository == nil {
		return nil, errors.New("indexers: repository must not be nil")
	}
	if opts.Registry == nil {
		opts.Registry = NewRegistry()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.SearchTimeout <= 0 {
		opts.SearchTimeout = 120 * time.Second
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 8
	}
	if opts.HealthCheckTimeout <= 0 {
		opts.HealthCheckTimeout = 120 * time.Second
	}
	sht := opts.SearchHealthTracker
	if sht == nil {
		sht = NewSearchHealthTracker(opts.Registry)
	}
	avail := opts.IndexerAvailability
	if avail == nil {
		avail = NewIndexerAvailability(opts.Clock)
	}
	return &Service{
		repo:                opts.Repository,
		registry:            opts.Registry,
		logger:              opts.Logger.With("module", "indexers"),
		clock:               opts.Clock,
		searchTimeout:       opts.SearchTimeout,
		maxParallel:         opts.MaxParallel,
		healthCheckTimeout:  opts.HealthCheckTimeout,
		routeExtensions:     opts.RouteExtensions,
		definitionLister:    opts.DefinitionLister,
		searchHealthTracker: sht,
		indexerAvailability: avail,
		queryLog:            opts.QueryLog,
		auditLog:            opts.AuditLog,
	}, nil
}

// Repository returns the underlying persistence layer.
func (s *Service) Repository() Repository { return s.repo }

// Registry returns the underlying live-instance registry.
func (s *Service) Registry() *Registry { return s.registry }

// Availability returns the circuit-breaker tracker so callers can
// inspect per-indexer cooldown state.
func (s *Service) Availability() *IndexerAvailability { return s.indexerAvailability }

// resolveIndexerID maps a human-readable indexer name back to its
// registry ID by scanning the live instances.
func (s *Service) resolveIndexerID(name string) string {
	for _, ix := range s.registry.List() {
		if ix.Name() == name {
			return ix.ID()
		}
	}
	return ""
}

// HydrateAll reads every persisted indexer and registers a live
// instance for each enabled row. Called once at startup. Indexers
// that fail to build are logged and skipped so a single broken
// definition never blocks the boot.
func (s *Service) HydrateAll(ctx context.Context) error {
	defs, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		if err := s.hydrateOne(ctx, def); err != nil {
			s.logger.Warn("hydrate indexer failed",
				"id", def.ID, "kind", def.Kind, "err", err)
		}
	}
	s.logger.Info("indexers hydrated",
		"registered", s.registry.Len(), "definitions", len(defs))
	return nil
}

func (s *Service) hydrateOne(ctx context.Context, def Definition) error {
	ix, err := build(ctx, def)
	if err != nil {
		return err
	}
	return s.registry.Replace(ix)
}

// Create inserts a new persisted indexer and registers a live
// instance if it is enabled.
func (s *Service) Create(ctx context.Context, def Definition) (Definition, error) {
	if def.ID == "" {
		return Definition{}, errors.New("indexers: id required")
	}
	if def.Kind == "" {
		return Definition{}, errors.New("indexers: kind required")
	}
	if _, err := LookupKind(def.Kind); err != nil {
		return Definition{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	saved, err := s.repo.Create(ctx, def)
	if err != nil {
		return Definition{}, err
	}
	if saved.Enabled {
		if err := s.hydrateOne(ctx, saved); err != nil {
			s.logger.Warn("create: hydrate failed", "id", saved.ID, "err", err)
		}
	}
	// Seed a "unknown" health row so list endpoints have something
	// to return until the first scheduled check fires.
	_ = s.repo.UpsertHealth(ctx, Health{
		IndexerID:     saved.ID,
		Status:        StatusUnknown,
		LastCheckedAt: s.clock.Now(),
	})

	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "indexer",
		EventType:  "indexer.created",
		Message:    fmt.Sprintf("Indexer %q created", saved.Name),
		EntityType: auditlog.StrPtr("indexer"),
		EntityID:   auditlog.StrPtr(saved.ID),
		EntityName: auditlog.StrPtr(saved.Name),
		Level:      "info",
		Source:     auditlog.StrPtr("user"),
	})

	return saved, nil
}

// Get returns the persisted Definition for id.
func (s *Service) Get(ctx context.Context, id string) (Definition, error) {
	return s.repo.Get(ctx, id)
}

// List returns every persisted Definition with health and rate-limit
// attached.
func (s *Service) List(ctx context.Context) ([]DefinitionWithHealth, error) {
	defs, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	healths, err := s.repo.ListHealth(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DefinitionWithHealth, 0, len(defs))
	for _, d := range defs {
		dh := DefinitionWithHealth{Definition: d}
		if h, ok := healths[d.ID]; ok {
			dh.Health = &h
		}
		// Best-effort: a missing rate_limit row is fine — the response
		// just shows the package defaults via newRateLimitView.
		if cfg, gerr := s.repo.GetRateLimit(ctx, d.ID); gerr == nil {
			dh.RateLimit = newRateLimitView(cfg)
		} else {
			dh.RateLimit = newRateLimitView(throttle.Config{MaxRetries: -1})
		}
		out = append(out, dh)
	}
	return out, nil
}

// GetWithHealth is the single-row sibling of List: returns the
// definition, health, and resolved rate-limit view in one call.
func (s *Service) GetWithHealth(ctx context.Context, id string) (DefinitionWithHealth, error) {
	def, err := s.repo.Get(ctx, id)
	if err != nil {
		return DefinitionWithHealth{}, err
	}
	dh := DefinitionWithHealth{Definition: def}
	if h, herr := s.repo.GetHealth(ctx, id); herr == nil {
		dh.Health = &h
	}
	if cfg, gerr := s.repo.GetRateLimit(ctx, id); gerr == nil {
		dh.RateLimit = newRateLimitView(cfg)
	} else {
		dh.RateLimit = newRateLimitView(throttle.Config{MaxRetries: -1})
	}
	return dh, nil
}

// DefinitionWithHealth is the API-facing pair returned by List/Get.
// RateLimit is attached when the indexer has any persisted rate-limit
// row; nil means the runtime is using package defaults (which are
// documented in docs/indexers-rate-limits.md). We expose the resolved
// values here too — `effective_*` — so the UI doesn't have to repeat
// the defaulting logic on the client.
type DefinitionWithHealth struct {
	Definition
	Health    *Health         `json:"health,omitempty"`
	RateLimit *RateLimitView  `json:"rate_limit,omitempty"`
}

// RateLimitView is the JSON-friendly mirror of throttle.Config plus
// the fully-resolved values that the transport will actually use.
// NULLable database columns surface as nil pointers so operators can
// see "this field is unset" distinctly from "this field is zero".
type RateLimitView struct {
	PerMinute          *int `json:"per_minute,omitempty"`
	Burst              *int `json:"burst,omitempty"`
	MaxRetries         *int `json:"max_retries,omitempty"`
	EffectivePerMinute int  `json:"effective_per_minute"`
	EffectiveBurst     int  `json:"effective_burst"`
	EffectiveMaxRetries int `json:"effective_max_retries"`
}

func newRateLimitView(cfg throttle.Config) *RateLimitView {
	resolved := throttle.Resolve(cfg)
	v := &RateLimitView{
		EffectivePerMinute:  resolved.PerMinute,
		EffectiveBurst:      resolved.Burst,
		EffectiveMaxRetries: resolved.MaxRetries,
	}
	if cfg.PerMinute > 0 {
		p := cfg.PerMinute
		v.PerMinute = &p
	}
	if cfg.Burst > 0 {
		b := cfg.Burst
		v.Burst = &b
	}
	if cfg.MaxRetries >= 0 {
		m := cfg.MaxRetries
		v.MaxRetries = &m
	}
	return v
}

// RateLimitFor implements RateLimitProvider. The transport layer calls
// this every time it builds a RoundTripper for an indexer; returning
// (cfg, true) means "use these explicit values", (zero, false) means
// "fall back to throttle defaults". A bare repository error is logged
// and treated as "unset" — we'd rather lose a custom limit than fail
// to build a transport at startup.
func (s *Service) RateLimitFor(indexerID string) (throttle.Config, bool) {
	cfg, err := s.repo.GetRateLimit(context.Background(), indexerID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			s.logger.Warn("rate-limit lookup failed",
				"id", indexerID, "err", err)
		}
		return throttle.Config{}, false
	}
	if cfg.PerMinute == 0 && cfg.Burst == 0 && cfg.MaxRetries < 0 {
		return throttle.Config{}, false
	}
	return cfg, true
}

// SetRateLimit persists per-indexer rate-limit dials and rebuilds the
// live instance so the new transport composition takes effect on the
// next request. Pass throttle.Config{} (all zeros, MaxRetries=0 OK)
// with values you want stored; use throttle.Defaults() to stamp the
// package defaults explicitly.
func (s *Service) SetRateLimit(ctx context.Context, id string, cfg throttle.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.repo.SetRateLimit(ctx, id, cfg); err != nil {
		return err
	}
	// Rehydrate so the transport picks up the new config. We tolerate
	// a hydrate failure here (e.g. an unknown kind) the same way the
	// other write paths do — log and continue.
	def, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil
	}
	if def.Enabled {
		if herr := s.hydrateOne(ctx, def); herr != nil {
			s.logger.Warn("rate-limit: hydrate failed", "id", id, "err", herr)
		}
	}
	return nil
}

// GetRateLimit returns the persisted Config for id (zero values where
// columns are NULL). Use throttle.Resolve to apply defaults.
func (s *Service) GetRateLimit(ctx context.Context, id string) (throttle.Config, error) {
	return s.repo.GetRateLimit(ctx, id)
}

// Replace overwrites the persisted row and the live instance.
func (s *Service) Replace(ctx context.Context, def Definition) (Definition, error) {
	if _, err := LookupKind(def.Kind); err != nil {
		return Definition{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	saved, err := s.repo.Replace(ctx, def)
	if err != nil {
		return Definition{}, err
	}
	if saved.Enabled {
		if err := s.hydrateOne(ctx, saved); err != nil {
			s.logger.Warn("replace: hydrate failed", "id", saved.ID, "err", err)
		}
	} else {
		s.registry.Remove(saved.ID)
	}

	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "indexer",
		EventType:  "indexer.updated",
		Message:    fmt.Sprintf("Indexer %q updated (replace)", saved.Name),
		EntityType: auditlog.StrPtr("indexer"),
		EntityID:   auditlog.StrPtr(saved.ID),
		EntityName: auditlog.StrPtr(saved.Name),
		Level:      "info",
		Source:     auditlog.StrPtr("user"),
	})

	return saved, nil
}

// Patch applies a partial update.
func (s *Service) Patch(ctx context.Context, p Patch) (Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	saved, err := s.repo.Patch(ctx, p)
	if err != nil {
		return Definition{}, err
	}
	if saved.Enabled {
		if err := s.hydrateOne(ctx, saved); err != nil {
			s.logger.Warn("patch: hydrate failed", "id", saved.ID, "err", err)
		}
	} else {
		s.registry.Remove(saved.ID)
	}

	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "indexer",
		EventType:  "indexer.updated",
		Message:    fmt.Sprintf("Indexer %q updated (patch)", saved.Name),
		EntityType: auditlog.StrPtr("indexer"),
		EntityID:   auditlog.StrPtr(saved.ID),
		EntityName: auditlog.StrPtr(saved.Name),
		Level:      "info",
		Source:     auditlog.StrPtr("user"),
	})

	return saved, nil
}

// Delete removes both the live instance and the persisted row.
func (s *Service) Delete(ctx context.Context, id string) error {
	// Capture name before removal for the audit log.
	var name string
	if def, err := s.repo.Get(ctx, id); err == nil {
		name = def.Name
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry.Remove(id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "indexer",
		EventType:  "indexer.deleted",
		Message:    fmt.Sprintf("Indexer %q deleted", name),
		EntityType: auditlog.StrPtr("indexer"),
		EntityID:   auditlog.StrPtr(id),
		EntityName: auditlog.StrPtr(name),
		Level:      "info",
		Source:     auditlog.StrPtr("user"),
	})
	return nil
}

// TestOne runs a single connectivity check, persists the outcome, and
// returns it. Used by POST /indexers/{id}/test.
func (s *Service) TestOne(ctx context.Context, id string) (Health, error) {
	ix, ok := s.registry.Get(id)
	if !ok {
		return Health{}, ErrNotFound
	}
	timeout := s.healthCheckTimeout
	checkCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	started := s.clock.Now()
	err := ix.Test(checkCtx)
	finished := s.clock.Now()

	h := Health{
		IndexerID:     id,
		LastCheckedAt: finished,
		Latency:       finished.Sub(started),
	}
	h.LatencyMS = h.Latency.Milliseconds()
	if err != nil {
		switch {
		case IsTimeoutErr(err):
			h.Status = StatusDegraded
			h.LastError = fmt.Sprintf("indexer %q test timed out: %s", id, err.Error())
		case IsRateLimitErr(err):
			h.Status = StatusDegraded
			h.LastError = fmt.Sprintf("indexer %q test rate limited: %s", id, err.Error())
		default:
			h.Status = StatusFailed
			h.LastError = fmt.Sprintf("indexer %q test failed: %s", id, err.Error())
		}
	} else {
		h.Status = StatusOK
		t := finished
		h.LastSuccessAt = &t
	}
	if perr := s.repo.UpsertHealth(ctx, h); perr != nil {
		s.logger.Warn("persist health failed", "id", id, "err", perr)
	}

	status := "passed"
	level := "info"
	if err != nil {
		if h.Status == StatusDegraded {
			status = "degraded"
		} else {
			status = "failed"
		}
		level = "warn"
	}
	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "indexer",
		EventType:  "indexer.test",
		Message:    fmt.Sprintf("Indexer test %s: %s", status, ix.Name()),
		Detail:     auditlog.StrPtr(h.LastError),
		EntityType: auditlog.StrPtr("indexer"),
		EntityID:   auditlog.StrPtr(id),
		EntityName: auditlog.StrPtr(ix.Name()),
		Level:      level,
		Source:     auditlog.StrPtr("user"),
	})

	return h, err
}

// Search fans an aggregated search out across the registered
// indexers, applying the configured timeout/parallelism caps.
func (s *Service) Search(ctx context.Context, q Query, ids []string, perTimeout time.Duration) AggregatedResults {
	if perTimeout <= 0 {
		perTimeout = s.searchTimeout
	}

	// Log query start.
	var queryLogID string
	if s.queryLog != nil {
		queryLogID = NewQueryID()
		if err := s.queryLog.StartQuery(ctx, queryLogID, q.Term, "search", "", ""); err != nil {
			s.logger.Warn("query log: start failed", "err", err)
		}
	}

	// Filter out indexers that are in a circuit-breaker cooldown.
	availableIDs, skippedIDs := s.indexerAvailability.FilterAvailable(ids)
	if len(skippedIDs) > 0 {
		s.logger.Info("search: skipping indexers in cooldown",
			"skipped", skippedIDs, "available", len(availableIDs))
	}

	agg := s.registry.Search(ctx, q, SearchOptions{
		IndexerIDs:        availableIDs,
		PerIndexerTimeout: perTimeout,
		MaxParallel:       s.maxParallel,
	})

	// Record availability: clear on success, record failure on error.
	if agg.Diagnostics != nil {
		for _, d := range agg.Diagnostics.Indexers {
			indexerID := s.resolveIndexerID(d.Name)
			if indexerID == "" {
				continue
			}
			if d.Status == "error" || d.Status == "timeout" {
				s.indexerAvailability.RecordFailure(indexerID)
			} else {
				s.indexerAvailability.RecordSuccess(indexerID)
			}
		}
	}

	// Add skipped indexers to diagnostics so callers see them.
	if len(skippedIDs) > 0 && agg.Diagnostics != nil {
		for _, id := range skippedIDs {
			name := id
			if ix, ok := s.registry.Get(id); ok {
				name = ix.Name()
			}
			agg.Diagnostics.Indexers = append(agg.Diagnostics.Indexers, IndexerDiagnostic{
				ID:           id,
				Name:         name,
				Status:       "skipped",
				ErrorMessage: "circuit breaker cooldown",
			})
		}
	}

	// Record search metrics and query log entries for each indexer.
	if agg.Diagnostics != nil {
		for _, d := range agg.Diagnostics.Indexers {
			indexerID := d.ID
			if indexerID == "" {
				indexerID = s.resolveIndexerID(d.Name) // fallback for backwards compat
			}
			if indexerID == "" {
				continue
			}
			dur := time.Duration(d.ResponseTimeMS) * time.Millisecond
			var searchErr error
			if d.Status == "error" || d.Status == "timeout" {
				searchErr = errors.New(d.ErrorMessage)
			}
			if s.searchHealthTracker != nil {
				s.searchHealthTracker.RecordSearch(indexerID, dur, d.ResultCount, searchErr)
			}

			// Per-indexer query log entry.
			if s.queryLog != nil && queryLogID != "" {
				iqID := NewQueryID()
				if err := s.queryLog.StartIndexerQuery(ctx, iqID, queryLogID, indexerID, d.Name); err != nil {
					s.logger.Warn("query log: start indexer failed", "err", err)
				}
				if err := s.queryLog.FinishIndexerQuery(ctx, iqID, d.ResultCount, d.ResponseTimeMS, searchErr); err != nil {
					s.logger.Warn("query log: finish indexer failed", "err", err)
				}
			}
		}
	}

	// Finish query log.
	if s.queryLog != nil && queryLogID != "" {
		if err := s.queryLog.FinishQuery(ctx, queryLogID, len(agg.Results), nil); err != nil {
			s.logger.Warn("query log: finish failed", "err", err)
		}
	}

	indexerCount := 0
	failedCount := 0
	if agg.Diagnostics != nil {
		indexerCount = len(agg.Diagnostics.Indexers)
		for _, d := range agg.Diagnostics.Indexers {
			if d.Status == "error" || d.Status == "timeout" {
				failedCount++
			}
		}
	}
	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "search",
		EventType:  "search.completed",
		Message:    fmt.Sprintf("Search %q returned %d results from %d indexers", q.Term, len(agg.Results), indexerCount),
		Detail:     auditlog.DetailJSON(map[string]any{"query_log_id": queryLogID, "query": q.Term, "query_type": "search", "total_results": len(agg.Results), "indexer_count": indexerCount, "failed_indexer_count": failedCount}),
		EntityType: auditlog.StrPtr("search"),
		EntityID:   auditlog.StrPtr(queryLogID),
		Level:      "info",
		Source:     auditlog.StrPtr("system"),
	})

	return agg
}

// SearchStream fans a streaming search out across the registered
// indexers, forwarding incremental events to the caller's channel.
func (s *Service) SearchStream(ctx context.Context, q Query, ids []string, perTimeout time.Duration, events chan<- StreamEvent) {
	if perTimeout <= 0 {
		perTimeout = s.searchTimeout
	}

	var queryLogID string
	if s.queryLog != nil {
		queryLogID = NewQueryID()
		if err := s.queryLog.StartQuery(ctx, queryLogID, q.Term, "search-stream", "", ""); err != nil {
			s.logger.Warn("query log: start failed", "err", err)
		}
	}

	// Filter out indexers in circuit-breaker cooldown.
	availableIDs, skippedIDs := s.indexerAvailability.FilterAvailable(ids)
	if len(skippedIDs) > 0 {
		s.logger.Info("search-stream: skipping indexers in cooldown",
			"skipped", skippedIDs, "available", len(availableIDs))
	}

	internal := make(chan StreamEvent, len(availableIDs)+16)
	go s.registry.SearchStream(ctx, q, SearchOptions{
		IndexerIDs:        availableIDs,
		PerIndexerTimeout: perTimeout,
		MaxParallel:       s.maxParallel,
	}, internal)

	var totalResults int
	for evt := range internal {
		if evt.Type == EventIndexerResult || evt.Type == EventIndexerError {
			indexerID := evt.IndexerID
			dur := time.Duration(evt.ElapsedMS) * time.Millisecond
			var searchErr error
			if evt.Type == EventIndexerError {
				searchErr = errors.New(evt.Error)
				s.indexerAvailability.RecordFailure(indexerID)
			} else {
				s.indexerAvailability.RecordSuccess(indexerID)
			}
			if s.searchHealthTracker != nil {
				s.searchHealthTracker.RecordSearch(indexerID, dur, evt.ResultCount, searchErr)
			}
			if s.queryLog != nil && queryLogID != "" {
				iqID := NewQueryID()
				if err := s.queryLog.StartIndexerQuery(ctx, iqID, queryLogID, indexerID, evt.IndexerName); err != nil {
					s.logger.Warn("query log: start indexer failed", "err", err)
				}
				if err := s.queryLog.FinishIndexerQuery(ctx, iqID, evt.ResultCount, evt.ElapsedMS, searchErr); err != nil {
					s.logger.Warn("query log: finish indexer failed", "err", err)
				}
			}
			if evt.Type == EventIndexerResult {
				totalResults += evt.ResultCount
			}
		}

		select {
		case events <- evt:
		case <-ctx.Done():
			for range internal {
			}
			if s.queryLog != nil && queryLogID != "" {
				_ = s.queryLog.FinishQuery(ctx, queryLogID, totalResults, ctx.Err())
			}
			return
		}
	}

	if s.queryLog != nil && queryLogID != "" {
		if err := s.queryLog.FinishQuery(ctx, queryLogID, totalResults, nil); err != nil {
			s.logger.Warn("query log: finish failed", "err", err)
		}
	}

	s.auditLog.Log(ctx, auditlog.Entry{
		Category:   "search",
		EventType:  "search.completed",
		Message:    fmt.Sprintf("Search (stream) %q returned %d results", q.Term, totalResults),
		Detail:     auditlog.DetailJSON(map[string]any{"query_log_id": queryLogID, "query": q.Term, "query_type": "search-stream", "total_results": totalResults}),
		EntityType: auditlog.StrPtr("search"),
		EntityID:   auditlog.StrPtr(queryLogID),
		Level:      "info",
		Source:     auditlog.StrPtr("system"),
	})
}

// SearchHealthTracker returns the search health tracker.
func (s *Service) SearchHealthTracker() *SearchHealthTracker { return s.searchHealthTracker }

// QueryLog returns the query log store (may be nil).
func (s *Service) QueryLog() *QueryLog { return s.queryLog }

// CapsFor returns the live capability snapshot for id.
func (s *Service) CapsFor(id string) (Caps, bool) {
	ix, ok := s.registry.Get(id)
	if !ok {
		return Caps{}, false
	}
	return ix.Caps(), true
}
