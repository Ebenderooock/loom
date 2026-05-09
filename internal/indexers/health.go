package indexers

import (
	"context"
	"sync"
	"time"
)

const maxBackoffInterval = 1 * time.Hour

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

	// Circuit breaker state: exponential backoff for consistently-failing indexers.
	mu                  sync.Mutex
	consecutiveFailures map[string]int
	nextCheckAt         map[string]time.Time
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
	return &HealthChecker{
		svc:                 svc,
		maxParallel:         maxParallel,
		timeout:             timeout,
		consecutiveFailures: make(map[string]int),
		nextCheckAt:         make(map[string]time.Time),
	}
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

		// Skip indexers whose backoff window has not elapsed yet.
		h.mu.Lock()
		if next, ok := h.nextCheckAt[ix.ID()]; ok && time.Now().Before(next) {
			h.mu.Unlock()
			continue
		}
		h.mu.Unlock()

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
	health, err := h.svc.TestOne(ctx, id)
	if err != nil {
		h.svc.logger.Warn("health check completed", "id", id, "status", health.Status, "latency_ms", health.LatencyMS, "err", err)

		h.mu.Lock()
		h.consecutiveFailures[id]++
		count := h.consecutiveFailures[id]
		backoff := h.timeout // use check timeout as base interval
		if backoff <= 0 {
			backoff = 30 * time.Second
		}
		shift := count
		if shift > 20 {
			shift = 20 // prevent integer overflow on 1<<shift
		}
		delay := backoff * time.Duration(1<<shift)
		if delay > maxBackoffInterval || delay <= 0 {
			delay = maxBackoffInterval
		}
		next := time.Now().Add(delay)
		h.nextCheckAt[id] = next
		h.mu.Unlock()

		h.svc.logger.Warn("indexer health check backing off",
			"indexer", id, "failures", count, "next_check", next)
	} else {
		h.svc.logger.Info("health check passed", "id", id, "status", health.Status, "latency_ms", health.LatencyMS)

		h.mu.Lock()
		delete(h.consecutiveFailures, id)
		delete(h.nextCheckAt, id)
		h.mu.Unlock()
	}
}
