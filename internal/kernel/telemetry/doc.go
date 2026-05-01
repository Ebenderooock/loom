// Package telemetry wires Loom's observability stack:
//
//   - OpenTelemetry traces with an OTLP/HTTP exporter, gated by config.
//   - An always-on Prometheus registry exposed at /metrics with the
//     standard process and Go-runtime collectors registered.
//
// Both an instance API (held by the HTTP server) and a small package-level
// singleton are exposed so callers can grab Tracer()/Meter() without
// threading the value everywhere.
//
// See docs/observability.md and ADR-0005.
package telemetry
