package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loomctl/loom/internal/kernel/eventbus"
)

// stalledState tracks the last known progress of a download item
// for stall detection.
type stalledState struct {
	progress        float64
	downloadedBytes int64
	firstSeenAt     time.Time
	lastProgressAt  time.Time
}

// Monitor periodically checks the status of downloads on all configured
// clients and emits completion events when items finish. It decouples
// status polling from the indexer intake and routing pipelines, reducing
// latency jitter and allowing recovery if a client is temporarily unavailable.
type Monitor struct {
	svc          *Service
	bus          eventbus.Bus
	logger       *slog.Logger
	clock        Clock
	stallHandler *StallHandler

	// Configurable check interval. Defaults to 30 seconds; can be
	// overridden via env variable or test injection.
	checkInterval time.Duration

	// Stall detection config.
	stallTimeout     time.Duration
	checkForStalled  bool

	mu sync.Mutex
	// lastCompleted tracks the item IDs we've already emitted as completed
	// so we only emit new completions.
	lastCompleted map[string]bool // itemID -> seen
	// lastProgress tracks per-item progress for stall detection.
	lastProgress map[string]stalledState
	// stalledEmitted prevents re-handling the same stalled item.
	stalledEmitted map[string]bool
}

// MonitorOptions wires a Monitor.
type MonitorOptions struct {
	Service         *Service
	Bus             eventbus.Bus
	Logger          *slog.Logger
	Clock           Clock
	CheckInterval   time.Duration
	StallTimeout    time.Duration
	CheckForStalled bool
	StallHandler    *StallHandler
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
	if opts.StallTimeout <= 0 {
		opts.StallTimeout = 30 * time.Minute
	}

	m := &Monitor{
		svc:             opts.Service,
		bus:             opts.Bus,
		logger:          opts.Logger.With("module", "downloads/monitor"),
		clock:           opts.Clock,
		stallHandler:    opts.StallHandler,
		checkInterval:   opts.CheckInterval,
		stallTimeout:    opts.StallTimeout,
		checkForStalled: opts.CheckForStalled,
		lastCompleted:   make(map[string]bool),
		lastProgress:    make(map[string]stalledState),
		stalledEmitted:  make(map[string]bool),
	}

	m.logger.Info("monitor initialized", "interval", opts.CheckInterval,
		"stall_detection", opts.CheckForStalled, "stall_timeout", opts.StallTimeout)
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

	// Process results: detect stalled/failed downloads.
	if m.checkForStalled {
		m.detectStalled(ctx, status.Items)
	}

	m.logger.Debug("monitor: status sweep completed", "items", len(status.Items))
	return nil
}

// emitCompletions checks each item's status and emits DownloadCompleted
// events for items that transitioned to completed in this sweep but were
// not completed in the previous sweep.
func (m *Monitor) emitCompletions(ctx context.Context, items []Item) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a map of completed items in this sweep.
	thisRun := make(map[string]bool) // itemID -> seen
	for _, item := range items {
		if item.Status == StatusItemCompleted {
			// Emit if we haven't seen this item before.
			if !m.lastCompleted[item.ID] {
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
			thisRun[item.ID] = true
		}
	}

	// Update lastCompleted for the next sweep.
	m.lastCompleted = thisRun
}

// detectStalled checks items for stall conditions and failed status.
func (m *Monitor) detectStalled(ctx context.Context, items []Item) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	activeIDs := make(map[string]bool, len(items))

	for _, item := range items {
		activeIDs[item.ID] = true

		// Handle failed items immediately.
		if item.Status == StatusItemFailed {
			if !m.stalledEmitted[item.ID] {
				m.stalledEmitted[item.ID] = true
				if m.stallHandler != nil {
					m.stallHandler.Handle(ctx, item, "download failed: "+item.Message)
				}
			}
			continue
		}

		// Only check downloading items for stalls.
		if item.Status != StatusItemDownloading {
			continue
		}

		prev, tracked := m.lastProgress[item.ID]
		if !tracked {
			// First time seeing this item — start tracking.
			m.lastProgress[item.ID] = stalledState{
				progress:        item.Progress,
				downloadedBytes: item.DownloadedBytes,
				firstSeenAt:     now,
				lastProgressAt:  now,
			}
			continue
		}

		// Check if progress changed.
		if item.Progress != prev.progress || item.DownloadedBytes != prev.downloadedBytes {
			// Progress made — update tracking.
			m.lastProgress[item.ID] = stalledState{
				progress:        item.Progress,
				downloadedBytes: item.DownloadedBytes,
				firstSeenAt:     prev.firstSeenAt,
				lastProgressAt:  now,
			}
			// Clear stalled flag if it was set.
			delete(m.stalledEmitted, item.ID)
			continue
		}

		// No progress — check if stall timeout exceeded.
		stalledDuration := now.Sub(prev.lastProgressAt)
		if stalledDuration >= m.stallTimeout && !m.stalledEmitted[item.ID] {
			m.stalledEmitted[item.ID] = true
			m.logger.Warn("monitor: download stalled",
				"item_id", item.ID,
				"title", item.Title,
				"stalled_for", stalledDuration.String(),
				"progress", item.Progress,
			)
			if m.stallHandler != nil {
				reason := fmt.Sprintf("no progress for %s", stalledDuration.Truncate(time.Second))
				m.stallHandler.Handle(ctx, item, reason)
			}
		}
	}

	// Prune tracking maps for items no longer active.
	for id := range m.lastProgress {
		if !activeIDs[id] {
			delete(m.lastProgress, id)
			delete(m.stalledEmitted, id)
		}
	}
}

// Close is a no-op for now but provided for consistency with Router.
func (m *Monitor) Close() error {
	return nil
}
