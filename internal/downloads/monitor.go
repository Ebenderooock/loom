package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/kernel/telemetry"
)

// stalledState tracks the last known progress of a download item
// for stall detection.
type stalledState struct {
	progress        float64
	downloadedBytes int64
	firstSeenAt     time.Time
	lastProgressAt  time.Time
	// stalledFor accumulates time without progress, counted only while the
	// network was healthy. Outages therefore never make a download "stall".
	stalledFor time.Duration
}

// MonitorOrchNotifier is an optional interface for the orchestrator to receive
// download completion and progress events from the monitor without introducing
// a direct dependency on the workflows package.
type MonitorOrchNotifier interface {
	// NotifyDownloadComplete tells the orchestrator that a download has finished.
	// contentPath is the actual on-disk path; savePath is the client's save directory.
	// Either may be empty if the client doesn't report them.
	NotifyDownloadComplete(clientID, downloadID, title, category, contentPath, savePath string)
	// NotifyDownloadProgress reports progress for a specific download.
	// contentPath and savePath are passed so the orchestrator can cache them in
	// workflow metadata before the download client removes the completed item.
	NotifyDownloadProgress(clientID, downloadID string, progress float64, downSpeed, upSpeed int64, ratio float64, status, contentPath, savePath string)
	// NotifyDownloadFailed tells the orchestrator that a download stalled or
	// failed and was removed from the client, so its workflow can be failed
	// and a replacement searched for.
	NotifyDownloadFailed(clientID, downloadID, reason string)
}

// ConnectivityCheck reports whether general network connectivity is up. It
// lets stall detection tell a dead torrent apart from a network outage.
type ConnectivityCheck func(ctx context.Context) bool

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
	stallTimeout      time.Duration
	noStartTimeout    time.Duration
	checkForStalled   bool
	connectivityCheck ConnectivityCheck
	lastSweepAt       time.Time

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
	Service       *Service
	Bus           eventbus.Bus
	Logger        *slog.Logger
	Clock         Clock
	CheckInterval time.Duration
	StallTimeout  time.Duration
	// NoStartTimeout is the stall timeout for downloads that have never
	// received any data (dead magnets, no seeders). Defaults to 2×StallTimeout.
	NoStartTimeout  time.Duration
	CheckForStalled bool
	// ConnectivityCheck, if set, is consulted when no download is moving to
	// decide whether the network is up. Stall time only accrues while it is.
	ConnectivityCheck ConnectivityCheck
	StallHandler      *StallHandler
	HistoryStore      *HistoryStore
	OrchNotifier      MonitorOrchNotifier
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
	if opts.NoStartTimeout <= 0 {
		opts.NoStartTimeout = 2 * opts.StallTimeout
	}

	m := &Monitor{
		svc:               opts.Service,
		bus:               opts.Bus,
		logger:            opts.Logger.With("module", "downloads/monitor"),
		clock:             opts.Clock,
		stallHandler:      opts.StallHandler,
		historyStore:      opts.HistoryStore,
		orchNotifier:      opts.OrchNotifier,
		checkInterval:     opts.CheckInterval,
		stallTimeout:      opts.StallTimeout,
		noStartTimeout:    opts.NoStartTimeout,
		checkForStalled:   opts.CheckForStalled,
		connectivityCheck: opts.ConnectivityCheck,
		lastCompleted:     make(map[string]bool),
		lastProgress:      make(map[string]stalledState),
		stalledEmitted:    make(map[string]bool),
	}

	m.logger.Info("monitor initialized", "interval", opts.CheckInterval,
		"stall_detection", opts.CheckForStalled, "stall_timeout", opts.StallTimeout,
		"no_start_timeout", opts.NoStartTimeout)
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
					item.ContentPath, item.SavePath,
				)
			}
		}
	}

	// Process results: detect stalled/failed downloads.
	if m.checkForStalled {
		erroredClients := make(map[string]bool, len(status.Errors))
		for clientID := range status.Errors {
			erroredClients[clientID] = true
		}
		m.detectStalledSweep(ctx, status.Items, erroredClients)
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
			// Workflow delivery is idempotent and must not be suppressed by
			// download history: a prior command may have been dropped.
			if m.orchNotifier != nil {
				m.orchNotifier.NotifyDownloadComplete(item.ClientID, item.ID, item.Title, item.Category, item.ContentPath, item.SavePath)
			}
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
					ContentPath: item.ContentPath,
					SavePath:    item.SavePath,
					CompletedAt: m.clock.Now(),
				}
				_ = m.bus.Publish(ctx, event)

				durationSeconds := 0.0
				if state, ok := m.lastProgress[key]; ok && !state.firstSeenAt.IsZero() {
					durationSeconds = m.clock.Now().Sub(state.firstSeenAt).Seconds()
				}
				telemetry.ObserveDownloadCompleted(item.ClientID, durationSeconds)

				// Persist to history store if available.
				if m.historyStore != nil {
					if err := m.historyStore.RecordCompletion(ctx, event); err != nil {
						m.logger.Warn("monitor: failed to record history",
							"item_id", item.ID, "error", err)
					}
				}

				m.logger.Debug("monitor: emitted DownloadCompleted",
					"item_id", item.ID, "client_id", item.ClientID,
					"title", item.Title, "status", string(item.Status))
			}
			thisRun[key] = true
		}
	}

	// Update lastCompleted for the next sweep.
	m.lastCompleted = thisRun
}

// detectStalled checks items for stall conditions and failed status,
// assuming every client responded.
func (m *Monitor) detectStalled(ctx context.Context, items []Item) {
	m.detectStalledSweep(ctx, items, nil)
}

