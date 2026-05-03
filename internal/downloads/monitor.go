package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loomctl/loom/internal/kernel/eventbus"
)

// Monitor periodically checks the status of downloads on all configured
// clients and emits completion events when items finish. It decouples
// status polling from the indexer intake and routing pipelines, reducing
// latency jitter and allowing recovery if a client is temporarily unavailable.
type Monitor struct {
	svc    *Service
	bus    eventbus.Bus
	logger *slog.Logger
	clock  Clock

	// Configurable check interval. Defaults to 30 seconds; can be
	// overridden via env variable or test injection.
	checkInterval time.Duration

	mu sync.Mutex
	// lastCompleted tracks the per-client items we saw as completed
	// in the last check, so we only emit new completions.
	lastCompleted map[string]map[string]bool // clientID -> itemID -> seen
}

// MonitorOptions wires a Monitor.
type MonitorOptions struct {
	Service       *Service
	Bus           eventbus.Bus
	Logger        *slog.Logger
	Clock         Clock
	CheckInterval time.Duration
}

// NewMonitor returns a Monitor wired to the Service and event bus.
// Callers are responsible for invoking Run() periodically; integration
// with the scheduler happens at a higher level.
func NewMonitor(opts MonitorOptions) (*Monitor, error) {
	if opts.Service == nil {
		return nil, fmt.Errorf("downloads: Monitor Service required")
	}
	if opts.Bus == nil {
		return nil, fmt.Errorf("downloads: Monitor Bus required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = 30 * time.Second
	}

	m := &Monitor{
		svc:           opts.Service,
		bus:           opts.Bus,
		logger:        opts.Logger.With("module", "downloads/monitor"),
		clock:         opts.Clock,
		checkInterval: opts.CheckInterval,
		lastCompleted: make(map[string]map[string]bool),
	}

	m.logger.Info("monitor initialized", "interval", opts.CheckInterval)
	return m, nil
}

// Run is the scheduler entry point. It runs a single status sweep across
// all clients and emits completion events for any newly-completed items.
func (m *Monitor) Run(ctx context.Context) error {
	m.logger.Debug("monitor: starting status sweep")

	// Fan out Status() across all clients. We use the service's configured
	// timeouts and parallelism limits.
	opts := m.svc.FanOutOpts(nil) // nil = all clients
	status := m.svc.registry.Status(ctx, nil, opts)

	if len(status.Errors) > 0 {
		m.logger.Warn("monitor: some clients failed",
			"error_count", len(status.Errors))
		// Log each error without failing the whole sweep.
		for clientID, errStr := range status.Errors {
			m.logger.Debug("monitor: client status failed",
				"client_id", clientID, "error", errStr)
		}
	}

	// Process results: emit DownloadCompleted for any newly completed items.
	m.emitCompletions(ctx, status.Items)

	m.logger.Debug("monitor: status sweep completed", "items", len(status.Items))
	return nil
}

// emitCompletions checks each item's status and emits DownloadCompleted
// events for items that transitioned to completed in this sweep but were
// not completed in the previous sweep.
func (m *Monitor) emitCompletions(ctx context.Context, items []Item) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a map of completed items by client in this sweep.
	thisRun := make(map[string]map[string]bool) // clientID -> itemID -> seen
	for _, item := range items {
		if item.Status == StatusItemCompleted {
			if _, ok := thisRun[item.ID]; !ok {
				thisRun[item.ID] = make(map[string]bool)
			}
			// For completed items, we extract the client ID from the item's
			// Category or metadata. Since the Item structure does not carry
			// the client ID explicitly, we infer it from the status response.
			// In practice, the Registry.Status aggregation will group items
			// by client, but here we treat it as a flat list. We emit based
			// on the item's presence in the results; the client can be
			// inferred from context. TODO: extend Item to carry ClientID.

			// For now, emit if we haven't seen this item before.
			wasCompleted := false
			for _, clientCompleted := range m.lastCompleted {
				if clientCompleted[item.ID] {
					wasCompleted = true
					break
				}
			}
			if !wasCompleted {
				_ = m.bus.Publish(ctx, &DownloadCompletedEvent{
					DownloadID:  item.ID,
					ClientID:    "", // Inferred from context; TODO: add to Item.
					Title:       item.Title,
					Category:    item.Category,
					CompletedAt: m.clock.Now(),
				})
				m.logger.Info("monitor: emitted DownloadCompleted",
					"item_id", item.ID, "title", item.Title)
			}
		}
	}

	// Update lastCompleted for the next sweep.
	m.lastCompleted = thisRun
}

// Close is a no-op for now but provided for consistency with Router.
func (m *Monitor) Close() error {
	return nil
}
