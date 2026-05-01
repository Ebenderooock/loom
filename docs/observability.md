# Observability

Loom is observable from day one — logs, metrics, traces, and an optional
profile endpoint, all configurable. See ADR-0005 for the rationale.

## Logging

- Implementation: Go's standard `log/slog` via `internal/kernel/logging`.
- Format: JSON by default; `text` for local development.
- Level: `debug` | `info` | `warn` | `error`. Both level and format are
  hot-reloadable when `hot_reload: true` and a YAML file is in use.
- **Redaction.** A fixed set of attribute keys is replaced with
  `[REDACTED]` before emission so credentials cannot leak: `password`,
  `passwd`, `secret`, `token`, `api_key`, `apikey`, `authorization`,
  `cookie`, `set-cookie`. Add new sensitive keys via a PR to
  `internal/kernel/logging/logging.go`.

Each request also carries a request-ID (`X-Request-Id`, populated by
chi's `RequestID` middleware) which is logged on the access-log line.

### Sample line

```json
{
  "time": "2025-05-01T13:50:14.221Z",
  "level": "INFO",
  "msg": "http",
  "method": "GET",
  "path": "/api/v1/system/status",
  "status": 200,
  "bytes": 142,
  "duration_ms": 4,
  "request_id": "loom/abc123",
  "remote": "10.0.0.7"
}
```

## Metrics

- Always-on Prometheus registry exposed at **`GET /metrics`**.
- Standard Go-runtime + process collectors are pre-registered.
- Exposition format: `text/plain; version=0.0.4`.

### Sample `prometheus.yml` scrape

The repo ships `deploy/prometheus.yml`:

```yaml
global:
  scrape_interval: 30s
  evaluation_interval: 30s

scrape_configs:
  - job_name: loom
    metrics_path: /metrics
    static_configs:
      - targets: ["loom:8989"]
```

Drop it next to a Prometheus container and point Prometheus at it; the
included `docker-compose.yml` already wires this up.

### Grafana dashboards

A Loom-curated dashboard set will land in Phase 11. Until then, the Go
runtime and process collectors are enough to chart memory, GC pauses,
and goroutine count out of the box.

## Traces

- OpenTelemetry SDK with an OTLP/HTTP exporter.
- Gated by `otel.enabled: true` (or `telemetry.otlp_endpoint` non-empty).
- Sampling: `telemetry.trace_ratio` ∈ [0, 1] (parent-based).
- W3C trace-context and Baggage propagators are installed globally so
  traces stitch across hops.

Point an OTel collector (or Tempo, Jaeger, Honeycomb, Datadog, …) at
`otel.endpoint`. The collector then fans out to the storage backend of
your choice.

## Profiling

- `debug.pprof: true` mounts the standard `net/http/pprof` handlers under
  `/debug/pprof/*`.
- **Disabled by default.** Profiling endpoints expose detailed runtime
  data and should be gated behind the reverse proxy / network policy.

## Health probes

| Path | Purpose |
|---|---|
| `GET /healthz` | Liveness — process is up. |
| `GET /livez` | Same shape as `/healthz`; conventional alias. |
| `GET /readyz` | Readiness — checks DB ping and the internal `ready` flag; returns 503 until both pass. |

Use `/livez` for Kubernetes liveness probes and `/readyz` for readiness;
`/healthz` is the Docker `HEALTHCHECK` target invoked by the
`loom healthcheck` subcommand.

## Quick mental model

| Signal | Endpoint | Required config | Default |
|---|---|---|---|
| Logs (stdout) | — | `log.*` | on |
| Metrics | `/metrics` | `telemetry.prometheus` | on |
| Traces | OTLP/HTTP push | `otel.enabled` + `otel.endpoint` | off |
| Profiles | `/debug/pprof/*` | `debug.pprof` | off |
| Health | `/healthz`, `/livez`, `/readyz` | — | on |
