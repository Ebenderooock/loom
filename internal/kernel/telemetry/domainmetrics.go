package telemetry

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	domainMetricsOnce sync.Once
	domainMetrics     struct {
		downloadLifecycle *prometheus.CounterVec
		downloadDuration  *prometheus.HistogramVec

		workflowTransitions *prometheus.CounterVec
		workflowRetries     *prometheus.CounterVec
		workflowActive      *prometheus.GaugeVec
		workflowStale       *prometheus.CounterVec

		schedulerRuns     *prometheus.CounterVec
		schedulerDuration *prometheus.HistogramVec
		schedulerInFlight *prometheus.GaugeVec

		rssSyncTotal    *prometheus.CounterVec
		rssSyncDuration *prometheus.HistogramVec
		rssItemsTotal   *prometheus.CounterVec
	}
)

func registerDomainMetrics() {
	domainMetricsOnce.Do(func() {
		// PromQL:
		//   sum by(stage, client_id) (rate(loom_download_lifecycle_total[5m]))
		// Alert suggestion:
		//   rate(loom_download_lifecycle_total{stage="failed"}[10m]) > 0.2
		domainMetrics.downloadLifecycle = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_download_lifecycle_total",
			Help: "Total download lifecycle events.",
		}, []string{"stage", "client_id", "reason"})

		// PromQL:
		//   histogram_quantile(0.95, sum by(le, client_id) (rate(loom_download_completion_seconds_bucket[15m])))
		// Alert suggestion:
		//   histogram_quantile(0.95, sum by(le) (rate(loom_download_completion_seconds_bucket[30m]))) > 7200
		domainMetrics.downloadDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "loom_download_completion_seconds",
			Help:    "Observed time from first-seen to completion for downloads.",
			Buckets: prometheus.ExponentialBuckets(30, 2, 10),
		}, []string{"client_id"})

		// PromQL:
		//   sum by(from_state, to_state) (rate(loom_workflow_transitions_total[10m]))
		// Alert suggestion:
		//   rate(loom_workflow_transitions_total{to_state="failed"}[10m]) > 0.1
		domainMetrics.workflowTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_workflow_transitions_total",
			Help: "Total workflow state transitions.",
		}, []string{"from_state", "to_state"})

		// PromQL:
		//   sum by(state, outcome) (rate(loom_workflow_retries_total[10m]))
		// Alert suggestion:
		//   rate(loom_workflow_retries_total{outcome="exhausted"}[15m]) > 0
		domainMetrics.workflowRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_workflow_retries_total",
			Help: "Total workflow retry outcomes.",
		}, []string{"state", "outcome"})

		// PromQL:
		//   sum by(state) (loom_workflows_active)
		// Alert suggestion:
		//   sum(loom_workflows_active{state="searching"}) > 200
		domainMetrics.workflowActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "loom_workflows_active",
			Help: "Current number of active workflows by state.",
		}, []string{"state"})

		// PromQL:
		//   sum by(state, action) (rate(loom_workflow_stale_total[10m]))
		// Alert suggestion:
		//   rate(loom_workflow_stale_total{action="failed"}[15m]) > 0
		domainMetrics.workflowStale = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_workflow_stale_total",
			Help: "Total stale workflow detections and outcomes.",
		}, []string{"state", "action"})

		// PromQL:
		//   sum by(job, status) (rate(loom_scheduler_runs_total[5m]))
		// Alert suggestion:
		//   rate(loom_scheduler_runs_total{status="failed"}[10m]) > 0
		domainMetrics.schedulerRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_scheduler_runs_total",
			Help: "Total scheduler job runs by status.",
		}, []string{"job", "status"})

		// PromQL:
		//   histogram_quantile(0.95, sum by(le, job) (rate(loom_scheduler_duration_seconds_bucket[15m])))
		// Alert suggestion:
		//   histogram_quantile(0.95, sum by(le) (rate(loom_scheduler_duration_seconds_bucket[30m]))) > 300
		domainMetrics.schedulerDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "loom_scheduler_duration_seconds",
			Help:    "Scheduler job run duration in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 10),
		}, []string{"job", "status"})

		// PromQL:
		//   sum by(job) (loom_scheduler_in_flight)
		// Alert suggestion:
		//   loom_scheduler_in_flight > 0 for 30m
		domainMetrics.schedulerInFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "loom_scheduler_in_flight",
			Help: "Number of in-flight scheduler job executions.",
		}, []string{"job"})

		// PromQL:
		//   sum by(source_id, outcome) (rate(loom_rss_sync_total[10m]))
		// Alert suggestion:
		//   rate(loom_rss_sync_total{outcome="failed"}[15m]) > 0
		domainMetrics.rssSyncTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_rss_sync_total",
			Help: "Total RSS source sync attempts by outcome.",
		}, []string{"source_id", "outcome"})

		// PromQL:
		//   histogram_quantile(0.95, sum by(le, source_id) (rate(loom_rss_sync_duration_seconds_bucket[30m])))
		// Alert suggestion:
		//   histogram_quantile(0.95, sum by(le) (rate(loom_rss_sync_duration_seconds_bucket[30m]))) > 60
		domainMetrics.rssSyncDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "loom_rss_sync_duration_seconds",
			Help:    "RSS source sync duration in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 12),
		}, []string{"source_id", "outcome"})

		// PromQL:
		//   sum by(source_id, result) (rate(loom_rss_items_total[10m]))
		// Alert suggestion:
		//   rate(loom_rss_items_total{result="stored"}[30m]) == 0
		domainMetrics.rssItemsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "loom_rss_items_total",
			Help: "Total RSS items processed by source and result.",
		}, []string{"source_id", "result"})

		reg := prometheus.DefaultRegisterer
		if t := Default(); t != nil && t.Registry() != nil {
			reg = t.Registry()
		}
		defer func() { _ = recover() }()
		reg.MustRegister(
			domainMetrics.downloadLifecycle,
			domainMetrics.downloadDuration,
			domainMetrics.workflowTransitions,
			domainMetrics.workflowRetries,
			domainMetrics.workflowActive,
			domainMetrics.workflowStale,
			domainMetrics.schedulerRuns,
			domainMetrics.schedulerDuration,
			domainMetrics.schedulerInFlight,
			domainMetrics.rssSyncTotal,
			domainMetrics.rssSyncDuration,
			domainMetrics.rssItemsTotal,
		)
	})
}

