# ADR-0007: Indexer Abstraction

- Status: Accepted
- Date: 2025-01-13
- Deciders: @loom-maintainers

## Context

Loom replaces Radarr/Sonarr/Prowlarr by aggregating searches across
multiple torrent and Usenet trackers. The first piece of that
aggregation layer is the abstraction every concrete source plugs
into. Constraints we want to satisfy from day one:

- **Pluggable kinds.** Cardigann YAML definitions, Newznab APIs,
  Torznab feeds, and a built-in null stub all need to live behind
  one interface. Adding a new kind should not require changes to
  the HTTP surface, the scheduler, or persistence.
- **Concurrent and cancellable.** Fan-out search hits dozens of
  sources in parallel; one slow source must not stall the rest.
  Every method that does I/O takes a `context.Context`.
- **Persistence-aware.** Indexers are configured at runtime by
  operators, not at compile time. CRUD has to be transactional
  and survive restarts.
- **Engine-neutral.** Loom supports SQLite and Postgres equally;
  any new persistence has to land in both engines together.
- **Observability.** Health and latency must be visible to
  operators without turning on debug logging.

## Decision

We introduce a single `internal/indexers` package owning four
collaborators:

- **`Indexer`** interface (`ID`, `Name`, `Caps`, `Search`, `Test`)
  — the contract every kind implements.
- **`Registry`** — concurrency-safe `map[string]Indexer` providing
  Register/Replace/Remove/Get/List plus `Search` for bounded fan-out
  with per-source error capture.
- **`Repository`** — engine-dispatch persistence over the
  sqlc-generated SQLite and Postgres query packages, exposing
  `Definition` + `Health` value types so the rest of the package
  doesn't see engine-specific column types.
- **`Service`** — the orchestration layer that owns
  Repository+Registry, runs `HydrateAll` at boot, and exposes
  CRUD/Test/Search to HTTP and to the persistent scheduler.

Each kind registers a `Factory` under its kind string (e.g.
`builtin/null`); the Service uses the factory catalogue to hydrate
persisted rows into live instances.

The persistent scheduler hosts `indexers.health` as a recurring job
that reuses `Service.TestOne` for per-indexer Test+latency capture.
Health rows are persisted by the Service so the manual `POST
/{id}/test` endpoint and the periodic sweep share a single code path.

## Consequences

### Positive
- One Go interface to teach contributors; new kinds are a single
  factory + tests.
- HTTP layer is thin (`Mount` + struct decoding); business logic
  lives in the Service.
- Health is observable via the same row regardless of whether the
  check came from the scheduler or a manual `POST /test`.
- Engine-dispatch is contained in the Repository; downstream code
  never branches on `Engine()`.

### Negative / trade-offs
- The Repository duplicates query wiring across two sqlc packages
  (one per engine). Mitigated by mirroring the same query names in
  both query files and keeping decoding helpers shared.
- The `Indexer` interface is intentionally minimal; richer
  capabilities (download throttling, rate-limit headers) will need
  follow-up additions.

### Neutral
- Choosing per-indexer fan-out timeouts (rather than a global
  budget) means a slow source can still consume full request
  latency if the operator picks a generous ceiling. This is
  documented in `docs/configuration.md`.

## Alternatives considered

- **Direct sqlc usage from HTTP.** Rejected: ties the API layer to
  storage details and makes the engine split visible to handlers.
- **One generic plugin host (e.g. WASM).** Rejected for now —
  out-of-process plugins add a deployment dimension we don't need
  for v1, and Cardigann/Newznab/Torznab are well-served by Go
  factories.
- **Per-kind tables.** Rejected: every kind would duplicate the
  same metadata columns. The current design keeps shared columns
  in `indexers` and pushes kind-specific config into the JSON
  blob.
