package indexers

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Factory builds a live Indexer from a persisted Definition. Each kind
// (Cardigann, Newznab, etc.) registers a Factory under its kind
// string; the Service uses the catalogue to hydrate rows at startup
// and after CRUD operations.
type Factory func(ctx context.Context, def Definition) (Indexer, error)

// kindRegistry is the package-global catalogue of factories. It is
// safe for concurrent reads after init; writes happen via
// RegisterKind and are guarded by a mutex.
var (
	kindMu       sync.RWMutex
	kindHandlers = make(map[Kind]Factory)
)

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
