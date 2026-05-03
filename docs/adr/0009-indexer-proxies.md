# ADR-0009: Indexer outbound proxies

- Status: Accepted
- Date: 2025-09-15
- Deciders: @loom-maintainers

## Context

Real-world deployments of Loom commonly need to route indexer traffic
through outbound proxies. Operators ask for this for three reasons:

1. **Network policy** — a corporate or homelab egress where only a
   sanctioned HTTP CONNECT or SOCKS5 gateway can reach the public
   internet.
2. **Geo-fenced indexers** — a private tracker that only answers from
   a residential IP, reachable via a SOCKS5 tunnel.
3. **Cloudflare-protected indexers** — sites that interrogate the
   client with a JavaScript challenge before serving anything. Native
   `*Arr` apps front these with [FlareSolverr], a headless-Chrome
   companion service that solves the challenge and returns the
   original response body.

Phase 2c left the newznab/torznab kind with a single
`http.DefaultTransport`-backed `*http.Client`. Adding proxies needs a
home that doesn't bloat each kind's package and that can be reused by
future kinds (Cardigann, custom HTTP).

We considered three architectural shapes:

1. **Inline `proxy_url` field on the indexer config.** Cheap, but
   non-shareable: ten indexers behind the same FlareSolverr would
   duplicate config and burn ten browser sessions.
2. **A global egress proxy in kernel config.** Easy, but loses the
   per-indexer flexibility above and makes FlareSolverr's
   per-indexer session model awkward.
3. **A first-class `proxies` resource** with its own table, CRUD API,
   and a callback the indexers package consults when building each
   kind's `*http.Client`.

## Decision

Option 3. Loom adds a new `internal/indexers/proxies` package and a
`proxies` table (migration 0008) holding `id, kind, name, enabled,
config (jsonb), created_at, updated_at`. Indexers gain an optional
`proxy_id` FK column.

The proxies package owns:

- `Repository` (sqlite + Postgres adapters, mirroring `indexers`).
- `Service` with config validation, transport caching, `TestProxy`,
  and an `indexers.RouteMounter` that registers
  `/api/v1/proxies/*` under the same auth scope as the indexers
  module (no changes to `internal/server`).
- `BuildTransport(p, fs)` and a `Provider` cache keyed by proxy ID,
  invalidated on any Replace/Patch/Delete.
- A FlareSolverr round-tripper that POSTs `{cmd:"request.get",...}`
  to `<URL>/v1` and synthesises an `*http.Response` from the
  solution envelope.

`indexers` exposes a small `TransportProvider` interface and a
package-level `SetTransportProvider` slot. The newznab/torznab kind
now takes a `Definition` argument when building its `*http.Client`
and looks up the round-tripper via `TransportForDefinition(def)`.
`SetTransportProvider` is wired in `cmd/loom` *before* the indexers
service is constructed so first-build of every indexer sees the
right transport.

## Consequences

### Positive

- One FlareSolverr instance is reused across many indexers, with
  optional `session_mode: shared` keeping a single browser session
  alive per proxy row.
- Proxies have their own lifecycle: operators can flip `enabled` or
  rotate credentials without touching every indexer that uses them.
- `TransportFor(id)` is the single seam every future kind needs to
  honour — no per-kind proxy implementation.
- The CRUD API is self-contained and doesn't widen the indexers
  service.

### Negative / trade-offs

- Adds a second resource that can drift out of sync with the
  indexer that uses it. Mitigated by a 409 `proxy_in_use` response
  on `DELETE /api/v1/proxies/{id}` whose `details.indexer_ids`
  enumerates the blockers, plus by automatic transport-cache
  invalidation on every Patch/Replace.
- The FlareSolverr round-tripper synthesises a `*http.Response`
  rather than streaming — the full body is in memory before the
  caller sees it. Acceptable for newznab RSS payloads, unsuitable
  for ever shipping large file fetches through the same path.
- `golang.org/x/net/proxy` becomes a direct dependency for SOCKS5.

### Neutral

- The existing `PatchIndexer` query is COALESCE-based and can't
  represent "set to NULL". A separate `SetIndexerProxyID :exec`
  query handles the proxy-detach case unconditionally.
- The proxies handlers use a slightly extended error envelope
  (`code/message/details`) versus the indexers module's
  `code/message`, because 409 needs a structured `indexer_ids`
  field. Both shapes share the same outer key (`error`).

## Alternatives considered

- **Reuse `internal/kernel/http.Client`.** Loom doesn't have one;
  every kind today calls `http.DefaultTransport`. We'd have to build
  it anyway.
- **Per-kind proxy plumbing.** Doesn't scale and leaks the same
  config UI into every future kind.
- **Run FlareSolverr as a child process.** Operationally complex and
  ties Loom's release cadence to FlareSolverr's. We let the operator
  bring their own instance via docker-compose.

[FlareSolverr]: https://github.com/FlareSolverr/FlareSolverr
