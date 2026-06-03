package featureflags

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Service is the orchestrator for feature flags. It keeps an in-memory cache
// of overrides (loaded from the Store at startup and updated on Set) so that
// hot-path callers like the autosearch engine can check a flag without a DB
// round trip on every search.
type Service struct {
	store  *Store
	logger *slog.Logger

	mu        sync.RWMutex
	overrides map[string]bool
}

// NewService creates a feature flag Service. Call Load before serving to warm
// the cache from persisted overrides.
func NewService(store *Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:     store,
		logger:    logger,
		overrides: make(map[string]bool),
	}
}

// Load reads persisted overrides into the cache. Errors are logged and treated
// as best-effort: a transient DB issue falls back to registry defaults rather
// than blocking startup.
func (s *Service) Load(ctx context.Context) {
	all, err := s.store.GetAll(ctx)
	if err != nil {
		s.logger.Warn("featureflags: failed to load overrides; using defaults", "error", err)
		return
	}
	s.mu.Lock()
	s.overrides = all
	s.mu.Unlock()
}

// Enabled reports whether a flag is currently enabled. Unknown keys are
// reported as disabled.
func (s *Service) Enabled(key string) bool {
	if _, ok := definition(key); !ok {
		return false
	}
	s.mu.RLock()
	v, ok := s.overrides[key]
	s.mu.RUnlock()
	if ok {
		return v
	}
	d, _ := definition(key)
	return d.Default
}

// EnabledFunc returns a closure that reports the live value of a flag. Safe to
// call concurrently; reflects the latest value after Set.
func (s *Service) EnabledFunc(key string) func() bool {
	return func() bool { return s.Enabled(key) }
}

// List returns every known flag with its effective value.
func (s *Service) List() []Flag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Flag, 0, len(Definitions))
	for _, d := range Definitions {
		enabled := d.Default
		if v, ok := s.overrides[d.Key]; ok {
			enabled = v
		}
		out = append(out, Flag{FlagDef: d, Enabled: enabled})
	}
	return out
}

// Set persists and caches a new value for a known flag.
func (s *Service) Set(ctx context.Context, key string, enabled bool) error {
	if _, ok := definition(key); !ok {
		return fmt.Errorf("featureflags: unknown flag %q", key)
	}
	if err := s.store.Set(ctx, key, enabled); err != nil {
		return err
	}
	s.mu.Lock()
	s.overrides[key] = enabled
	s.mu.Unlock()
	return nil
}
