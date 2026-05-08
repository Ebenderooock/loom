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
	MoviesTotal   prometheus.Gauge
	SeriesTotal   prometheus.Gauge
	EpisodesTotal prometheus.Gauge

	LibrariesTotal   prometheus.Gauge
	LibrarySizeBytes *prometheus.GaugeVec

	IndexersConfigured prometheus.Gauge
	IndexersHealthy    prometheus.Gauge

	ScanTotal          *prometheus.CounterVec
	ScanDuration       *prometheus.HistogramVec
	ScanFilesProcessed *prometheus.CounterVec

	DownloadsActive        prometheus.Gauge
	DownloadClientsTotal   prometheus.Gauge
	DownloadClientsHealthy prometheus.Gauge
	DownloadQueueSize      prometheus.Gauge
	DownloadSpeedBytes     prometheus.Gauge

	QualityProfilesTotal prometheus.Gauge

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
		EpisodesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_episodes_total",
			Help: "Current total episodes tracked.",
		}),
		LibrariesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_libraries_total",
			Help: "Number of configured libraries.",
		}),
		LibrarySizeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "loom_library_size_bytes",
			Help: "Size of each library in bytes.",
		}, []string{"library_name"}),
		IndexersConfigured: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_indexers_configured",
			Help: "Number of configured indexers.",
		}),
		IndexersHealthy: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_indexers_healthy",
			Help: "Number of indexers currently reporting healthy.",
		}),
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
		DownloadsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_downloads_active",
			Help: "Number of currently active downloads.",
		}),
		DownloadClientsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_download_clients_total",
			Help: "Configured download clients.",
		}),
		DownloadClientsHealthy: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_download_clients_healthy",
			Help: "Healthy download clients.",
		}),
		DownloadQueueSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_download_queue_size",
			Help: "Items in download queue.",
		}),
		DownloadSpeedBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_download_speed_bytes",
			Help: "Current download speed in bytes/sec.",
		}),
		QualityProfilesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "loom_quality_profiles_total",
			Help: "Number of quality profiles.",
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
		m.EpisodesTotal,
		m.LibrariesTotal,
		m.LibrarySizeBytes,
		m.IndexersConfigured,
		m.IndexersHealthy,
		m.ScanTotal,
		m.ScanDuration,
		m.ScanFilesProcessed,
		m.DownloadsActive,
		m.DownloadClientsTotal,
		m.DownloadClientsHealthy,
		m.DownloadQueueSize,
		m.DownloadSpeedBytes,
		m.QualityProfilesTotal,
		m.SearchRequests,
		m.ImportTotal,
		m.NotifSent,
	)
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
