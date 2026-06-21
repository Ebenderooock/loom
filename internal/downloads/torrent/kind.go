package torrent

import (
	"context"
	"log/slog"
	"sync"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind is the registry key under which this implementation registers
// itself with the downloads core.
const Kind = downloads.KindBuiltinTorrent

var (
	engineMu     sync.Mutex
	sharedEngine *Engine
)

// getOrCreateEngine returns the singleton Engine, creating it on the
// first call. The first Config wins for engine-level settings (listen
// port, DHT, PEX, etc.); subsequent Definitions with different
// engine-level values are tolerated silently because the engine is
// already running.
func getOrCreateEngine(_ context.Context, cfg Config, logger *slog.Logger) (*Engine, error) {
	engineMu.Lock()
	defer engineMu.Unlock()

	if sharedEngine != nil {
		return sharedEngine, nil
	}

	e, err := NewEngine(cfg, logger)
	if err != nil {
		return nil, err
	}

	// Start the seeding supervisor in the background.
	// nolint:contextcheck,gosec // Background task intentionally uses context.Background()
	go func() { _ = e.Start(context.Background()) }()

	sharedEngine = e
	return e, nil
}

// factory is the downloads.Factory the package registers under Kind.
func factory(ctx context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
	cfg, err := parseConfig(def)
	if err != nil {
		return nil, err
	}

	logger := slog.Default().With("kind", string(Kind), "client_id", def.ID)

	engine, err := getOrCreateEngine(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	return New(def, engine)
}

func init() {
	downloads.RegisterKind(Kind, factory)
}
