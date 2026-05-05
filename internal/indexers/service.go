package indexers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loomctl/loom/internal/indexers/throttle"
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
		opts.SearchTimeout = 15 * time.Second
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 8
	}
	if opts.HealthCheckTimeout <= 0 {
		opts.HealthCheckTimeout = 10 * time.Second
	}
	sht := opts.SearchHealthTracker
	if sht == nil {
		sht = NewSearchHealthTracker(opts.Registry)
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
	}, nil
}

// Repository returns the underlying persistence layer.
func (s *Service) Repository() Repository { return s.repo }

// Registry returns the underlying live-instance registry.
func (s *Service) Registry() *Registry { return s.registry }

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
	return saved, nil
}

// Delete removes both the live instance and the persisted row.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry.Remove(id)
	return s.repo.Delete(ctx, id)
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
		h.Status = StatusFailed
		h.LastError = fmt.Sprintf("indexer %q test failed: %s", id, err.Error())
	} else {
		h.Status = StatusOK
		t := finished
		h.LastSuccessAt = &t
	}
	if perr := s.repo.UpsertHealth(ctx, h); perr != nil {
		s.logger.Warn("persist health failed", "id", id, "err", perr)
	}
	return h, err
}

// Search fans an aggregated search out across the registered
// indexers, applying the configured timeout/parallelism caps.
func (s *Service) Search(ctx context.Context, q Query, ids []string, perTimeout time.Duration) AggregatedResults {
	if perTimeout <= 0 {
		perTimeout = s.searchTimeout
	}
	agg := s.registry.Search(ctx, q, SearchOptions{
		IndexerIDs:        ids,
		PerIndexerTimeout: perTimeout,
		MaxParallel:       s.maxParallel,
	})
	// Record search metrics for each indexer.
	if s.searchHealthTracker != nil && agg.Diagnostics != nil {
		for _, d := range agg.Diagnostics.Indexers {
			var indexerID string
			for _, ix := range s.registry.List() {
				if ix.Name() == d.Name {
					indexerID = ix.ID()
					break
				}
			}
			if indexerID == "" {
				continue
			}
			dur := time.Duration(d.ResponseTimeMS) * time.Millisecond
			var searchErr error
			if d.Status == "error" || d.Status == "timeout" {
				searchErr = errors.New(d.ErrorMessage)
			}
			s.searchHealthTracker.RecordSearch(indexerID, dur, d.ResultCount, searchErr)
		}
	}
	return agg
}

// SearchHealthTracker returns the search health tracker.
func (s *Service) SearchHealthTracker() *SearchHealthTracker { return s.searchHealthTracker }

// CapsFor returns the live capability snapshot for id.
func (s *Service) CapsFor(id string) (Caps, bool) {
	ix, ok := s.registry.Get(id)
	if !ok {
		return Caps{}, false
	}
	return ix.Caps(), true
}
