package analytics

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Poller periodically samples active sessions. It is gated by an enabled
// function (typically a feature flag) so admins can pause sampling without
// losing existing history.
type Poller struct {
	svc      *Service
	interval time.Duration
	enabled  func() bool
	logger   *slog.Logger

	mu       sync.Mutex
	cancel   context.CancelFunc
	running  bool
	sampling bool
}

// NewPoller creates an analytics poller. enabled may be nil (always on).
func NewPoller(svc *Service, interval time.Duration, enabled func() bool, logger *slog.Logger) *Poller {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Poller{
		svc:      svc,
		interval: interval,
		enabled:  enabled,
		logger:   logger.With("module", "analytics-poller"),
	}
}

// Start begins the polling loop after clearing startup orphans.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	childCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running = true
	p.mu.Unlock()

	p.svc.ResetOrphans(childCtx)
	go p.loop(childCtx)
	p.logger.Info("analytics poller started", "interval", p.interval)
}

// Stop halts the polling loop.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
	p.running = false
}

func (p *Poller) loop(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	if p.enabled != nil && !p.enabled() {
		return
	}
	// Skip overlapping samples if the previous one is still running.
	p.mu.Lock()
	if p.sampling {
		p.mu.Unlock()
		p.logger.Debug("analytics: skipping tick, previous sample still running")
		return
	}
	p.sampling = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.sampling = false
		p.mu.Unlock()
	}()

	p.svc.Sample(ctx, p.interval)
}
