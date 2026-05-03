package downloads

import (
	"context"
	"sync"
	"time"
)

// HealthCheckJobName is the scheduler key under which the periodic
// download-client Test sweep is registered.
const HealthCheckJobName = "downloads.health"

// HealthChecker drives the periodic Test sweep across every
// registered download client.
type HealthChecker struct {
	svc         *Service
	maxParallel int
	timeout     time.Duration
}

// NewHealthChecker returns a checker bound to svc.
//
// maxParallel <= 0 falls back to the Service's configured value;
// timeout <= 0 falls back to the Service's per-check timeout.
func NewHealthChecker(svc *Service, maxParallel int, timeout time.Duration) *HealthChecker {
	if svc == nil {
		panic("downloads: NewHealthChecker with nil Service")
	}
	if maxParallel <= 0 {
		maxParallel = svc.maxParallel
	}
	if timeout <= 0 {
		timeout = svc.healthTimeout
	}
	return &HealthChecker{svc: svc, maxParallel: maxParallel, timeout: timeout}
}

// Run is the scheduler.HandlerFunc entry point. It runs Test against
// every registered client and persists the outcome through the
// Service. Individual failures are recorded as health rows; returning
// nil keeps the scheduler from marking the job itself as failed when
// one client is sick.
func (h *HealthChecker) Run(ctx context.Context) error {
	clients := h.svc.registry.List()
	if len(clients) == 0 {
		return nil
	}

	limit := h.maxParallel
	if limit <= 0 || limit > len(clients) {
		limit = len(clients)
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup

	for _, c := range clients {
		c := c
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h.checkOne(ctx, c.ID())
		}()
	}
	wg.Wait()
	return nil
}

func (h *HealthChecker) checkOne(ctx context.Context, id string) {
	if _, err := h.svc.TestOne(ctx, id); err != nil {
		h.svc.logger.Debug("download health check failed", "id", id, "err", err)
	}
}