// maxSweepGapFactor caps how much stall time a single sweep may add, so a
// suspended host or a paused monitor doesn't instantly "stall" everything.
const maxSweepGapFactor = 5

// detectStalledSweep checks items for stall conditions and failed status.
// erroredClients lists clients whose status could not be fetched this sweep;
// their tracking state is preserved rather than pruned.
//
// Stall time accrues only while the network is healthy: if nothing is moving
// and the connectivity check fails, the outage is not counted. Downloads that
// have never received data are held to NoStartTimeout instead of being exempt.
func (m *Monitor) detectStalledSweep(ctx context.Context, items []Item, erroredClients map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	var elapsed time.Duration
	if !m.lastSweepAt.IsZero() {
		elapsed = now.Sub(m.lastSweepAt)
		maxGap := maxSweepGapFactor * m.checkInterval
		if maxGap < 5*time.Minute {
			maxGap = 5 * time.Minute
		}
		if elapsed > maxGap {
			elapsed = maxGap
		}
		if elapsed < 0 {
			elapsed = 0
		}
	}
	m.lastSweepAt = now

	healthyKnown, healthy := false, false
	networkHealthy := func() bool {
		if healthyKnown {
			return healthy
		}
		healthyKnown = true
		for _, it := range items {
			if it.DownloadRate > 0 || it.UploadRate > 0 {
				healthy = true
				return healthy
			}
		}
		if m.connectivityCheck == nil {
			healthy = true
			return healthy
		}
		healthy = m.connectivityCheck(ctx)
		if !healthy {
			m.logger.Info("monitor: network appears down, pausing stall timers")
		}
		return healthy
	}

	activeIDs := make(map[string]bool, len(items))

	for _, item := range items {
		key := item.ClientID + ":" + item.ID
		activeIDs[key] = true

		// Handle failed items immediately.
		if item.Status == StatusItemFailed {
			if m.historyStore != nil && !m.historyStore.WasRecorded(ctx, item.ClientID, item.ID, "failed") {
				if err := m.historyStore.RecordFailure(ctx, item.ClientID, item.ID, item.Title, item.Category, now); err != nil {
					m.logger.Warn("monitor: failed to record failed history entry",
						"item_id", item.ID, "client_id", item.ClientID, "error", err)
				}
			}
			if !m.stalledEmitted[key] {
				m.stalledEmitted[key] = true
				msg := item.Message
				if msg == "" {
					msg = "reported failed by client"
				}
				m.handleStalled(ctx, item, "download failed: "+msg)
			}
			continue
		}

		prev, tracked := m.lastProgress[key]
		if !tracked {
			m.lastProgress[key] = stalledState{
				progress:        item.Progress,
				downloadedBytes: item.DownloadedBytes,
				firstSeenAt:     now,
				lastProgressAt:  now,
			}
			continue
		}
		state := prev

		// Only downloading items can stall. Queued/paused/seeding items keep
		// their tracking but accrue no stall time.
		if item.Status != StatusItemDownloading {
			state.progress = item.Progress
			state.downloadedBytes = item.DownloadedBytes
			state.lastProgressAt = now
			state.stalledFor = 0
			m.lastProgress[key] = state
			delete(m.stalledEmitted, key)
			continue
		}

		if item.Progress != prev.progress || item.DownloadedBytes != prev.downloadedBytes || item.DownloadRate > 0 {
			state.progress = item.Progress
			state.downloadedBytes = item.DownloadedBytes
			state.lastProgressAt = now
			state.stalledFor = 0
			m.lastProgress[key] = state
			delete(m.stalledEmitted, key)
			continue
		}

		if elapsed > 0 && networkHealthy() {
			state.stalledFor += elapsed
		}
		m.lastProgress[key] = state

		started := item.DownloadedBytes > 0 || item.Progress > 0
		timeout := m.stallTimeout
		if !started {
			timeout = m.noStartTimeout
		}
		if state.stalledFor < timeout || m.stalledEmitted[key] {
			continue
		}

		m.stalledEmitted[key] = true
		var reason string
		if started {
			reason = fmt.Sprintf("no progress for %s", state.stalledFor.Truncate(time.Second))
		} else {
			reason = fmt.Sprintf("download never started after %s", state.stalledFor.Truncate(time.Second))
		}
		m.logger.Warn("monitor: download stalled",
			"item_id", item.ID,
			"client_id", item.ClientID,
			"title", item.Title,
			"stalled_for", state.stalledFor.String(),
			"progress", item.Progress,
			"started", started,
		)
		m.handleStalled(ctx, item, reason)
	}

	// Prune tracking maps for items no longer present. Items on clients that
	// failed to respond this sweep are kept (the client may be briefly down).
	for key := range m.lastProgress {
		if activeIDs[key] {
			continue
		}
		if clientID, _, ok := strings.Cut(key, ":"); ok && erroredClients[clientID] {
			continue
		}
		delete(m.lastProgress, key)
		delete(m.stalledEmitted, key)
	}
}

// handleStalled runs the stall handler and, if the download was abandoned
// (removed from the client), tells the orchestrator so the workflow fails
// and a replacement is searched for.
func (m *Monitor) handleStalled(ctx context.Context, item Item, reason string) {
	if m.stallHandler == nil {
		return
	}
	if abandoned := m.stallHandler.Handle(ctx, item, reason); abandoned && m.orchNotifier != nil {
		m.orchNotifier.NotifyDownloadFailed(item.ClientID, item.ID, reason)
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
