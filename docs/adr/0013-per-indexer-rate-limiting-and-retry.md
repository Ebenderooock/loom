# ADR-0013: Per-indexer rate limiting and retry/backoff

- Status: Accepted
- Date: 2025-02-12
- Deciders: @loom-maintainers

## Context

Phase 2f adds an outbound traffic-shaping layer between Loom and the
indexer endpoints it talks to. Without it Loom would happily fan out
every search, capability check, and health probe at line rate; that's
fine for a single private tracker on a fast LAN but quickly trips
public-tracker abuse limits, FlareSolverr backpressure, and Usenet
provider quotas. We also have to be a polite client when several
indexers share a tracker family (e.g. multiple Cardigann definitions
that point at the same backend).

The transport composition order was already established in Phase 2e:

```
base http.Transport → proxy transport (FlareSolverr / SOCKS / HTTP)
```

We need to slot rate-limit + retry behaviour into that chain without
mutating the protected `internal/indexers/types.go` interfaces and
without bypassing the proxy layer.

## Decision

We add a new `internal/indexers/throttle` package that exposes a
single `Wrap(base, indexerID, kind, cfg, opts) http.RoundTripper`.
The package layers on top of any existing transport:

```
base → proxy → throttle (rate limit → retry/backoff)
```

The throttle layer:

1. **Token bucket** for steady-state pacing. We hand-roll the bucket
   rather than depend on `golang.org/x/time/rate` because we want the
   "wait" duration as a first-class metric, sub-millisecond
   determinism in tests via an injected clock, and a dependency-free
   import graph.
2. **Retry classification** of `429 Too Many Requests`,
   `503 Service Unavailable`, and transient transport errors. Caller
   cancellation (`context.Canceled` / `DeadlineExceeded`) is *not*
   retried — that's the operator's deliberate kill switch. Other
   4xx responses pass through untouched.
3. **Retry-After honoured verbatim** on 429/503, including
   HTTP-date form. We cap the wait at `MaxBackoff` (30 s) so a
   misbehaving server can't stall a search forever.
4. **Exponential backoff with jitter** otherwise: 250 ms → 500 ms →
   1 s → 2 s → 4 s, capped at 30 s, jittered ±25 %.
5. **Body replay** for retried requests: we buffer the body once and
   hand the underlying transport a fresh `io.NopCloser(bytes.Reader)`
   (and re-set `ContentLength` + `GetBody`) for each attempt.

Three nullable INTEGER columns on `indexers` (`rate_limit_per_min`,
`rate_limit_burst`, `retry_max_attempts`) hold the per-row overrides.
The repository exposes `GetRateLimit` / `SetRateLimit`, surfaced on
the service as `DefinitionWithHealth.RateLimit` so the API can show
"effective" values (override-or-default) without changing the
protected `Definition` struct. A `RateLimitProvider` package-global
lets `TransportForDefinition` look the config up at transport-build
time.

Defaults: 60 req/min, burst 5, max 3 retries — chosen conservatively
because most public trackers publish "be polite" guidance in the
30–120 req/min range and Usenet providers tolerate small bursts well.

## Consequences

### Positive

- Operators can dial each indexer to its provider's published limits
  without recompiling.
- A bad day on one tracker (rolling 503s) no longer cascades into
  client-visible search failures — retries paper over the blip.
- Metrics (`loom_indexer_request_total`, `_duration_seconds`,
  `_retries_total`, `_ratelimit_wait_seconds`) make it obvious when
  rate-limiting is biting and which indexer is the loudest.
- The throttle layer is composable: future kinds (Jackett-style,
  custom) get the same behaviour for free.

### Negative / trade-offs

- We can't share a bucket across indexers that share an upstream
  tracker. A tracker with five Cardigann definitions sees five
  separate buckets. Operators have to size each one to its share of
  the global budget. We deliberately punt cross-indexer coordination
  to a future ADR — global token coordination needs distributed
  locking once we go multi-instance.
- Retry on 503 is unconditional. A definitively-down tracker still
  costs us four attempts before we surface the failure. The
  alternative — circuit breaking — is also future work.
- Body buffering forces every retried request through a `[]byte` even
  if it's a 5 MB upload. Indexer requests are small (XML query
  strings, NZB metadata), so the trade-off is fine in practice.

### Neutral

- We register metrics against `telemetry.Default().Registry()` with a
  `sync.Once` + `defer recover()` to tolerate test-time
  re-registration. This matches the pattern used elsewhere in the
  kernel.
- The MaxRetries field uses `< 0` as the "use default" sentinel so
  that explicit `0` ("never retry") is honoured exactly. The
  alternative — a separate `MaxRetriesSet bool` — leaks JSON noise.

## Alternatives considered

- **`golang.org/x/time/rate`**: rejected because we wanted the wait
  duration as a metric, the bucket math was a few dozen lines, and
  pulling in the dependency just to get it didn't pay for itself.
- **Adaptive rate limiting** (AIMD à la TCP): more clever, but
  requires per-tracker telemetry we don't have yet. Phase 2f hard-
  codes a static dial; an adaptive layer can sit on top later.
- **Global rate budget across indexers**: needs a coordinator process
  and changes our deployment story. Out of scope for Phase 2f.
- **Editing `Definition`** to carry rate-limit fields directly:
  ruled out by the protection on `internal/indexers/types.go`. The
  `DefinitionWithHealth` envelope keeps the wire surface clean.
