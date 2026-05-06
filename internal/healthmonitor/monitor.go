// Package healthmonitor runs periodic health checks against library disk
// space, download clients, and indexers, sending notifications when
// problems are detected.
package healthmonitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CheckResult represents a single health check result.
type CheckResult struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // "ok", "warning", "critical"
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

// IndexerChecker is the subset of the indexer service needed for health checks.
type IndexerChecker interface {
	List(ctx context.Context) ([]IndexerInfo, error)
}

// IndexerInfo is a minimal indexer representation for health checks.
type IndexerInfo struct {
	ID      string
	Name    string
	Enabled bool
	Status  string // "ok", "degraded", "failed", "unknown"
}

// DownloadChecker tests download client connectivity.
type DownloadChecker interface {
	ListClients(ctx context.Context) ([]ClientInfo, error)
}

// ClientInfo is a minimal download client representation for health checks.
type ClientInfo struct {
	ID      string
	Name    string
	Enabled bool
	Status  string // "ok", "degraded", "failed", "unknown"
}

// NotificationSender sends alert notifications.
type NotificationSender func(ctx context.Context, title, body string) error

// Options configures a Monitor.
type Options struct {
	Indexers  IndexerChecker
	Downloads DownloadChecker
	Notifier  NotificationSender
	LibPaths  []string
	Logger    *slog.Logger
	Interval  time.Duration // default: 5 minutes
	Cooldown  time.Duration // default: 1 hour
}

// Monitor runs periodic health checks and sends notifications.
type Monitor struct {
	indexerSvc  IndexerChecker
	downloadSvc DownloadChecker
	notifier    NotificationSender
	libPaths    []string
	logger      *slog.Logger
	interval    time.Duration
	cooldown    time.Duration
	cancel      context.CancelFunc

	mu         sync.Mutex
	lastAlerts map[string]time.Time
	lastResults []CheckResult
}

// New creates a Monitor with the given options.
func New(opts Options) *Monitor {
	interval := opts.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	cooldown := opts.Cooldown
	if cooldown <= 0 {
		cooldown = 1 * time.Hour
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Monitor{
		indexerSvc:  opts.Indexers,
		downloadSvc: opts.Downloads,
		notifier:    opts.Notifier,
		libPaths:    opts.LibPaths,
		logger:      logger,
		interval:    interval,
		cooldown:    cooldown,
		lastAlerts:  make(map[string]time.Time),
	}
}

// Start launches the periodic health-check goroutine. It blocks until
// ctx is cancelled or Stop is called.
func (m *Monitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	go m.loop(ctx)
}

// Stop cancels the background loop.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Monitor) loop(ctx context.Context) {
	// Run an initial check shortly after startup.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			results := m.RunChecks(ctx)
			m.processAlerts(ctx, results)
			timer.Reset(m.interval)
		}
	}
}

// RunChecks executes all health checks and returns the results. Safe
// for concurrent use — the API handler calls this directly.
func (m *Monitor) RunChecks(ctx context.Context) []CheckResult {
	var results []CheckResult
	results = append(results, m.checkDiskSpace(ctx)...)
	results = append(results, m.checkDownloadClients(ctx)...)
	results = append(results, m.checkIndexers(ctx)...)

	m.mu.Lock()
	m.lastResults = results
	m.mu.Unlock()

	return results
}

// LastResults returns the most recent check results without re-running.
func (m *Monitor) LastResults() []CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CheckResult, len(m.lastResults))
	copy(out, m.lastResults)
	return out
}

func (m *Monitor) processAlerts(ctx context.Context, results []CheckResult) {
	if m.notifier == nil {
		return
	}
	for _, r := range results {
		if r.Status == "ok" {
			continue
		}
		if !m.shouldAlert(r.Name) {
			continue
		}
		title := fmt.Sprintf("Health %s: %s", r.Status, r.Name)
		if err := m.notifier(ctx, title, r.Message); err != nil {
			m.logger.Warn("failed to send health alert", "check", r.Name, "err", err)
		} else {
			m.recordAlert(r.Name)
			m.logger.Info("health alert sent", "check", r.Name, "status", r.Status)
		}
	}
}

