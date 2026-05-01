// Package telemetry is the seam where OpenTelemetry traces, metrics, and
// the Prometheus registry are wired up. The Phase-1 implementation is a
// no-op shell that exposes the public surface; SDK wiring lands as the
// observability deps are introduced.
package telemetry

import (
	"context"
	"net/http"

	"github.com/loomctl/loom/internal/kernel/config"
)

// Telemetry holds the lifecycle handles for trace/metric exporters and
// the Prometheus registry.
type Telemetry struct {
	cfg config.TelemetryConfig
}

// New returns a Telemetry with the OTel SDK and Prometheus registry
// configured per cfg. Currently a no-op shell.
func New(_ context.Context, cfg config.TelemetryConfig) (*Telemetry, error) {
	return &Telemetry{cfg: cfg}, nil
}

// Handler returns the http.Handler for /metrics. Returns nil when
// Prometheus is disabled.
func (t *Telemetry) Handler() http.Handler {
	if !t.cfg.Prometheus {
		return nil
	}
	// Replaced with promhttp.Handler() in Phase 1.
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# loom metrics — placeholder; Prometheus exporter wires up in Phase 1\n"))
	})
}

// Shutdown flushes exporters.
func (t *Telemetry) Shutdown(_ context.Context) error { return nil }
