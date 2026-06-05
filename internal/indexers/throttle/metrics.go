package throttle

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ebenderooock/loom/internal/kernel/telemetry"
)

// Metric names. Kept under the `loom_indexer_` namespace so dashboards
// can group rate-limit / retry / latency / proxy metrics together.
const (
	metricRequestTotal      = "loom_indexer_request_total"
	metricRequestDuration   = "loom_indexer_request_duration_seconds"
	metricRetriesTotal      = "loom_indexer_retries_total"
	metricRateLimitWaitSecs = "loom_indexer_ratelimit_wait_seconds"
)

// Outcome label values for loom_indexer_request_total.
const (
	OutcomeSuccess     = "success"      // 2xx final response
	OutcomeClientError = "client_error" // 4xx final response (excl. 429 → retry)
	OutcomeServerError = "server_error" // 5xx final response after retries exhausted
	OutcomeError       = "error"        // transport error after retries exhausted
)

var (
	registerOnce sync.Once

	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	retriesTotal    *prometheus.CounterVec
	rateLimitWait   *prometheus.HistogramVec
)

// register lazily creates the metric vectors and attaches them to the
// telemetry registry. We use sync.Once because tests construct several
// transports per process, and re-registering identical collectors
// against the same prometheus.Registerer panics.
func register() {
	registerOnce.Do(func() {
		requestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRequestTotal,
			Help: "Total outbound indexer HTTP requests (final outcome only — retries are counted separately).",
		}, []string{"indexer", "kind", "outcome"})

		requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRequestDuration,
			Help:    "Wall-clock duration of outbound indexer HTTP requests, including any rate-limit wait and retry sleeps.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 10),
		}, []string{"indexer", "kind"})

		retriesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRetriesTotal,
			Help: "Total retry attempts performed by the throttle transport, labelled by reason.",
		}, []string{"indexer", "reason"})

		rateLimitWait = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRateLimitWaitSecs,
			Help:    "Time spent blocked on the per-indexer token bucket before a request was admitted.",
			Buckets: prometheus.ExponentialBuckets(0.001, 4, 8),
		}, []string{"indexer"})

		// Register against the telemetry registry if available.
		// During unit tests the telemetry package may not be Init'd,
		// so we tolerate a nil Default and fall back to the default
		// prometheus registerer just so the collectors are usable.
		var reg = prometheus.DefaultRegisterer
		if t := telemetry.Default(); t != nil && t.Registry() != nil {
			reg = t.Registry()
		}
		// MustRegister panics on duplicate; recover so re-runs in
		// the same process (or test binaries) don't crash.
		defer func() { _ = recover() }()
		reg.MustRegister(requestTotal, requestDuration, retriesTotal, rateLimitWait)
	})
}

// observeRequest records the final outcome of a request, including
// any rate-limit + retry time accumulated by the transport.
func observeRequest(indexer, kind, outcome string, durationSeconds float64) {
	register()
	requestTotal.WithLabelValues(indexer, kind, outcome).Inc()
	requestDuration.WithLabelValues(indexer, kind).Observe(durationSeconds)
}

func observeRetry(indexer string, reason Reason) {
	register()
	retriesTotal.WithLabelValues(indexer, string(reason)).Inc()
}

func observeRateLimitWait(indexer string, seconds float64) {
	register()
	rateLimitWait.WithLabelValues(indexer).Observe(seconds)
}