func (m *Monitor) shouldAlert(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.lastAlerts[name]; ok {
		return time.Since(last) >= m.cooldown
	}
	return true
}

func (m *Monitor) recordAlert(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAlerts[name] = time.Now()
}

// --- Disk space check ---

func (m *Monitor) checkDiskSpace(_ context.Context) []CheckResult {
	var results []CheckResult
	for _, path := range m.libPaths {
		_, freeGB, err := checkDiskSpace(path)
		name := "disk:" + path
		now := time.Now()
		if err != nil {
			results = append(results, CheckResult{
				Name:      name,
				Status:    "warning",
				Message:   fmt.Sprintf("Could not check disk space for %s: %v", path, err),
				CheckedAt: now,
			})
			continue
		}
		switch {
		case freeGB < 2:
			results = append(results, CheckResult{
				Name:      name,
				Status:    "critical",
				Message:   fmt.Sprintf("Critically low disk space on %s: %.1f GB free", path, freeGB),
				CheckedAt: now,
			})
		case freeGB < 10:
			results = append(results, CheckResult{
				Name:      name,
				Status:    "warning",
				Message:   fmt.Sprintf("Low disk space on %s: %.1f GB free", path, freeGB),
				CheckedAt: now,
			})
		default:
			results = append(results, CheckResult{
				Name:      name,
				Status:    "ok",
				Message:   fmt.Sprintf("%.1f GB free on %s", freeGB, path),
				CheckedAt: now,
			})
		}
	}
	return results
}

// --- Download client check ---

func (m *Monitor) checkDownloadClients(ctx context.Context) []CheckResult {
	if m.downloadSvc == nil {
		return nil
	}
	clients, err := m.downloadSvc.ListClients(ctx)
	if err != nil {
		return []CheckResult{{
			Name:      "download-clients",
			Status:    "warning",
			Message:   fmt.Sprintf("Failed to list download clients: %v", err),
			CheckedAt: time.Now(),
		}}
	}
	var results []CheckResult
	for _, c := range clients {
		if !c.Enabled {
			continue
		}
		name := "download:" + c.Name
		now := time.Now()
		switch c.Status {
		case "ok":
			results = append(results, CheckResult{
				Name: name, Status: "ok",
				Message: c.Name + " is reachable", CheckedAt: now,
			})
		case "degraded":
			results = append(results, CheckResult{
				Name: name, Status: "warning",
				Message: c.Name + " is degraded", CheckedAt: now,
			})
		default:
			results = append(results, CheckResult{
				Name: name, Status: "critical",
				Message: c.Name + " is unreachable", CheckedAt: now,
			})
		}
	}
	return results
}

// --- Indexer check ---

func (m *Monitor) checkIndexers(ctx context.Context) []CheckResult {
	if m.indexerSvc == nil {
		return nil
	}
	indexers, err := m.indexerSvc.List(ctx)
	if err != nil {
		return []CheckResult{{
			Name:      "indexers",
			Status:    "warning",
			Message:   fmt.Sprintf("Failed to list indexers: %v", err),
			CheckedAt: time.Now(),
		}}
	}
	var enabledCount, failedCount int
	var failedNames []string
	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}
		enabledCount++
		if idx.Status == "failed" {
			failedCount++
			failedNames = append(failedNames, idx.Name)
		}
	}
	now := time.Now()
	if enabledCount == 0 {
		return []CheckResult{{
			Name: "indexers", Status: "warning",
			Message: "No indexers are enabled", CheckedAt: now,
		}}
	}
	if failedCount == enabledCount {
		return []CheckResult{{
			Name: "indexers", Status: "critical",
			Message: fmt.Sprintf("All %d enabled indexers are failing: %v", failedCount, failedNames),
			CheckedAt: now,
		}}
	}
	if failedCount > 0 {
		return []CheckResult{{
			Name: "indexers", Status: "warning",
			Message: fmt.Sprintf("%d of %d indexers failing: %v", failedCount, enabledCount, failedNames),
			CheckedAt: now,
		}}
	}
	return []CheckResult{{
		Name: "indexers", Status: "ok",
		Message: fmt.Sprintf("All %d enabled indexers are healthy", enabledCount),
		CheckedAt: now,
	}}
}
