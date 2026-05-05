package telemetry

import "github.com/prometheus/client_golang/prometheus"

// AppMetrics holds domain-specific gauges and counters for Loom
// application telemetry. Create one via NewAppMetrics and update the
// fields from the relevant service layers. The metrics are registered
// with the canonical Prometheus registry so they appear on /metrics AND
// (when OTel metrics push is enabled) are also exported via OTLP.
type AppMetrics struct {
	MoviesTotal   prometheus.Gauge
	SeriesTotal   prometheus.Gauge

	IndexersConfigured prometheus.Gauge
	IndexersHealthy    prometheus.Gauge

	DownloadsActive prometheus.Gauge

	SearchRequests *prometheus.CounterVec
	ImportTotal    *prometheus.CounterVec
	NotifSent      *prometheus.CounterVec
}

// NewAppMetrics registers domain metrics with the given Prometheus
// registry and returns the handle. Callers should keep a reference and
// update gauges/counters as state changes.
func NewAppMetrics(reg *prometheus.Registry) *AppMetrics {
	m := &AppMetrics{
		MoviesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_movies_total",
			Help: "Current number of movies in the library.",
		}),
		SeriesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_series_total",
			Help: "Current number of series in the library.",
		}),
		IndexersConfigured: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_indexers_configured",
			Help: "Number of configured indexers.",
		}),
		IndexersHealthy: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_indexers_healthy",
			Help: "Number of indexers currently reporting healthy.",
		}),
		DownloadsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_downloads_active",
			Help: "Number of currently active downloads.",
		}),
		SearchRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_search_requests_total",
			Help: "Total search requests.",
		}, []string{"type"}),
		ImportTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_import_total",
			Help: "Total import operations.",
		}, []string{"status"}),
		NotifSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_notifications_sent_total",
			Help: "Total notifications sent.",
		}, []string{"provider", "status"}),
	}
	reg.MustRegister(
		m.MoviesTotal,
		m.SeriesTotal,
		m.IndexersConfigured,
		m.IndexersHealthy,
		m.DownloadsActive,
		m.SearchRequests,
		m.ImportTotal,
		m.NotifSent,
	)
	return m
}

var (
	appMetrics    *AppMetrics
)

// InitAppMetrics creates and stores the package-level AppMetrics
// singleton. Typically called once from server startup after the
// Telemetry instance has been initialised.
func InitAppMetrics(reg *prometheus.Registry) *AppMetrics {
	appMetrics = NewAppMetrics(reg)
	return appMetrics
}

// App returns the package-level AppMetrics, or nil if InitAppMetrics
// has not been called.
func App() *AppMetrics {
	return appMetrics
}
