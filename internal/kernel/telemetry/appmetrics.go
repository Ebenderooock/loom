package telemetry

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// AppMetrics holds domain-specific gauges and counters for Loom
// application telemetry. Create one via NewAppMetrics and update the
// fields from the relevant service layers. The metrics are registered
// with the canonical Prometheus registry so they appear on /metrics AND
// (when OTel metrics push is enabled) are also exported via OTLP.
type AppMetrics struct {
	ScanTotal          *prometheus.CounterVec
	ScanDuration       *prometheus.HistogramVec
	ScanFilesProcessed *prometheus.CounterVec
}

// NewAppMetrics registers domain metrics with the given Prometheus
// registry and returns the handle. Callers should keep a reference and
// update gauges/counters as state changes.
func NewAppMetrics(reg *prometheus.Registry) *AppMetrics {
	m := &AppMetrics{
		ScanTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_scan_total",
			Help: "Total scan operations.",
		}, []string{"type", "status"}),
		ScanDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "loom_scan_duration_seconds",
			Help:    "Scan duration in seconds.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}, []string{"type"}),
		ScanFilesProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_scan_files_processed_total",
			Help: "Files processed during scans.",
		}, []string{"type", "result"}),
	}
	defer func() { _ = recover() }()
	reg.MustRegister(m.ScanTotal, m.ScanDuration, m.ScanFilesProcessed)
	return m
}

var (
	appMetrics     *AppMetrics
	appMetricsOnce sync.Once
)

// InitAppMetrics creates and stores the package-level AppMetrics
// singleton. Safe to call multiple times — only the first call has
// effect (protected by sync.Once).
func InitAppMetrics(reg *prometheus.Registry) *AppMetrics {
	appMetricsOnce.Do(func() {
		appMetrics = NewAppMetrics(reg)
	})
	return appMetrics
}

// App returns the package-level AppMetrics, or nil if InitAppMetrics
// has not been called.
func App() *AppMetrics {
	return appMetrics
}
