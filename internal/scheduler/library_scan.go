package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// LibraryScanner is an interface for triggering library scans.
type LibraryScanner interface {
	// StartSeriesScan triggers a scan of the given library.
	StartSeriesScan(ctx context.Context, libraryID, rootFolder string) (string, error)
}

// LibraryProvider returns the list of libraries to scan.
type LibraryProvider interface {
	ListAll(ctx context.Context) ([]LibraryInfo, error)
}

// LibraryInfo holds enough info to trigger a scan.
type LibraryInfo struct {
	ID        string
	Path      string
	MediaType string
}

// PeriodicScanner runs library scans on a configurable interval.
type PeriodicScanner struct {
	seriesScanner LibraryScanner
	libProvider   LibraryProvider
	logger        *slog.Logger

	mu        sync.Mutex
	interval  time.Duration
	cancel    context.CancelFunc
	running   bool
	scanning  bool // true while a scan cycle is in progress
	lastRun   *time.Time
	nextRun   *time.Time
}

// NewPeriodicScanner creates a periodic scanner that doesn't run until Start().
func NewPeriodicScanner(seriesScanner LibraryScanner, libProvider LibraryProvider, interval time.Duration, logger *slog.Logger) *PeriodicScanner {
	return &PeriodicScanner{
		seriesScanner: seriesScanner,
		libProvider:   libProvider,
		interval:      interval,
		logger:        logger.With("module", "periodic-scan"),
	}
}

// Start begins the periodic scan loop.
func (ps *PeriodicScanner) Start(ctx context.Context) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.running {
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	ps.cancel = cancel
	ps.running = true

	next := time.Now().Add(ps.interval)
	ps.nextRun = &next

	go ps.loop(childCtx)
	ps.logger.Info("periodic library scan started", "interval", ps.interval)
}

// Stop stops the periodic scanner.
func (ps *PeriodicScanner) Stop() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel != nil {
		ps.cancel()
	}
	ps.running = false
}

// Status returns the current state of the periodic scanner.
func (ps *PeriodicScanner) Status() map[string]interface{} {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	status := map[string]interface{}{
		"running":  ps.running,
		"interval": ps.interval.String(),
	}
	if ps.lastRun != nil {
		status["lastRun"] = ps.lastRun.Format(time.RFC3339)
	}
	if ps.nextRun != nil {
		status["nextRun"] = ps.nextRun.Format(time.RFC3339)
	}
	return status
}

func (ps *PeriodicScanner) loop(ctx context.Context) {
	ticker := time.NewTicker(ps.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ps.runScan(ctx)
		}
	}
}

func (ps *PeriodicScanner) runScan(ctx context.Context) {
	ps.mu.Lock()
	if ps.scanning {
		ps.mu.Unlock()
		ps.logger.Warn("periodic scan: skipping, previous scan still in progress")
		return
	}
	ps.scanning = true
	ps.mu.Unlock()

	defer func() {
		ps.mu.Lock()
		ps.scanning = false
		ps.mu.Unlock()
	}()

	libs, err := ps.libProvider.ListAll(ctx)
	if err != nil {
		ps.logger.Error("periodic scan: failed to list libraries", "err", err)
		return
	}

	now := time.Now()
	ps.mu.Lock()
	ps.lastRun = &now
	next := now.Add(ps.interval)
	ps.nextRun = &next
	ps.mu.Unlock()

	for _, lib := range libs {
		if lib.MediaType != "series" {
			ps.logger.Debug("periodic scan: skipping non-series library", "id", lib.ID, "type", lib.MediaType)
			continue
		}
		ps.logger.Info("periodic scan: scanning library", "id", lib.ID, "path", lib.Path)
		if _, err := ps.seriesScanner.StartSeriesScan(ctx, lib.ID, lib.Path); err != nil {
			ps.logger.Error("periodic scan: failed", "library", lib.ID, "err", err)
		}
	}
}
