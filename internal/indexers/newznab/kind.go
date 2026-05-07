package newznab

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/ebenderooock/loom/internal/indexers"
)

// Kind strings registered by this package. Definitions in the
// database reference these and the OpenAPI spec documents both shapes
// under Indexer.config oneOf.
const (
	KindNewznab indexers.Kind = "newznab"
	KindTorznab indexers.Kind = "torznab"
)

// capsCache is a package-level pointer set by SetCapsCache so factory
// closures can persist caps without altering the
// indexers.Factory(ctx, def) signature.
var (
	capsCacheMu sync.RWMutex
	capsCache   indexers.CapsCache
)

// SetCapsCache wires a CapsCache into the package. cmd/loom calls
// this once during boot, after building storage but before the first
// HydrateAll. Safe to call concurrently and idempotent.
func SetCapsCache(c indexers.CapsCache) {
	capsCacheMu.Lock()
	defer capsCacheMu.Unlock()
	capsCache = c
}

func currentCapsCache() indexers.CapsCache {
	capsCacheMu.RLock()
	defer capsCacheMu.RUnlock()
	return capsCache
}

// httpClientFactory is overridable by tests so they can hand the
// Client a transport pointing at httptest.NewServer without resorting
// to monkey-patching. The default builder honours per-indexer
// proxies via indexers.TransportForDefinition; if no provider is
// installed (or def.ProxyID is empty) the indexer ends up with
// http.DefaultTransport.
var httpClientFactory = func(cfg Config, def indexers.Definition) *http.Client {
	rt, err := indexers.TransportForDefinition(def)
	if err != nil || rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{Timeout: cfg.Timeout.duration(), Transport: rt}
}

// SetHTTPClientFactory installs a custom builder. Production callers
// don't need this; tests use it to inject httptest transports. The
// builder receives both the parsed Config and the source Definition
// so tests can branch on def.ProxyID, ID, or anything else they need.
func SetHTTPClientFactory(f func(cfg Config, def indexers.Definition) *http.Client) {
	httpClientFactory = f
}

// factoryFor returns an indexers.Factory closure pinned to flavour.
// Both newznab and torznab share build logic; only the attribute
// namespace differs.
func factoryFor(flavour attrFlavour) indexers.Factory {
	return func(_ context.Context, def indexers.Definition) (indexers.Indexer, error) {
		cfg, err := parseConfig(def.Config)
		if err != nil {
			return nil, fmt.Errorf("indexer %q (%s): %w", def.ID, flavour.kind(), err)
		}
		cfg.attrFlavour = flavour
		client := NewClient(def.ID, def.Name, cfg, httpClientFactory(cfg, def), currentCapsCache())
		return client, nil
	}
}

func init() {
	indexers.RegisterKind(KindNewznab, factoryFor(flavourNewznab))
	indexers.RegisterKind(KindTorznab, factoryFor(flavourTorznab))
}
