# ADR-0014: Download client abstraction (Phase 3a)

- Status: Accepted
- Date: 2025-02-13
- Deciders: @loom-maintainers

## Context

Phase 3 of the Loom roadmap covers grabbing releases that the indexer
aggregator surfaces. Before any grabber/queue logic can land we need a
stable place for download clients to live: a pluggable abstraction that
can host qBittorrent, Transmission, Deluge, SABnzbd and NZBGet drivers
without re-relitigating registry/health/transport plumbing each time. We
already have the same shape in `internal/indexers/`; Phase 3a is about
mirroring that shape for downloads — interface, registry, repository,
service, health checker, HTTP handlers — so each subsequent driver is a
single self-contained file plus a kind registration.

## Decision

We introduce `internal/downloads/` as a parallel of `internal/indexers/`
with these intentional asymmetries:

1. **No search/aggregator surface.** Downloads do not fan-out to
   external clients to satisfy a single user query the way the
   aggregator does; per-client operations (Add, Status, Pause, Resume,
   Categories, FreeSpace, Test) are addressed by client id.
2. **No rate-limit/retry per-client config.** Download clients are
   typically on the operator's LAN and run at a single concurrency
   knob. We still reuse `internal/indexers/throttle.Wrap` so transports
   compose with FlareSolverr/SOCKS/HTTP proxies the same way indexers
   do, but we don't expose `rate_limit_per_min` etc. on the API. If a
   particular driver needs throttling it can wire that internally.
3. **`builtin/null` is shipped.** A no-op driver registered via
   `init()` lets the rest of the stack (CRUD, health checker, fan-out)
   be exercised in tests and in fresh installs without forcing a real
   client. Subsequent phases register additional kinds with
   `downloads.RegisterKind(KindFoo, factoryFn)` from each driver's own
   package init.
4. **`SetDownloads` setter on the HTTP server.** The download service
   is built after `server.New` (the order is fixed by `cmd/loom/serve.go`
   for indexer parity reasons), so we expose `*server.Server.SetDownloads`
   which swaps the field and rebuilds the chi mux. Routes are mounted
   under `/api/v1/download-clients`.

### Schema

Migration `0010` adds `download_clients` and `download_client_health`
on both SQLite and Postgres. Columns mirror the indexer tables minus
the rate-limit columns and plus a few download-specific knobs
(`category_default`, `save_path_default`, `remove_completed`,
`remove_failed`, cached `last_categories_json`, `last_free_space_bytes`).
sqlc generates the row types for both engines and the repository layer
collapses the engine difference behind a single `Repository` interface.

### Interface

```go
type DownloadClient interface {
    ID() string
    Name() string
    Kind() string
    Protocol() Protocol

    Add(ctx context.Context, req AddRequest) (AddResult, error)
    Status(ctx context.Context, ids ...string) ([]Item, error)
    Pause(ctx context.Context, ids ...string) error
    Resume(ctx context.Context, ids ...string) error
    Remove(ctx context.Context, ids []string, deleteFiles bool) error
    Categories(ctx context.Context) ([]Category, error)
    FreeSpace(ctx context.Context) (int64, error) // -1 if unknown
    Test(ctx context.Context) error
}
```

## Consequences

**Positive**

- Adding a new download client is a focused diff: implement
  `DownloadClient`, register a factory, optionally add a config schema.
  No changes to the registry, repository, service, health checker, or
  HTTP handlers.
- Tests are cheap. Most behaviour is verifiable with `builtin/null`
  plus an in-memory fake client.
- The repository pattern keeps the engine fork (sqlite vs postgres
  sqlc-generated rows) confined to `repository.go`.

**Negative**

- Two parallel package trees (`internal/indexers/`, `internal/downloads/`)
  share an idiom but not code. Future deviations risk drift; we accept
  this rather than carve out a generic CRUD/health framework before we
  have a third instance to inform the abstraction.

## Deviations from the original brief

1. **Plaintext credentials.** The brief said "encrypt the same way
   indexers do." Reality: indexer and proxy configs are stored as
   plaintext JSON blobs today. We follow the existing convention.
   Adding at-rest encryption is tracked separately and will land
   uniformly across `indexers`, `proxies`, and `downloads` rather than
   piecemeal.
2. **`Item` vs `ItemStatus` naming.** The brief spec'd
   `Status(...) ([]ItemStatus, error)`. We split the names: `Item` is
   the row struct and `ItemStatus` is the lifecycle string enum
   (`queued`, `downloading`, `seeding`, `completed`, `paused`,
   `failed`). This matches how `HealthStatus` is modelled in the
   indexers package and avoids a struct type with a name that already
   reads as an enum.

## References

- Phase 3a brief (download client abstraction).
- ADR-0010 (indexers + proxies UI) — established the CRUD-and-health
  shape we mirror here.
- ADR-0013 (per-indexer rate limiting) — defined
  `internal/indexers/throttle`, which downloads now reuses for
  transport composition.
