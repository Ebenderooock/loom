# Indexer rate limits

Loom shapes outbound traffic to every indexer through a token-bucket
rate limiter and an exponential-backoff retry loop. This page is the
operator's reference for tuning that layer.

## Defaults

| Knob          | Default | Notes                                |
|---------------|---------|--------------------------------------|
| `per_minute`  | 60      | Token-bucket fill rate (req/min).    |
| `burst`       | 5       | Max requests back-to-back.           |
| `max_retries` | 3       | Retried attempts after the first.    |

Defaults are conservative on purpose: they keep Loom polite against
public trackers' published limits without operator intervention.

## Per-indexer overrides

Three optional fields on the `Indexer` API object override the
defaults:

```jsonc
{
  "id": "rarbg-mirror",
  "kind": "newznab",
  "name": "RARBG Mirror",
  // ...
  "rate_limit_per_min":  30,   // halve the default fill rate
  "rate_limit_burst":    2,    // smaller burst
  "retry_max_attempts":  5     // be patient with a flaky mirror
}
```

The same fields are accepted on `POST /api/v1/indexers`,
`PUT /api/v1/indexers/{id}`, and `PATCH /api/v1/indexers/{id}`. Omit
them to leave the column NULL, in which case the package default
applies.

The response always includes a `rate_limit` block under
`DefinitionWithHealth` showing both the configured override (may be
absent) and the effective value Loom is using:

```jsonc
{
  "id": "rarbg-mirror",
  // ...
  "rate_limit": {
    "per_minute":           30,
    "burst":                 2,
    "max_retries":           5,
    "effective_per_minute": 30,
    "effective_burst":       2,
    "effective_max_retries": 5
  }
}
```

`max_retries: 0` is honoured exactly — Loom will never retry that
indexer. To restore the default, send `null` (or omit the field on a
full PUT).

## What gets retried

| Outcome                         | Retried? | Reason label    |
|---------------------------------|----------|-----------------|
| HTTP 429 Too Many Requests      | yes      | `rate_limited`  |
| HTTP 503 Service Unavailable    | yes      | `unavailable`   |
| Transient network errors        | yes      | `network_error` |
| HTTP 4xx (other)                | no       | —               |
| HTTP 5xx (other)                | no       | —               |
| `context.Canceled` / deadline   | no       | —               |

Retried 429/503 responses honour the server's `Retry-After` header
verbatim (both seconds and HTTP-date forms), capped at 30 s so a
misbehaving server can't stall a search forever. Otherwise the
backoff schedule grows 250 ms → 500 ms → 1 s → 2 s → 4 s with a
±25 % jitter.

## Metrics

The throttle layer publishes four Prometheus metrics, all under the
`loom_indexer_` namespace:

- `loom_indexer_request_total{indexer,kind,outcome}` — final outcome
  of every outbound request. `outcome` is one of `success`,
  `client_error`, `server_error`, `error`.
- `loom_indexer_request_duration_seconds{indexer,kind}` — wall-clock
  latency including any rate-limit wait and retry sleeps.
- `loom_indexer_retries_total{indexer,reason}` — incremented each
  time a retry is performed (or, on the last attempt, the giving-up
  retry).
- `loom_indexer_ratelimit_wait_seconds{indexer}` — time blocked on
  the token bucket before the request was admitted.

A few queries operators tend to want:

```promql
# Top 5 noisiest indexers by 429s in the last 15m.
topk(5, sum by (indexer) (
  increase(loom_indexer_retries_total{reason="rate_limited"}[15m])
))

# Indexers spending more than 250ms on average waiting for a token.
avg by (indexer) (
  rate(loom_indexer_ratelimit_wait_seconds_sum[5m])
  /
  rate(loom_indexer_ratelimit_wait_seconds_count[5m])
) > 0.25
```

## Cookbook

- **Public tracker with a "max 1 req/sec" guideline** → `per_minute:
  60`, `burst: 1`. Loom never bursts past one in-flight request.
- **Usenet provider with a generous quota** → `per_minute: 600`,
  `burst: 20`, `max_retries: 5`. The high burst lets parallel
  searches go fast; the deep retry budget rides over their occasional
  503s.
- **Brittle Cardigann mirror** → `per_minute: 12`, `burst: 1`,
  `max_retries: 1`. One retry to paper over a flap, then surface the
  failure rather than thrash.
- **Disable retries entirely** → `max_retries: 0`. Useful when you'd
  rather see the raw failure in alerting than have Loom mask it.

## Future work

- Adaptive rate limiting (AIMD) once we have enough per-tracker
  telemetry to calibrate.
- Cross-indexer buckets when several indexers share an upstream
  tracker. Today each row gets its own bucket; coordinated buckets
  need distributed locking.
- Circuit breaking on definitively-down indexers so we stop retrying
  a corpse.
