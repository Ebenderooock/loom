package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/loomctl/loom/internal/indexers"
)

// RollingSearcher periodically searches indexers for missing library
// items in small batches, respecting per-indexer quota limits.
type RollingSearcher struct {
	store      *Store
	indexerSvc *indexers.Service
	quota      *QuotaTracker
	logger     *slog.Logger

	mu       sync.Mutex
	config   RollingSearchConfig
	cancel   context.CancelFunc
	running  bool
	lastRun  *time.Time
	nextRun  *time.Time
	searched int // items searched since boot
}

// NewRollingSearcher constructs a searcher but does not start it.
func NewRollingSearcher(store *Store, indexerSvc *indexers.Service, logger *slog.Logger, cfg RollingSearchConfig) *RollingSearcher {
	return &RollingSearcher{
		store:      store,
		indexerSvc: indexerSvc,
		quota:      NewQuotaTracker(cfg.MaxSearchesPerDay),
		logger:     logger.With("module", "rolling-search"),
		config:     cfg,
	}
}

// Start launches the background ticker goroutine. It is a no-op if the
// scheduler is already running or config is disabled.
func (rs *RollingSearcher) Start(ctx context.Context) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.running || !rs.config.Enabled {
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	rs.cancel = cancel
	rs.running = true
	next := time.Now().Add(rs.interval())
	rs.nextRun = &next

	go rs.loop(childCtx)
	rs.logger.Info("rolling search started", "interval_h", rs.config.IntervalHours, "batch", rs.config.BatchSize)
}

// Stop halts the background goroutine. Safe to call multiple times.
func (rs *RollingSearcher) Stop() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.cancel != nil {
		rs.cancel()
		rs.cancel = nil
	}
	rs.running = false
	rs.nextRun = nil
}

// Trigger runs one search cycle immediately in the caller's goroutine.
func (rs *RollingSearcher) Trigger(ctx context.Context) {
	rs.runOnce(ctx)
}

// Status returns a snapshot of the current scheduler state.
func (rs *RollingSearcher) Status(ctx context.Context) RollingSearchStatus {
	rs.mu.Lock()
	cfg := rs.config
	lastRun := rs.lastRun
	nextRun := rs.nextRun
	searched := rs.searched
	running := rs.running
	rs.mu.Unlock()

	queueSize, _ := rs.store.QueueSize(ctx, cfg.MinResearchDays)

	return RollingSearchStatus{
		Running:       running,
		LastRunAt:     lastRun,
		NextRunAt:     nextRun,
		ItemsSearched: searched,
		ItemsInQueue:  queueSize,
		QuotaUsage:    rs.quota.Usage(),
	}
}

// Config returns the current configuration.
func (rs *RollingSearcher) Config() RollingSearchConfig {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.config
}

// UpdateConfig applies a new configuration. If the enabled state
// changed the scheduler is started or stopped accordingly.
func (rs *RollingSearcher) UpdateConfig(ctx context.Context, cfg RollingSearchConfig) {
	rs.mu.Lock()
	wasEnabled := rs.config.Enabled
	rs.config = cfg
	rs.mu.Unlock()

	rs.quota.SetMax(cfg.MaxSearchesPerDay)

	if cfg.Enabled && !wasEnabled {
		rs.Start(ctx)
	} else if !cfg.Enabled && wasEnabled {
		rs.Stop()
	}
}

func (rs *RollingSearcher) interval() time.Duration {
	h := rs.config.IntervalHours
	if h <= 0 {
		h = 12
	}
	return time.Duration(h) * time.Hour
}

func (rs *RollingSearcher) loop(ctx context.Context) {
	ticker := time.NewTicker(rs.interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rs.runOnce(ctx)
			// Reset ticker with potentially updated interval.
			ticker.Reset(rs.interval())
			rs.mu.Lock()
			next := time.Now().Add(rs.interval())
			rs.nextRun = &next
			rs.mu.Unlock()
		}
	}
}

func (rs *RollingSearcher) runOnce(ctx context.Context) {
	rs.mu.Lock()
	cfg := rs.config
	rs.mu.Unlock()

	rs.logger.Info("rolling search run starting")
	now := time.Now()

	half := cfg.BatchSize / 2
	if half < 1 {
		half = 1
	}
	epHalf := cfg.BatchSize - half

	movieCandidates, err := rs.store.GetCandidates(ctx, "movie", half, cfg.MinResearchDays)
	if err != nil {
		rs.logger.Error("get movie candidates", "err", err)
	}
	epCandidates, err := rs.store.GetCandidates(ctx, "episode", epHalf, cfg.MinResearchDays)
	if err != nil {
		rs.logger.Error("get episode candidates", "err", err)
	}

	candidates := append(movieCandidates, epCandidates...)
	count := 0
	for _, c := range candidates {
		if ctx.Err() != nil {
			break
		}
		rs.searchCandidate(ctx, c)
		count++
	}

	rs.mu.Lock()
	rs.lastRun = &now
	rs.searched += count
	rs.mu.Unlock()

	rs.logger.Info("rolling search run complete", "searched", count)
}

func (rs *RollingSearcher) searchCandidate(ctx context.Context, c SearchCandidate) {
	if rs.indexerSvc == nil {
		return
	}

	// Build query based on media type.
	q := indexers.Query{
		Term: c.Title,
	}
	switch c.MediaType {
	case "movie":
		q.Categories = []indexers.Category{indexers.CategoryMovies}
	case "episode":
		q.Categories = []indexers.Category{indexers.CategoryTV}
	}

	// Fan out across all enabled indexers, respecting quotas.
	defs, err := rs.indexerSvc.List(ctx)
	if err != nil {
		rs.logger.Warn("list indexers", "err", err)
		return
	}

	var ids []string
	for _, dh := range defs {
		if dh.Definition.Enabled && rs.quota.CanSearch(dh.Definition.ID) {
			ids = append(ids, dh.Definition.ID)
		}
	}
	if len(ids) == 0 {
		rs.logger.Debug("no indexers available (all quota-exhausted or disabled)", "item", c.MediaID)
		return
	}

	rs.logger.Debug("searching", "type", c.MediaType, "id", c.MediaID, "title", c.Title, "indexers", len(ids))
	_ = rs.indexerSvc.Search(ctx, q, ids, 0)

	// Record quota usage.
	for _, id := range ids {
		rs.quota.RecordSearch(id)
	}

	// Persist last-searched timestamp.
	if err := rs.store.RecordSearch(ctx, c.MediaType, c.MediaID); err != nil {
		rs.logger.Warn("record search state", "err", err)
	}
}
