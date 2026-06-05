package indexers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ebenderooock/loom/internal/indexers/throttle"
)

// Factory builds a live Indexer from a persisted Definition. Each kind
// (Cardigann, Newznab, etc.) registers a Factory under its kind
// string; the Service uses the catalogue to hydrate rows at startup
// and after CRUD operations.
type Factory func(ctx context.Context, def Definition) (Indexer, error)

// TransportProvider returns the http.RoundTripper that an indexer
// kind should use for outbound HTTP. proxyID is the Definition's
// ProxyID — empty means "use the default transport". Implementations
// must return a non-nil RoundTripper; on lookup failure they should
// fall back to http.DefaultTransport rather than blocking the build.
//
// The interface is small on purpose: it lets the proxies package
// (Phase 2e) inject per-indexer routing without dragging the kinds
// package into the proxies type tree.
type TransportProvider interface {
	TransportFor(proxyID string) (http.RoundTripper, error)
}

// kindRegistry is the package-global catalogue of factories. It is
// safe for concurrent reads after init; writes happen via
// RegisterKind and are guarded by a mutex.
var (
	kindMu       sync.RWMutex
	kindHandlers = make(map[Kind]Factory)

	transportMu       sync.RWMutex
	transportProvider TransportProvider

	rateLimitMu       sync.RWMutex
	rateLimitProvider RateLimitProvider
)

// RateLimitProvider returns the per-indexer Config that should govern
// outbound HTTP for the given indexer ID. The Service implements this
// interface and registers itself at startup; tests can install a
// stub via SetRateLimitProvider. A nil provider, or a missing row,
// means "use throttle.Defaults()".
type RateLimitProvider interface {
	RateLimitFor(indexerID string) (throttle.Config, bool)
}

// SetRateLimitProvider installs the package-global RateLimitProvider.
// Passing nil clears it (TransportForDefinition then falls back to
// throttle defaults).
func SetRateLimitProvider(p RateLimitProvider) {
	rateLimitMu.Lock()
	rateLimitProvider = p
	rateLimitMu.Unlock()
}

// CurrentRateLimitProvider returns the currently installed
// RateLimitProvider, or nil if none has been set.
func CurrentRateLimitProvider() RateLimitProvider {
	rateLimitMu.RLock()
	defer rateLimitMu.RUnlock()
	return rateLimitProvider
}

// SetTransportProvider installs the package-global TransportProvider.
// Kind factories should call CurrentTransportProvider() inside their
// build function to honour per-indexer proxy selection. Passing nil
// reverts to "no provider" (kinds will fall back to default
// transports).
func SetTransportProvider(p TransportProvider) {
	transportMu.Lock()
	transportProvider = p
	transportMu.Unlock()
}

// CurrentTransportProvider returns the currently installed
// TransportProvider, or nil if none has been set. Kind factories
// should treat nil as "use http.DefaultTransport".
func CurrentTransportProvider() TransportProvider {
	transportMu.RLock()
	defer transportMu.RUnlock()
	return transportProvider
}

// TransportForDefinition is a thin convenience wrapper used by kind
// factories. It composes the outbound transport stack:
//
//	base → proxy → throttle (rate-limit + retry)
//
// The proxy lookup is delegated to the registered TransportProvider
// (Phase 2e); the throttle layer wraps the result using the per-
// indexer Config supplied by the registered RateLimitProvider, or
// throttle.Defaults() when no provider is installed. When no proxy
// is requested we still apply throttle on top of http.DefaultTransport
// so rate limiting works in tests and minimal deployments.
func TransportForDefinition(def Definition) (http.RoundTripper, error) {
	// Step 1: pick the base / proxy transport.
	var (
		base = http.DefaultTransport
		err  error
	)
	if def.ProxyID != "" {
		if p := CurrentTransportProvider(); p != nil {
			base, err = p.TransportFor(def.ProxyID)
			if err != nil {
				return nil, err
			}
		}
	}
	// Step 2: wrap with the per-indexer throttle layer.
	cfg := throttle.Defaults()
	if rp := CurrentRateLimitProvider(); rp != nil {
		if c, ok := rp.RateLimitFor(def.ID); ok {
			cfg = throttle.Resolve(c)
		}
	}
	// Honour per-definition RequestDelay (milliseconds between
	// requests). When set, cap PerMinute so the bucket never issues
	// tokens faster than the definition demands.
	if def.RequestDelay > 0 {
		maxRPM := 60000 / def.RequestDelay // e.g. 2000ms → 30 rpm
		if maxRPM < 1 {
			maxRPM = 1
		}
		if cfg.PerMinute > maxRPM || cfg.PerMinute <= 0 {
			cfg.PerMinute = maxRPM
		}
		if cfg.Burst > maxRPM {
			cfg.Burst = maxRPM
		}
	}
	return throttle.Wrap(base, def.ID, string(def.Kind), cfg, throttle.Options{}), nil
}

// RegisterKind installs f as the factory for kind. It is idempotent;
// re-registering the same kind overwrites the previous factory, which
// is convenient for tests.
func RegisterKind(kind Kind, f Factory) {
	if kind == "" {
		panic("indexers: RegisterKind with empty kind")
	}
	if f == nil {
		panic("indexers: RegisterKind with nil factory")
	}
	kindMu.Lock()
	defer kindMu.Unlock()
	kindHandlers[kind] = f
}

// LookupKind returns the factory for kind, or ErrUnknownKind if no
// factory is registered.
func LookupKind(kind Kind) (Factory, error) {
	kindMu.RLock()
	defer kindMu.RUnlock()
	f, ok := kindHandlers[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, string(kind))
	}
	return f, nil
}

// Kinds returns every registered kind in insertion-independent order.
// Useful for listing in /api/v1/indexers/kinds (added in a later
// phase) and for logging.
func Kinds() []Kind {
	kindMu.RLock()
	defer kindMu.RUnlock()
	out := make([]Kind, 0, len(kindHandlers))
	for k := range kindHandlers {
		out = append(out, k)
	}
	return out
}

// build hydrates a Definition into an Indexer using the registered
// factory for def.Kind.
func build(ctx context.Context, def Definition) (Indexer, error) {
	if def.ID == "" {
		return nil, errors.New("indexers: build: empty id")
	}
	f, err := LookupKind(def.Kind)
	if err != nil {
		return nil, err
	}
	ix, err := f(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("indexers: build %q (%s): %w", def.ID, def.Kind, err)
	}
	return ix, nil
}

// nullIndexer is the Phase 2a placeholder. It satisfies Indexer and
// returns no results so the API can be exercised end-to-end before
// real kinds (Newznab/Cardigann) land.
type nullIndexer struct {
	id   string
	name string
}

func (n *nullIndexer) ID() string   { return n.id }
func (n *nullIndexer) Name() string { return n.name }
func (n *nullIndexer) Caps() Caps {
	return Caps{
		SearchTypes:  []string{"search"},
		Categories:   CategoryFamilies(),
		SupportedIDs: []string{},
	}
}
func (n *nullIndexer) Search(_ context.Context, _ Query) (*Results, error) {
	return &Results{IndexerID: n.id, Items: []Result{}, Total: 0}, nil
}
func (n *nullIndexer) Test(_ context.Context) error { return nil }

func init() {
	RegisterKind(KindNull, func(_ context.Context, def Definition) (Indexer, error) {
		return &nullIndexer{id: def.ID, name: def.Name}, nil
	})
}
