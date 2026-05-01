// Package telemetry wires Loom's observability stack:
//
//   - OpenTelemetry traces with an OTLP/HTTP exporter, gated by config.
//   - An always-on Prometheus registry exposed at /metrics with the
//     standard process + go runtime collectors registered.
//
// The package exposes both an instance API (server/Telemetry holds the
// returned value) and a small package-level singleton so the rest of the
// app can grab Tracer()/Meter() without threading the value everywhere.
package telemetry

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	mnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	tnoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/loomctl/loom/internal/buildinfo"
	"github.com/loomctl/loom/internal/kernel/config"
)

// Telemetry holds the lifecycle handles for trace exporters and the
// Prometheus registry.
type Telemetry struct {
	registry *prometheus.Registry
	tp       *sdktrace.TracerProvider
	tracer   trace.Tracer
	meter    metric.Meter
	otelOn   bool
}

var (
	defaultMu sync.RWMutex
	def       *Telemetry
)

// Init builds a Telemetry from cfg and stores it as the package default.
// It is safe to call multiple times (subsequent calls replace the
// default after shutting down the previous one).
func Init(ctx context.Context, cfg *config.Config) (*Telemetry, error) {
	t, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defaultMu.Lock()
	prev := def
	def = t
	defaultMu.Unlock()
	if prev != nil {
		_ = prev.Shutdown(ctx)
	}
	return t, nil
}

// New constructs a Telemetry. Server code should prefer Init, which also
// sets the package default.
func New(ctx context.Context, cfg *config.Config) (*Telemetry, error) {
	t := &Telemetry{registry: prometheus.NewRegistry()}
	t.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)

	// Default to no-op providers so callers can always grab a tracer.
	t.tracer = tnoop.NewTracerProvider().Tracer("github.com/loomctl/loom")
	t.meter = mnoop.NewMeterProvider().Meter("github.com/loomctl/loom")

	if cfg != nil && cfg.OTel.Enabled {
		if err := t.startOTel(ctx, cfg); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (t *Telemetry) startOTel(ctx context.Context, cfg *config.Config) error {
	opts := []otlptracehttp.Option{}
	if ep := cfg.OTel.Endpoint; ep != "" {
		opts = append(opts, otlptracehttp.WithEndpointURL(ep))
	}
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return err
	}
	res, _ := resource.Merge(resource.Default(), resource.NewSchemaless(
		semconv.ServiceName("loom"),
		semconv.ServiceVersion(buildinfo.Version),
	))
	ratio := cfg.Telemetry.TraceRatio
	if ratio <= 0 {
		ratio = 0
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	t.tp = tp
	t.tracer = tp.Tracer("github.com/loomctl/loom")
	t.otelOn = true
	return nil
}

// Handler returns the Prometheus /metrics http.Handler. Always non-nil.
func (t *Telemetry) Handler() http.Handler {
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		Registry:          t.registry,
	})
}

// Registry returns the Prometheus registry so other packages can register
// their own collectors against the canonical Loom registry.
func (t *Telemetry) Registry() *prometheus.Registry { return t.registry }

// Tracer returns this telemetry's tracer; never nil.
func (t *Telemetry) Tracer() trace.Tracer { return t.tracer }

// Meter returns this telemetry's meter; never nil.
func (t *Telemetry) Meter() metric.Meter { return t.meter }

// OTelEnabled reports whether the OTLP exporter is wired up.
func (t *Telemetry) OTelEnabled() bool { return t.otelOn }

// Shutdown flushes any exporters. Safe to call multiple times.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.tp == nil {
		return nil
	}
	err := t.tp.Shutdown(ctx)
	t.tp = nil
	return err
}

// Tracer returns the package-default tracer, or a no-op tracer if Init
// has not been called.
func Tracer() trace.Tracer {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if def == nil {
		return tnoop.NewTracerProvider().Tracer("github.com/loomctl/loom")
	}
	return def.tracer
}

// Meter returns the package-default meter, or a no-op meter if Init has
// not been called.
func Meter() metric.Meter {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if def == nil {
		return mnoop.NewMeterProvider().Meter("github.com/loomctl/loom")
	}
	return def.meter
}

// Default returns the singleton wired up by Init. Returns nil if Init has
// not been called.
func Default() *Telemetry {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return def
}

// ErrNotInitialized indicates a caller asked for a value that requires
// Init to have been called.
var ErrNotInitialized = errors.New("telemetry: not initialized")
