# ADR-0003: API surface — REST + WebSocket native, gRPC internal, OpenAPI 3.1

- Status: Accepted
- Date: 2025-05-01
- Deciders: Loom maintainers

## Context

Loom must expose:

1. A modern API for its own React frontend and third-party integrators.
2. **Wire-compatible** surfaces matching Radarr v3, Sonarr v3 + v4, and
   Prowlarr v1, so existing clients (Overseerr, Jellyseerr, Bazarr,
   Notifiarr, Tautulli) continue to work.
3. An internal boundary that lets each module be hosted in a separate
   process when split-mode deployment is enabled.

## Decision

- **Native API**: REST authored as **OpenAPI 3.1** in `api/openapi/loom.yaml`,
  served under `/api/v1/...`. Live events streamed over **WebSocket** at
  `/api/v1/events`.
- **Compat APIs**: each upstream surface implemented as a thin handler
  package under `internal/compat/<app>/` mounted at the upstream's path
  (`/api/v3/...` for Radarr/Sonarr, `/api/v1/...` for Prowlarr — namespaces
  disambiguated by route prefix).
- **Internal RPC**: **gRPC** between modules when running in split-mode;
  schemas in `api/internal/*.proto`.
- **Codegen**: TypeScript client generated for the frontend from
  `loom.yaml` via `openapi-typescript`. Go server stubs generated via
  `oapi-codegen`.

## Consequences

### Positive
- One source of truth (OpenAPI) drives docs, the TS client, and server
  stubs. The website's API reference is generated, not hand-written.
- WebSocket replaces SignalR for live updates without a JS SDK lock-in.
- Compat handlers are isolated; we can iterate on the native API without
  destabilizing downstream integrations.

### Negative / trade-offs
- Maintaining compat handlers is ongoing work; we mitigate via an
  acceptance-test suite that runs Overseerr/Bazarr/Notifiarr in CI.
- gRPC adds a dependency only used in split-mode; we accept it for the
  type safety it gives module boundaries.

### Neutral
- Authentication is uniform across surfaces (forms cookie, API key, OIDC,
  trusted proxy) — see ADR-0004.

## Alternatives considered

- **GraphQL** — overkill for our resource shapes; harder to mimic
  upstream REST surfaces; more complex caching for the queue/history hot
  path. We may revisit for the public read API later.
- **gRPC for everything** — fights browser support, makes compat layers
  awkward, and excludes the curl-friendly debugging that self-hosters
  expect.
