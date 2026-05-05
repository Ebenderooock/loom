package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics records per-route HTTP metrics as both Prometheus collectors
// and (when OTel metrics are enabled) OTLP-pushed instruments. It
// implements chi-compatible middleware via its Handler method.
type HTTPMetrics struct {
	reqTotal    *prometheus.CounterVec
	reqDuration *prometheus.HistogramVec
	reqSize     *prometheus.HistogramVec
	respSize    *prometheus.HistogramVec
}

// NewHTTPMetrics creates HTTP metrics and registers them with the given
// Prometheus registry. The returned middleware is safe to use regardless
// of whether OTel metrics push is enabled — the Prometheus collectors
// always work.
func NewHTTPMetrics(reg *prometheus.Registry) *HTTPMetrics {
	m := &HTTPMetrics{
		reqTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "route", "status_code"}),
		reqDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		reqSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_size_bytes",
			Help:    "HTTP request body size in bytes.",
			Buckets: prometheus.ExponentialBuckets(100, 10, 6),
		}, []string{"method", "route"}),
		respSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "HTTP response body size in bytes.",
			Buckets: prometheus.ExponentialBuckets(100, 10, 6),
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.reqTotal, m.reqDuration, m.reqSize, m.respSize)
	return m
}

// Handler returns a chi-compatible middleware that records HTTP metrics.
// It uses chi's RouteContext to obtain the route pattern (avoiding
// cardinality explosion from raw paths).
func (m *HTTPMetrics) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		route := routePattern(r)
		method := r.Method
		status := strconv.Itoa(ww.status)
		elapsed := time.Since(start).Seconds()

		m.reqTotal.WithLabelValues(method, route, status).Inc()
		m.reqDuration.WithLabelValues(method, route).Observe(elapsed)
		m.reqSize.WithLabelValues(method, route).Observe(float64(r.ContentLength))
		m.respSize.WithLabelValues(method, route).Observe(float64(ww.written))
	})
}

// routePattern extracts the matched chi route pattern. Falls back to
// the raw path if no pattern is available yet (e.g. 404s).
func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx != nil {
		if pat := rctx.RoutePattern(); pat != "" {
			return pat
		}
	}
	return r.URL.Path
}

// responseWriter wraps http.ResponseWriter to capture status code and
// bytes written.
type responseWriter struct {
	http.ResponseWriter
	status      int
	written     int64
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

// Unwrap supports http.ResponseController and middleware that check for
// wrapped writers.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
