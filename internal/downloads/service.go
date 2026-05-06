package downloads

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
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

// RouteMounter mounts additional routes alongside the download-clients
// router. Mirrors the indexers package shape so future extensions
// (e.g. a per-client log viewer) can attach without editing server.go.
type RouteMounter func(chi.Router)

// ServiceOptions wires Service dependencies.
type ServiceOptions struct {
	Repository       Repository
	Registry         *Registry
	Logger           *slog.Logger
	Clock            Clock
	OperationTimeout time.Duration
	MaxParallel      int
	HealthTimeout    time.Duration
	RouteExtensions  []RouteMounter
	HistoryStore     *HistoryStore
}

// Service is the orchestrator that the HTTP layer depends on. It owns
// the lifecycle that links a persisted Definition to a live
// DownloadClient in the Registry.
type Service struct {
	repo             Repository
	registry         *Registry
	logger           *slog.Logger
	clock            Clock
	operationTimeout time.Duration
	maxParallel      int
	healthTimeout    time.Duration
	routeExtensions  []RouteMounter
	historyStore     *HistoryStore

	mu sync.Mutex
}

// NewService validates opts and returns a wired Service.
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Repository == nil {
		return nil, errors.New("downloads: repository must not be nil")
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
	if opts.OperationTimeout <= 0 {
		opts.OperationTimeout = 15 * time.Second
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 8
	}
	if opts.HealthTimeout <= 0 {
		opts.HealthTimeout = 10 * time.Second
	}
	return &Service{
		repo:             opts.Repository,
		registry:         opts.Registry,
		logger:           opts.Logger.With("module", "downloads"),
		clock:            opts.Clock,
		operationTimeout: opts.OperationTimeout,
		maxParallel:      opts.MaxParallel,
		healthTimeout:    opts.HealthTimeout,
		routeExtensions:  opts.RouteExtensions,
		historyStore:     opts.HistoryStore,
	}, nil
}

// Repository returns the underlying persistence layer.
func (s *Service) Repository() Repository { return s.repo }

// Registry returns the underlying live-instance registry.
func (s *Service) Registry() *Registry { return s.registry }

// AddRouteExtension appends a route mounter to the service's extensions.
// Must be called before the HTTP server is started.
func (s *Service) AddRouteExtension(m RouteMounter) {
	s.routeExtensions = append(s.routeExtensions, m)
}

// HydrateAll reads every persisted client and registers a live
// instance for each enabled row. Failures are logged and skipped so a
// single broken row never blocks boot.
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
			s.logger.Warn("hydrate download client failed",
				"id", def.ID, "kind", def.Kind, "err", err)
		}
	}
	s.logger.Info("download clients hydrated",
		"registered", s.registry.Len(), "definitions", len(defs))
	return nil
}

func (s *Service) hydrateOne(ctx context.Context, def Definition) error {
	c, err := build(ctx, def)
	if err != nil {
		return err
	}
	return s.registry.Replace(c)
}

// Create inserts a new persisted client and registers a live
// instance if it is enabled.
func (s *Service) Create(ctx context.Context, def Definition) (Definition, error) {
	if def.ID == "" {
		return Definition{}, errors.New("downloads: id required")
	}
	if def.Kind == "" {
		return Definition{}, errors.New("downloads: kind required")
	}
	if def.Protocol == "" {
		return Definition{}, errors.New("downloads: protocol required")
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
	_ = s.repo.UpsertHealth(ctx, Health{
		ClientID:      saved.ID,
		Status:        StatusUnknown,
		LastCheckedAt: s.clock.Now(),
	})
	return saved, nil
}

// Get returns the persisted Definition for id.
func (s *Service) Get(ctx context.Context, id string) (Definition, error) {
	return s.repo.Get(ctx, id)
}

// DefinitionWithHealth is the API-facing pair returned by List/Get.
type DefinitionWithHealth struct {
	Definition
	Health *Health `json:"health,omitempty"`
}

// List returns every persisted Definition with health attached.
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
		out = append(out, dh)
	}
	return out, nil
}

// GetWithHealth is the single-row sibling of List.
func (s *Service) GetWithHealth(ctx context.Context, id string) (DefinitionWithHealth, error) {
	def, err := s.repo.Get(ctx, id)
	if err != nil {
		return DefinitionWithHealth{}, err
	}
	dh := DefinitionWithHealth{Definition: def}
	if h, herr := s.repo.GetHealth(ctx, id); herr == nil {
		dh.Health = &h
	}
	return dh, nil
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

// TestOne runs a connectivity check against id, persists the outcome,
// and returns it. Used by POST /download-clients/{id}/test and the
// HealthChecker.
func (s *Service) TestOne(ctx context.Context, id string) (Health, error) {
	c, ok := s.registry.Get(id)
	if !ok {
		return Health{}, ErrNotFound
	}
	checkCtx := ctx
	if s.healthTimeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(ctx, s.healthTimeout)
		defer cancel()
	}
	err := c.Test(checkCtx)
	finished := s.clock.Now()

	prev, _ := s.repo.GetHealth(ctx, id)
	h := Health{
		ClientID:            id,
		LastCheckedAt:       finished,
		ConsecutiveFailures: prev.ConsecutiveFailures,
		LastSuccessAt:       prev.LastSuccessAt,
		LastFailureAt:       prev.LastFailureAt,
		LastFreeSpaceBytes:  prev.LastFreeSpaceBytes,
		LastCategories:      prev.LastCategories,
	}
	if err != nil {
		h.Status = StatusFailed
		h.LastError = fmt.Sprintf("download client %q test failed: %s", id, err.Error())
		h.ConsecutiveFailures++
		t := finished
		h.LastFailureAt = &t
	} else {
		h.Status = StatusOK
		h.ConsecutiveFailures = 0
		h.LastError = ""
		t := finished
		h.LastSuccessAt = &t
		// Best-effort enrich from the same client.
		if cats, cerr := c.Categories(checkCtx); cerr == nil {
			h.LastCategories = cats
		}
		if free, ferr := c.FreeSpace(checkCtx); ferr == nil && free >= 0 {
			f := free
			h.LastFreeSpaceBytes = &f
		}
	}
	if perr := s.repo.UpsertHealth(ctx, h); perr != nil {
		s.logger.Warn("persist download client health failed", "id", id, "err", perr)
	}
	return h, err
}

// FanOutOpts returns the fan-out options the HTTP layer hands to
// Registry.Status / Registry.FreeSpace / Registry.Test, configured
// from the Service's defaults.
func (s *Service) FanOutOpts(ids []string) FanOutOptions {
	return FanOutOptions{
		ClientIDs:        ids,
		PerClientTimeout: s.operationTimeout,
		MaxParallel:      s.maxParallel,
	}
}
