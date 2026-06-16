# Indexers

Loom's indexer subsystem is the search-aggregation layer that connects
client UIs (and the Radarr/Sonarr/Prowlarr wire-compat surfaces in
later phases) to upstream torrent and Usenet sources. This document
covers the Phase 2a core: persistence, registry, fan-out search, and
periodic health checks. Phase 2c adds first-class `newznab` and
`torznab` kinds — see [indexers-newznab.md](indexers-newznab.md) for
their config schema, curl examples, and the caps-cache lifecycle.
Phase 2d adds an aggregator that **exposes** the indexer set as a
single Prowlarr-compatible Newznab/Torznab endpoint —
see [indexers-aggregator.md](indexers-aggregator.md). Phase 2b adds
the `cardigann` kind, which loads YAML tracker definitions from disk
and turns them into runnable indexers — see
[indexers-cardigann.md](indexers-cardigann.md) for the YAML schema,
trust model, and a worked walkthrough.

## Concepts

| Term | Meaning |
|---|---|
| **Definition** | The persisted row: ID, kind, name, enabled flag, priority, free-form `config_json`, list of Newznab category IDs, list of tag strings. Stored in `indexers`. |
| **Kind** | A factory key — e.g. `builtin/null`, `newznab`. Each kind owns one Go factory that builds a live `Indexer` from a `Definition`. |
| **Indexer** | The runtime object that satisfies the `indexers.Indexer` interface (`Search`, `Test`, `Caps`). One live instance per enabled definition. |
| **Registry** | Concurrency-safe in-memory map from ID to live Indexer. Hydrated at startup; mutated as the operator creates/edits/deletes indexers. |
| **Health** | The latest connectivity verdict for a single indexer: status (`ok` / `degraded` / `failed` / `unknown`), last check timestamp, last success timestamp, latency, last error message. Stored in `indexer_health`. |

## Lifecycle

1. **Boot.** `loom serve` opens storage, runs migrations, builds the
   indexer Service, and calls `Service.HydrateAll`. Every enabled
   `Definition` is run through its kind's factory; the resulting live
   instance lands in the Registry. Failures to hydrate are logged at
   warn level and do not block startup.
2. **Create / Replace / Patch.** Each mutation persists first, then
   updates the Registry. Disabling an indexer evicts the live
   instance; re-enabling it re-hydrates from the persisted
   definition.
3. **Delete.** Removes the live instance, then deletes the row.
   `indexer_health` rows cascade automatically (`ON DELETE CASCADE`).
4. **Periodic health.** The persistent scheduler runs the
   `indexers.health` job at `cfg.Indexers.HealthCheckSchedule`
   (default every 10 minutes). The job iterates the Registry,
   invokes `Test()` with `cfg.Indexers.HealthCheckTimeoutSec` per
   indexer, captures latency, and persists the new `Health` row via
   the Repository.
5. **Search.** `POST /api/v1/indexers/search` fans the Query out to
   either every registered indexer or a specified subset. Each call
   is bounded by the configured per-indexer timeout; the response
   merges all successful results and a per-source error map.

## Newznab categories

The `Category` constants in `internal/indexers/types.go` mirror the
canonical Newznab IDs used by Sonarr / Radarr / Lidarr clients:

| ID | Family |
|---|---|
| `1000` | Console |
| `2000` | Movies |
| `3000` | Audio |
| `4000` | PC |
| `5000` | TV |
| `6000` | XXX |
| `7000` | Books |
| `8000` | Other |

Sub-categories (e.g. `2040 Movies/HD`) are passed through as raw
integers and are not pre-declared on the Go side.

## Curl walkthrough

The examples below assume Loom is running on `:1925` and that you have
an API key in `$LOOM_KEY`. See [auth.md](auth.md) for how to mint one.

### Create

```bash
curl -sS -X POST "http://127.0.0.1:1925/api/v1/indexers/" \
  -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{
    "id": "demo",
    "kind": "builtin/null",
    "name": "Demo Null",
    "enabled": true,
    "priority": 25,
    "categories": [2000, 5000],
    "tags": ["public"]
  }'
```

### List

```bash
curl -sS -H "X-Api-Key: $LOOM_KEY" \
  http://127.0.0.1:1925/api/v1/indexers/
```

The list response wraps each `Definition` in a `DefinitionWithHealth`
that carries the latest `Health` row, when one exists.

### Capabilities

```bash
curl -sS -H "X-Api-Key: $LOOM_KEY" \
  http://127.0.0.1:1925/api/v1/indexers/demo/caps
```

### Test (manual health check)

```bash
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" \
  http://127.0.0.1:1925/api/v1/indexers/demo/test
```

Returns `{ok, latency_ms, error?}`; the same row is persisted into
`indexer_health` so subsequent `GET /api/v1/indexers/demo` reflects
it.

### Aggregated search

```bash
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"query":"ubuntu","categories":[4000]}' \
  http://127.0.0.1:1925/api/v1/indexers/search
```

Results from every enabled indexer are merged into a single list.
Per-source failures (timeouts, upstream 5xx, decode errors) appear in
the response's `errors` map keyed by indexer ID.

## Configuration

See [configuration.md](configuration.md#indexers). The relevant keys
are:

- `indexers.search_timeout` — default per-indexer fan-out ceiling
  (seconds).
- `indexers.max_parallel` — concurrency cap for a single fan-out.
- `indexers.health_check_schedule` — 5-field cron expression.
- `indexers.health_check_timeout` — per-Test() ceiling (seconds).

## Troubleshooting

- **Indexer absent from registry after start.** Inspect the boot
  log for `hydrate indexer failed`. Check the row in `indexers` —
  the `kind` must match a registered factory; an unknown kind logs
  and skips so a single broken row never blocks startup.
- **All searches time out.** Lower `search_timeout` is the usual
  cause; raise it to give slow sources a chance, or fall back to
  `indexer_ids` to cherry-pick fast ones. The health endpoint will
  also flag unreachable sources.
- **Health row stuck at `unknown`.** The first row is seeded as
  `unknown` on Create; the next run of `indexers.health` populates
  it. Confirm `scheduler.enabled: true` and that the scheduler log
  reports `indexers.health` in `JobNames`.
- **404 on every endpoint.** Indexer routes live under
  `/api/v1/indexers/...`. They require auth (or `auth.mode=disabled`
  in development) — anonymous calls reply with the auth-package's
  401 envelope, which is shaped differently from the indexer
  envelope.

## See also

- [ADR-0007](adr/0007-indexer-abstraction.md) — design rationale for
  the Indexer abstraction.
- [api.md](api.md) — wire-format reference.
- [configuration.md](configuration.md) — all `indexers.*` keys.
