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

### Curl the metrics endpoint

The default Prometheus collectors are mounted out of the box, so a fresh
`loom serve` already exposes hundreds of useful series. Confirm with:

```bash
$ curl -s http://localhost:1925/metrics | head -20
# HELP go_build_info Build information about the main Go module.
# TYPE go_build_info gauge
go_build_info{checksum="",path="github.com/ebenderooock/loom",version="..."} 1
# HELP go_gc_duration_seconds A summary of the wall-time pause (stop-the-world) duration in garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 3.1e-05
go_gc_duration_seconds{quantile="0.25"} 5.3e-05
go_gc_duration_seconds{quantile="0.5"} 5.9e-05
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 9
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 2.1827584e+07
```

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
      - targets: ["loom:1925"]
```

Drop it next to a Prometheus container and point Prometheus at it; the
included `docker-compose.yml` already wires this up.

### Grafana dashboards

A Loom-curated dashboard set will land in Phase 11 at
`deploy/grafana/loom.json` (placeholder — file does not exist yet).
Until then, the Go runtime and process collectors are enough to chart
memory, GC pauses, and goroutine count out of the box.

### Indexer traffic-shaping metrics

The throttle layer in front of every indexer publishes four series
under `loom_indexer_*`:

| Metric | Labels | Meaning |
|---|---|---|
| `loom_indexer_request_total` | `indexer`, `kind`, `outcome` | Final outcome of every outbound HTTP request. `outcome` ∈ `success`, `client_error`, `server_error`, `error`. |
| `loom_indexer_request_duration_seconds` | `indexer`, `kind` | Wall-clock latency including any rate-limit wait and retry sleeps. |
| `loom_indexer_retries_total` | `indexer`, `reason` | Retry attempts performed. `reason` ∈ `rate_limited` (429), `unavailable` (503), `network_error`. |
| `loom_indexer_ratelimit_wait_seconds` | `indexer` | Time blocked on the per-indexer token bucket before the request was admitted. |

See [`docs/indexers-rate-limits.md`](indexers-rate-limits.md) for
example PromQL queries and tuning advice.

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
