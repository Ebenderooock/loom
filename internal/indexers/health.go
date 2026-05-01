package indexers

import (
	"context"
	"sync"
	"time"
)

// HealthCheckJobName is the scheduler key under which the periodic
// indexer.Test sweep is registered. Exported so tests and admin
// tooling can refer to it without stringly-typed lookups.
const HealthCheckJobName = "indexers.health"

// HealthChecker drives the periodic Test sweep across every
// registered indexer. It is constructed with an upper bound on
// parallelism so a slow source cannot starve healthy ones.
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
		panic("indexers: NewHealthChecker with nil Service")
	}
	if maxParallel <= 0 {
		maxParallel = svc.maxParallel
	}
	if timeout <= 0 {
		timeout = svc.healthCheckTimeout
	}
	return &HealthChecker{svc: svc, maxParallel: maxParallel, timeout: timeout}
}

// Run is the scheduler.HandlerFunc entry point. It runs Test against
// every registered indexer, persists the outcome through the Service,
// and returns nil unless the context is canceled.
//
// Individual indexer failures do NOT propagate as a job error — they
// are recorded as health rows. Returning nil keeps the scheduler from
// marking the periodic job itself as failed when one downstream
// source is sick.
func (h *HealthChecker) Run(ctx context.Context) error {
	indexers := h.svc.registry.List()
	if len(indexers) == 0 {
		return nil
	}

	limit := h.maxParallel
	if limit <= 0 || limit > len(indexers) {
		limit = len(indexers)
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup

	for _, ix := range indexers {
		ix := ix
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
			h.checkOne(ctx, ix.ID())
		}()
	}
	wg.Wait()
	return nil
}

func (h *HealthChecker) checkOne(ctx context.Context, id string) {
	// TestOne already persists and reports; we only need to swallow
	// the per-indexer error here.
	if _, err := h.svc.TestOne(ctx, id); err != nil {
		h.svc.logger.Debug("health check failed", "id", id, "err", err)
	}
}
