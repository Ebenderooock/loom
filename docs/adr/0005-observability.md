# ADR-0005: Observability — OpenTelemetry first, Prometheus + slog + pprof

- Status: Accepted
- Date: 2025-05-01
- Deciders: Loom maintainers

## Context

Self-hosters increasingly run Prometheus + Grafana on their homelab. The
*arr stack offers no native metrics, only log lines. Loom must be
observable from the first commit, not as a v2 bolt-on.

## Decision

- **Logs**: `log/slog` JSON handler. Per-module loggers carry
  `module=indexers` etc. PII (API keys, tokens, full URLs with credentials)
  is redacted by a wrapping handler.
- **Metrics**: OpenTelemetry SDK feeding both an OTLP exporter (when
  configured) and a Prometheus registry exposed at `/metrics`.
- **Traces**: OTel SDK with OTLP/gRPC exporter. Default sampler is
  parent-based + ratio (1% in production, 100% in dev).
- **Profiling**: `net/http/pprof` mounted at `/debug/pprof` behind an
  admin-only route, off unless `LOOM_PROFILING=true`.
- **Health**: `/healthz` (process), `/readyz` (DB + scheduler reachable),
  `/livez` (process not deadlocked, last-tick within budget).
- **Dashboards**: ship Grafana dashboards in `deploy/grafana/` for library
  health, indexer SLOs, queue throughput, import success rate.

## Consequences

### Positive
- Day-one Prometheus scraping. No plugin needed.
- OTLP path lets users send to Honeycomb, Tempo, Datadog, etc.
- Structured logs drop straight into Loki / Elasticsearch.

### Negative / trade-offs
- OpenTelemetry pulls a non-trivial dep set; we accept it for the
  long-term flexibility.

### Neutral
- We expose business metrics deliberately (indexer success rate, parse
  failure counts, hardlink-vs-copy ratio) — not just Go runtime stats.

## Alternatives considered

- **Prometheus client only** — easy, but locks out OTLP users.
- **Datadog/NewRelic SDK first** — vendor lock-in inappropriate for a
  self-hosted project.
- **Logs only (no metrics)** — what *arr does today; we're trying to fix
  that.
