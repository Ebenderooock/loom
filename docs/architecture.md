# Architecture

Loom is a **modular monolith** in Go. Modules — `indexers`, `movies`, `series`,
`downloads`, `metadata`, `notifications`, `parser`, `importer`, `compat`,
`migrate` — live in one process by default but communicate through a typed
event bus and stable HTTP boundaries, so they can be split into separate
deployments later without code changes.

This page summarises the architecture. The authoritative source is the
project plan (Phase 0 / Phase 1 sections of the master plan) and ADR-0001.

## Diagram

```text
                   ┌──────────────────────────────────────────────┐
                   │                  loom (binary)               │
  HTTP/WS  ─────►  │  ┌────────────┐   ┌──────────────────────┐   │
                   │  │ API Gateway│   │  Internal Event Bus  │   │
                   │  │ (REST/WS/  │◄─►│  (in-proc channels;  │   │
                   │  │  compat)   │   │   embedded NATS in   │   │
                   │  └─────┬──────┘   │   split-mode)        │   │
                   │        │          └──────────┬───────────┘   │
                   │        │                     │               │
                   │  ┌─────▼─────┐ ┌────────┐ ┌──▼─────┐ ┌─────┐ │
                   │  │ Indexers  │ │ Movies │ │ Series │ │ ... │ │
                   │  │ (Prowlarr)│ │(Radarr)│ │(Sonarr)│ │     │ │
                   │  └─────┬─────┘ └───┬────┘ └───┬────┘ └─────┘ │
                   │        │           │           │             │
                   │  ┌─────▼───────────▼───────────▼──────────┐  │
                   │  │  Shared kernel: scheduler, config,    │  │
                   │  │  download-clients, metadata-providers,│  │
                   │  │  notifications, history, queue, RSS,  │  │
                   │  │  parser, quality, custom formats      │  │
                   │  └─────┬─────────────────────────────────┘  │
                   │        │                                    │
                   │  ┌─────▼────────┐    ┌──────────────────┐   │
                   │  │ Storage (DB) │    │  Filesystem I/O  │   │
                   │  │ SQLite/PG    │    │  (atomic, hard-  │   │
                   │  │              │    │  link aware)     │   │
                   │  └──────────────┘    └──────────────────┘   │
                   └──────────────────────────────────────────────┘
```

## Module boundaries

Every module exposes:

- A Go interface package (`pkg/<module>/api`) — the stable contract.
- An HTTP/REST surface mounted at `/api/v1/<module>`.
- A wire-compat surface where applicable (`/api/v3/...` for Radarr/Sonarr,
  `/api/v1/...` for Prowlarr — disambiguated by route prefix).
- Typed publishers and subscribers on the internal event bus
  (`grab.requested`, `import.completed`, …).

This is what enables the **optional split deployment**: each module's HTTP
surface and event subscriber can be hosted in its own pod, sharing the same
database and embedded-NATS bus, with no code change — only a deploy-mode
flag.

## Repository layout

```text
loom/
├── cmd/loom/                  # main binary entrypoint
├── internal/
│   ├── server/                # HTTP/WS server, middleware, auth wiring
│   ├── kernel/                # config, scheduler, eventbus, logging, telemetry
│   ├── storage/               # db abstraction, migrations, sqlc-generated queries
│   ├── auth/                  # forms / API key / OIDC / proxy auth (Phase 1c)
│   ├── indexers/              # Prowlarr-equivalent module     (Phase 2)
│   ├── movies/                # Radarr-equivalent module       (Phase 5)
│   ├── series/                # Sonarr-equivalent module       (Phase 6)
│   ├── downloads/             # download-client adapters       (Phase 3)
│   ├── metadata/              # TMDB/TVDB/IMDB/Trakt providers (Phase 4)
│   ├── parser/                # release-name parser            (Phase 5)
│   ├── notifications/         # Discord/Slack/Apprise/...      (Phase 10)
│   ├── importer/              # file import pipeline           (Phase 5)
│   ├── compat/                # wire-compat handlers           (Phase 7)
│   └── migrate/               # importers from existing arr DBs (Phase 8)
├── api/
│   ├── openapi/loom.yaml      # native API spec
│   └── compat/                # captured upstream specs we mimic
├── web/                       # React + TS app (Vite)
├── deploy/
│   ├── docker/                # Dockerfile (distroless), compose example
│   ├── helm/loom/             # Helm chart                    (Phase 11)
│   └── kustomize/             # base + overlays               (Phase 11)
├── docs/                      # this directory; powers the docs site
└── .github/workflows/         # CI: test, lint, build, release, docker
```

## Process model

- **Default (single binary).** Everything runs in one process. Modules
  communicate over the in-process event bus (`internal/kernel/eventbus`)
  and call each other's Go interfaces. SQLite is the database. This is
  the 90 % case.

- **Split mode (Phase 11).** Modules can be hosted in separate processes
  (one pod per module in Kubernetes is the canonical layout). They share
  a Postgres database and an embedded-NATS bus. Module Go code is
  unchanged; only the deploy-mode flag and the bus implementation differ.

## Persistence

See [storage.md](storage.md). One schema, two engines (SQLite default,
Postgres optional), goose-managed migrations, sqlc-typed queries.

## Scheduling

See [scheduler.md](scheduler.md). Recurring work runs through a
persistent, cron-driven scheduler (`internal/kernel/scheduler`) backed
by `robfig/cron/v3`. Job definitions live in code; the
`scheduled_jobs` table records `last_run_at`, `next_run_at`,
`last_status`, and `last_error` so schedules and run history are
durable across restarts. At-most-one execution per job is enforced
per-process, and shutdown gives in-flight handlers a configurable
grace before abandoning them.

## Indexers

See [indexers.md](indexers.md). The `internal/indexers` package owns
the search-aggregation layer. A `Service` orchestrates a persistent
`Repository` (engine-dispatch over the sqlc-generated SQLite and
Postgres query packages), an in-memory `Registry` of live indexers,
and a `HealthChecker` that runs as a scheduler job. New source
kinds plug in by registering a `Factory` for their kind string.
Phase 2a shipped `builtin/null` to exercise the abstraction
end-to-end. Phase 2c adds first-class `newznab` and `torznab` kinds
(`internal/indexers/newznab`) that share a single `Client` and persist
their caps document via `CapsCache` to `indexer_health.last_caps_json`,
so a restart doesn't blank-state every feed. See
[indexers-newznab.md](indexers-newznab.md).

## Observability

See [observability.md](observability.md). slog JSON logs, Prometheus
`/metrics`, OpenTelemetry traces over OTLP/HTTP, pprof gated by config.

## API surface

See [api.md](api.md) and [`api/openapi/loom.yaml`](../api/openapi/loom.yaml).
The native API is `/api/v1/*`; wire-compat surfaces are added in Phase 7.

## References

- ADR-0001 — Use Go for the backend.
- ADR-0002 — Storage: SQLite default, Postgres optional, one schema.
- ADR-0003 — API: REST + WebSocket native, gRPC internal, OpenAPI 3.1.
- ADR-0004 — Authentication strategy.
- ADR-0005 — Observability strategy.
- ADR-0006 — Persistent scheduler (cron + scheduled_jobs).
- ADR-0007 — Indexer abstraction.
- ADR-0008 — Newznab + Torznab outbound client.
- Project plan (mirrored as [ROADMAP.md](../ROADMAP.md)).
