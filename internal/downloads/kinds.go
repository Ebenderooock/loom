package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ebenderooock/loom/internal/indexers/throttle"
)

// Factory builds a live DownloadClient from a persisted Definition.
// Each kind registers a Factory under its kind string; the Service
// uses the catalogue to hydrate rows at startup and after CRUD.
type Factory func(ctx context.Context, def Definition) (DownloadClient, error)

// TransportProvider returns the http.RoundTripper that a download
// kind should use for outbound HTTP. proxyID is unused today (the
// download_clients schema does not carry a proxy_id column yet) but
// the seam exists so a future phase can drop it in without a wire
// break.
//
// The interface is small on purpose: kinds depend on it instead of
// the concrete proxies package. Any implementation that satisfies it
// — including the indexers' *proxies.Provider — can be wired in
// directly.
type TransportProvider interface {
	TransportFor(proxyID string) (http.RoundTripper, error)
}

// RateLimitProvider returns the per-client throttle Config. A nil
// provider, or a missing entry, means "use throttle.Defaults()". The
// downloads package consumes the interface but does not implement it
// itself today — most download protocols already self-throttle.
type RateLimitProvider interface {
	RateLimitFor(clientID string) (throttle.Config, bool)
}

var (
	kindMu       sync.RWMutex
	kindHandlers = make(map[Kind]Factory)

	transportMu       sync.RWMutex
	transportProvider TransportProvider

	rateLimitMu       sync.RWMutex
	rateLimitProvider RateLimitProvider
)

// SetTransportProvider installs the package-global TransportProvider.
// Passing nil clears it.
func SetTransportProvider(p TransportProvider) {
	transportMu.Lock()
	transportProvider = p
	transportMu.Unlock()
}

// CurrentTransportProvider returns the currently installed
// TransportProvider, or nil if none has been set.
func CurrentTransportProvider() TransportProvider {
	transportMu.RLock()
	defer transportMu.RUnlock()
	return transportProvider
}

// SetRateLimitProvider installs the package-global RateLimitProvider.
// Passing nil clears it.
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

// TransportForDefinition composes the outbound transport stack for a
// download client:
//
//	base → (optional proxy) → throttle (rate-limit + retry)
//
// We reuse the indexers throttle layer verbatim — a downloader that
// hammers a tracker API gets the same protections as a search client
// would. The proxy seam is kept identical to the indexers package so
// a future per-client proxy column drops in without API churn.
func TransportForDefinition(def Definition) (http.RoundTripper, error) {
	var (
		base = http.DefaultTransport
		err  error
	)
	// Reserved for a future per-client proxy_id column. The provider
	// is consulted today only when explicitly invoked by a kind.
	_ = base
	cfg := throttle.Defaults()
	if rp := CurrentRateLimitProvider(); rp != nil {
		if c, ok := rp.RateLimitFor(def.ID); ok {
			cfg = throttle.Resolve(c)
		}
	}
	wrapped := throttle.Wrap(base, def.ID, string(def.Kind), cfg, throttle.Options{})
	return wrapped, err
}

// RegisterKind installs f as the factory for kind. Idempotent;
// re-registering the same kind overwrites the previous factory.
func RegisterKind(kind Kind, f Factory) {
	if kind == "" {
		panic("downloads: RegisterKind with empty kind")
	}
	if f == nil {
		panic("downloads: RegisterKind with nil factory")
	}
	kindMu.Lock()
	defer kindMu.Unlock()
	kindHandlers[kind] = f
}

// LookupKind returns the factory for kind, or ErrUnknownKind if none
// is registered.
func LookupKind(kind Kind) (Factory, error) {
	kindMu.RLock()
	defer kindMu.RUnlock()
	f, ok := kindHandlers[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, string(kind))
	}
	return f, nil
}

// Kinds returns every registered kind.
func Kinds() []Kind {
	kindMu.RLock()
	defer kindMu.RUnlock()
	out := make([]Kind, 0, len(kindHandlers))
	for k := range kindHandlers {
		out = append(out, k)
	}
	return out
}

// build hydrates a Definition into a DownloadClient.
func build(ctx context.Context, def Definition) (DownloadClient, error) {
	if def.ID == "" {
		return nil, errors.New("downloads: build: empty id")
	}
	f, err := LookupKind(def.Kind)
	if err != nil {
		return nil, err
	}
	c, err := f(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("downloads: build %q (%s): %w", def.ID, def.Kind, err)
	}
	return c, nil
}

// nullClient is the Phase 3a placeholder. It satisfies DownloadClient
// and never actually downloads anything, so the API can be exercised
// end-to-end before real kinds (qBittorrent, SABnzbd, ...) land.
type nullClient struct {
	id       string
	name     string
	protocol Protocol
}

func (n *nullClient) ID() string         { return n.id }
func (n *nullClient) Name() string       { return n.name }
func (n *nullClient) Kind() Kind         { return KindNull }
func (n *nullClient) Protocol() Protocol { return n.protocol }

func (n *nullClient) Add(_ context.Context, _ AddRequest) (AddResult, error) {
	return AddResult{ClientID: n.id, ItemID: ""}, nil
}
func (n *nullClient) Status(_ context.Context, _ ...string) ([]Item, error) {
	return []Item{}, nil
}
func (n *nullClient) Pause(_ context.Context, _ ...string) error  { return nil }
func (n *nullClient) Resume(_ context.Context, _ ...string) error { return nil }
func (n *nullClient) Remove(_ context.Context, _ []string, _ bool) error {
	return nil
}
func (n *nullClient) SetPriority(_ context.Context, _ Priority, _ ...string) error {
	return nil
}
func (n *nullClient) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error {
	return nil
}
func (n *nullClient) ForceStart(_ context.Context, _ ...string) error { return nil }
func (n *nullClient) Recheck(_ context.Context, _ ...string) error    { return nil }
func (n *nullClient) Reannounce(_ context.Context, _ ...string) error { return nil }
func (n *nullClient) Categories(_ context.Context) ([]Category, error) {
	return []Category{}, nil
}
func (n *nullClient) FreeSpace(_ context.Context) (int64, error) { return -1, nil }
func (n *nullClient) Test(_ context.Context) error               { return nil }

func init() {
	RegisterKind(KindNull, func(_ context.Context, def Definition) (DownloadClient, error) {
		proto := def.Protocol
		if proto == "" {
			proto = ProtocolTorrent
		}
		return &nullClient{id: def.ID, name: def.Name, protocol: proto}, nil
	})
}