func boundedLabel(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

// ObserveDownloadQueued records a successful queue event for a download client.
func ObserveDownloadQueued(clientID string) {
	registerDomainMetrics()
	domainMetrics.downloadLifecycle.WithLabelValues("queued", boundedLabel(clientID), "none").Inc()
}

// ObserveDownloadFailed records a failed queue/routing attempt.
func ObserveDownloadFailed(clientID, reason string) {
	registerDomainMetrics()
	domainMetrics.downloadLifecycle.WithLabelValues("failed", boundedLabel(clientID), boundedLabel(reason)).Inc()
}

// ObserveDownloadCompleted records a completion event and optional latency.
func ObserveDownloadCompleted(clientID string, durationSeconds float64) {
	registerDomainMetrics()
	client := boundedLabel(clientID)
	domainMetrics.downloadLifecycle.WithLabelValues("completed", client, "none").Inc()
	if durationSeconds > 0 {
		domainMetrics.downloadDuration.WithLabelValues(client).Observe(durationSeconds)
	}
}

// ObserveWorkflowTransition records a workflow state transition.
func ObserveWorkflowTransition(fromState, toState string) {
	registerDomainMetrics()
	domainMetrics.workflowTransitions.WithLabelValues(boundedLabel(fromState), boundedLabel(toState)).Inc()
}

// ObserveWorkflowRetry records a workflow retry outcome.
func ObserveWorkflowRetry(state, outcome string) {
	registerDomainMetrics()
	domainMetrics.workflowRetries.WithLabelValues(boundedLabel(state), boundedLabel(outcome)).Inc()
}

// SetWorkflowActiveByState sets active workflow gauges by state.
func SetWorkflowActiveByState(counts map[string]int) {
	registerDomainMetrics()
	for _, state := range []string{"searching", "grabbed", "downloading", "post_download", "importing", "cleaning_up"} {
		domainMetrics.workflowActive.WithLabelValues(state).Set(float64(counts[state]))
	}
}

// ObserveWorkflowStale records stale workflow detection outcomes.
func ObserveWorkflowStale(state, action string) {
	registerDomainMetrics()
	domainMetrics.workflowStale.WithLabelValues(boundedLabel(state), boundedLabel(action)).Inc()
}

// ObserveSchedulerStart increments the in-flight gauge for a scheduler job.
func ObserveSchedulerStart(job string) {
	registerDomainMetrics()
	domainMetrics.schedulerInFlight.WithLabelValues(boundedLabel(job)).Inc()
}

// ObserveSchedulerFinish records scheduler completion status and duration.
func ObserveSchedulerFinish(job, status string, durationSeconds float64) {
	registerDomainMetrics()
	jobLabel := boundedLabel(job)
	statusLabel := boundedLabel(status)
	domainMetrics.schedulerInFlight.WithLabelValues(jobLabel).Dec()
	domainMetrics.schedulerRuns.WithLabelValues(jobLabel, statusLabel).Inc()
	if durationSeconds > 0 {
		domainMetrics.schedulerDuration.WithLabelValues(jobLabel, statusLabel).Observe(durationSeconds)
	}
}

// ObserveRSSSourceSync records RSS sync outcome, duration, and item counters.
func ObserveRSSSourceSync(sourceID, outcome string, durationSeconds float64, stored, deduped int64) {
	registerDomainMetrics()
	source := boundedLabel(sourceID)
	outcomeLabel := boundedLabel(outcome)
	domainMetrics.rssSyncTotal.WithLabelValues(source, outcomeLabel).Inc()
	if durationSeconds >= 0 {
		domainMetrics.rssSyncDuration.WithLabelValues(source, outcomeLabel).Observe(durationSeconds)
	}
	if stored > 0 {
		domainMetrics.rssItemsTotal.WithLabelValues(source, "stored").Add(float64(stored))
	}
	if deduped > 0 {
		domainMetrics.rssItemsTotal.WithLabelValues(source, "deduped").Add(float64(deduped))
	}
}
