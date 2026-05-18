package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// stalledState tracks the last known progress of a download item
// for stall detection.
type stalledState struct {
	progress        float64
	downloadedBytes int64
	firstSeenAt     time.Time
	lastProgressAt  time.Time
}

// MonitorOrchNotifier is an optional interface for the orchestrator to receive
// download completion and progress events from the monitor without introducing
// a direct dependency on the workflows package.
type MonitorOrchNotifier interface {
	// NotifyDownloadComplete tells the orchestrator that a download has finished.
	NotifyDownloadComplete(clientID, downloadID, title, category string)
	// NotifyDownloadProgress reports progress for a specific download.
	NotifyDownloadProgress(clientID, downloadID string, progress float64, downSpeed, upSpeed int64, ratio float64, status string)
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
	historyStore *HistoryStore
	orchNotifier MonitorOrchNotifier

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
	HistoryStore    *HistoryStore
	OrchNotifier    MonitorOrchNotifier
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
		historyStore:    opts.HistoryStore,
		orchNotifier:    opts.OrchNotifier,
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

	// Forward progress updates to orchestrator for in-progress, seeding,
	// and completed downloads. Including completed ensures the orchestrator
	// can recover workflows stuck in "downloading" if the completion command
	// was dropped or deduped.
	if m.orchNotifier != nil {
		for _, item := range status.Items {
			switch item.Status {
			case StatusItemDownloading, StatusItemPaused, StatusItemSeeding, StatusItemCompleted:
				m.orchNotifier.NotifyDownloadProgress(
					item.ClientID, item.ID, item.Progress,
					item.DownloadRate, item.UploadRate,
					item.Ratio, string(item.Status),
				)
			}
		}
	}

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

	// Build a map of completed items in this sweep, keyed by clientID:itemID.
	// Seeding counts as "completed" for import purposes — the download is
	// finished and files are fully available on disk.
	thisRun := make(map[string]bool)
	for _, item := range items {
		key := item.ClientID + ":" + item.ID
		if item.Status == StatusItemCompleted || item.Status == StatusItemSeeding {
			if !m.lastCompleted[key] {
				// Check history store for idempotency across restarts.
				if m.historyStore != nil && m.historyStore.WasCompleted(ctx, item.ClientID, item.ID) {
					m.logger.Debug("monitor: skipping already-completed item",
						"item_id", item.ID, "client_id", item.ClientID)
					thisRun[key] = true
					continue
				}

				event := &DownloadCompletedEvent{
					DownloadID:  item.ID,
					ClientID:    item.ClientID,
					Title:       item.Title,
					Category:    item.Category,
					CompletedAt: m.clock.Now(),
				}
				_ = m.bus.Publish(ctx, event)

				// Notify orchestrator for workflow-tracked downloads.
				if m.orchNotifier != nil {
					m.orchNotifier.NotifyDownloadComplete(item.ClientID, item.ID, item.Title, item.Category)
				}

				// Persist to history store if available.
				if m.historyStore != nil {
					if err := m.historyStore.RecordCompletion(ctx, event); err != nil {
						m.logger.Warn("monitor: failed to record history",
							"item_id", item.ID, "error", err)
					}
				}

				m.logger.Info("monitor: emitted DownloadCompleted",
					"item_id", item.ID, "client_id", item.ClientID,
					"title", item.Title, "status", string(item.Status))
			}
			thisRun[key] = true
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
		key := item.ClientID + ":" + item.ID
		activeIDs[key] = true

		// Handle failed items immediately.
		if item.Status == StatusItemFailed {
			if !m.stalledEmitted[key] {
				m.stalledEmitted[key] = true
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

		prev, tracked := m.lastProgress[key]
		if !tracked {
			// First time seeing this item — start tracking.
			m.lastProgress[key] = stalledState{
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
			m.lastProgress[key] = stalledState{
				progress:        item.Progress,
				downloadedBytes: item.DownloadedBytes,
				firstSeenAt:     prev.firstSeenAt,
				lastProgressAt:  now,
			}
			// Clear stalled flag if it was set.
			delete(m.stalledEmitted, key)
			continue
		}

		// No progress — check if stall timeout exceeded.
		stalledDuration := now.Sub(prev.lastProgressAt)
		if stalledDuration >= m.stallTimeout && !m.stalledEmitted[key] {
			m.stalledEmitted[key] = true
			m.logger.Warn("monitor: download stalled",
				"item_id", item.ID,
				"client_id", item.ClientID,
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
	for key := range m.lastProgress {
		if !activeIDs[key] {
			delete(m.lastProgress, key)
			delete(m.stalledEmitted, key)
		}
	}
}

// RunLoop starts a polling loop that calls Run() at the configured interval.
// It blocks until ctx is cancelled. Use this instead of the scheduler for
// sub-minute polling intervals.
func (m *Monitor) RunLoop(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	m.logger.Info("monitor: starting polling loop", "interval", m.checkInterval)

	// Run an immediate sweep on startup so downloads that completed
	// during a restart are detected without waiting for the first tick.
	if err := m.Run(ctx); err != nil {
		m.logger.Error("monitor: initial sweep error", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("monitor: polling loop stopped")
			return
		case <-ticker.C:
			if err := m.Run(ctx); err != nil {
				m.logger.Error("monitor: sweep error", "error", err)
			}
		}
	}
}

// Close is a no-op for now but provided for consistency with Router.
func (m *Monitor) Close() error {
	return nil
}
