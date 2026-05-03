package cardigann

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/loomctl/loom/internal/indexers"
)

// Kind is the registry key for this indexer kind.
const Kind indexers.Kind = "cardigann"

// loaderMu guards the package-global Loader so cmd/loom can install
// it once at boot and the factory closure can find it on demand.
var (
	loaderMu  sync.RWMutex
	defLoader *Loader
)

// SetLoader installs the package-global Loader. cmd/loom calls this
// once during boot, before HydrateAll constructs any cardigann
// engines. Passing nil disables the kind: factory calls will return
// a clear error.
func SetLoader(l *Loader) {
	loaderMu.Lock()
	defer loaderMu.Unlock()
	defLoader = l
}

// CurrentLoader returns the installed Loader or nil. Exported for
// tests and for the eventual "list available definitions" API.
func CurrentLoader() *Loader {
	loaderMu.RLock()
	defer loaderMu.RUnlock()
	return defLoader
}

// httpClientFactory mirrors the newznab pattern: tests override it to
// hand the engine a transport that talks to httptest.NewServer. The
// production builder honours per-indexer proxies via
// indexers.TransportForDefinition.
var httpClientFactory = func(cfg Config, def indexers.Definition) *http.Client {
	rt, err := indexers.TransportForDefinition(def)
	if err != nil || rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{Timeout: cfg.Timeout.duration(), Transport: rt}
}

// SetHTTPClientFactory installs a custom HTTP client builder. Tests
// inject a transport that points at httptest.NewServer; production
// code never needs to call this.
func SetHTTPClientFactory(f func(cfg Config, def indexers.Definition) *http.Client) {
	httpClientFactory = f
}

// factory builds a live Engine for a persisted Definition.
func factory(_ context.Context, def indexers.Definition) (indexers.Indexer, error) {
	cfg, err := parseConfig(def.Config)
	if err != nil {
		return nil, fmt.Errorf("indexer %q (cardigann): %w", def.ID, err)
	}
	loader := CurrentLoader()
	if loader == nil {
		return nil, errors.New("cardigann: no definition loader is installed; call cardigann.SetLoader at boot")
	}
	id := cfg.resolvedDefinitionID()
	defYAML, ok := loader.Get(id)
	if !ok {
		return nil, fmt.Errorf("cardigann: definition %q not found under %q", id, loader.Root())
	}
	return NewEngine(def.ID, def.Name, defYAML, cfg, httpClientFactory(cfg, def))
}

func init() {
	indexers.RegisterKind(Kind, factory)
}
