# Changelog

All notable changes to Loom are documented here.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Phase 3e — SABnzbd download client.** First Usenet driver on the
  Phase 3a download-client abstraction. New
  `internal/downloads/sabnzbd/` package speaks the SABnzbd JSON API
  (SABnzbd 3.x and newer), authenticating with the operator's apikey
  on every request and surfacing SAB's HTTP-200 `{"status":false,"error":...}`
  envelope as typed errors (`ErrAuth`, `ErrServer`, `ErrNotFound`,
  `ErrUpstream`, `ErrMalformedNZB`, `ErrConfig`). `Add` covers both
  URL fetches (`mode=addurl`) and raw `.nzb` uploads
  (`mode=addfile`, multipart), encoding category, priority, script,
  and post-processing via tag-prefixed `AddRequest.Tags`. `Status`
  merges the live queue and recent history into a single
  `[]downloads.Item`, mapping SAB's queue + history vocabularies onto
  Loom's `ItemStatus` enum in two audited tables; `Pause`, `Resume`,
  `Remove(deleteFiles)` (with queue→history fallback), `Categories`
  (rich `get_config` with `get_cats` fallback, filtering SAB's `*`
  default sentinel), `FreeSpace` (incomplete dir, GB-string→bytes),
  and `Test` (`mode=version`) round-trip the matching endpoints.
  Outbound HTTP composes the same proxy + throttle stack as indexers
  via `downloads.TransportForDefinition`. OpenAPI adds a
  `SabnzbdConfig` schema documenting the recognised `config` blob.
  Server-config management, post-processing scripts management,
  speed-limit control, and TLS-skip are deferred. See
  [`docs/downloads-sabnzbd.md`](docs/downloads-sabnzbd.md) and
  [ADR-0016](docs/adr/0016-sabnzbd-download-client-kind.md).

- **Phase 3a — Download client abstraction.** New
  `internal/downloads/` package providing a pluggable download-client
  registry that mirrors the indexer subsystem: `DownloadClient`
  interface, in-process registry with timeout-bounded fan-out, dual
  SQLite/Postgres repository, service layer with hydrate-on-startup,
  scheduled `downloads.health` job, and a chi-mounted HTTP surface at
  `/api/v1/download-clients` (CRUD + `test`, `categories`,
  `free-space`, `items`, `pause`, `resume`). Migration `0010` adds
  `download_clients` and `download_client_health` on both engines.
  A no-op `builtin/null` driver ships in-tree so the rest of the
  stack is exercisable without configuring a real client; subsequent
  drivers register themselves via `downloads.RegisterKind(...)` from
  their own `init()`. A new `downloads:` config block (with
  `operation_timeout`, `max_parallel`, `health_check_schedule`, and
  `health_check_timeout`) wires defaults; transport composition reuses
  `internal/indexers/throttle` so FlareSolverr/SOCKS/HTTP proxies
  apply uniformly. See [`docs/downloads.md`](docs/downloads.md) and
  [ADR-0014](docs/adr/0014-download-clients-abstraction.md).

- **Phase 2f — Per-indexer rate limiting and retry/backoff.** Every
  outbound indexer request now passes through a token-bucket rate
  limiter (default 60 req/min, burst 5) followed by an exponential-
  backoff retry loop (default 3 retries, capped at 30s) that honours
  `Retry-After` on 429/503 responses. Defaults can be overridden per
  indexer via three new optional fields on the API object —
  `rate_limit_per_min`, `rate_limit_burst`, `retry_max_attempts` —
  and the response surfaces a `rate_limit` block showing both the
  configured override and the effective value. Migration `0009`
  adds nullable INTEGER columns on `indexers` for both SQLite and
  Postgres. Four new Prometheus metrics under `loom_indexer_*`
  expose request outcomes, latency, retries, and rate-limit wait
  time. New package `internal/indexers/throttle`. See
  [`docs/indexers-rate-limits.md`](docs/indexers-rate-limits.md) and
  [ADR-0013](docs/adr/0013-per-indexer-rate-limiting-and-retry.md).

- **Phase 2b — Cardigann YAML definition loader.** New `cardigann`
  indexer kind that loads tracker definitions from
  `<data_dir>/definitions/cardigann/` (override via
  `indexers.cardigann.definitions_dir`) and drives login + search
  flows through CSS/XPath selectors and a small filter chain. The
  API never accepts inline definitions — operators distribute YAML
  out-of-band — and the indexer config references a definition by
  filename. Implements the form/post/cookie login modes, the
  most-used filters (replace, trim, regexp, querystring, prepend,
  append, split, lowercase, uppercase, join), and the standard
  Newznab category-name lookup (`Movies/HD`, `TV/HD`, …). New
  package `internal/indexers/cardigann`. See
  [`docs/indexers-cardigann.md`](docs/indexers-cardigann.md) and
  [ADR-0012](docs/adr/0012-cardigann-yaml-definition-loader.md).

- **Phase 2d — Newznab/Torznab aggregator server.** Loom now
  presents every enabled indexer as a single Prowlarr-compatible
  Newznab/Torznab endpoint, mounted at both `/api` (the canonical
  Prowlarr-compat path) and `/api/v1/aggregate` (Loom-namespaced
  alias). Supports `t=caps`, `t=search`, `t=movie`, `t=tvsearch`,
  `t=music`, `t=book`. Caps document is the union of every
  indexer's modes/categories/supported IDs. Authentication is via
  the existing API key store, supplied as `?apikey=…` (with
  `X-Api-Key` accepted as a fallback) and surfaced as Newznab-shape
  XML errors. Per-indexer failures during a fan-out are logged and
  dropped from the response rather than failing the whole request,
  matching Prowlarr's partial-success semantics. New package
  `internal/indexers/newznabserver`. See
  [`docs/indexers-aggregator.md`](docs/indexers-aggregator.md) and
  [ADR-0011](docs/adr/0011-newznab-aggregator-server.md).

- **Phase 2g — indexer & proxy management UI.** New `/indexers` and
  `/proxies` pages in the React frontend, with create/edit/delete
  dialogs, manual search per indexer (via the fan-out endpoint scoped
  with `indexer_ids`), health badges with a 24 h staleness rule,
  credential-masked proxy URLs, and PATCH null-vs-omit `proxy_id`
  semantics matching the OpenAPI contract. Typed fetch client parses
  the `{error: {code, message}}` envelope into an `ApiError`, surfacing
  `409 proxy_in_use` inline. See
  [`docs/web/indexers-ui.md`](docs/web/indexers-ui.md) and
  [ADR-0010](docs/adr/0010-indexers-and-proxies-ui.md).

### Changed

- **Indexers — first-class torrent fields on `Result`.** Added
  `Infohash`, `MagnetURI`, and switched `Seeders`/`Peers` to `*int` on
  `internal/indexers.Result` so torrent metadata no longer rides on
  the unrelated `Quality` field. Torznab parsing populates all four
  from the matching `<torznab:attr>` tags; Usenet results leave the
  torrent fields zero/nil, which lets callers tell "indexer didn't
  report this" apart from "torrent has zero seeders right now". The
  OpenAPI `Result` schema gains `infohash`, `peers` (replacing the
  unused `leechers` placeholder), `magnet_uri`, and `quality`
  properties with descriptions.

### Added

- **Phase 0 — bootstrap.** Repository skeleton, AGPL-3.0 license,
  `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, GitHub issue
  templates, GoReleaser config, multi-arch Dockerfile, and the first
  five ADRs (language, storage, API, auth, observability). (`2d291e6`)
- **Phase 1a — platform kernel.** Layered Viper config with hot-reload,
  `slog` JSON logging with PII redaction, OpenTelemetry SDK + OTLP
  exporter, Prometheus `/metrics`, pprof gating, in-process event bus,
  cron-style scheduler, chi-based HTTP server with health probes
  (`/healthz`, `/livez`, `/readyz`) and `/api/v1/system/status`. (`f0ea941`)
- **Phase 1b — storage layer.** Two-engine database abstraction
  (SQLite via `modernc.org/sqlite`, Postgres via `jackc/pgx`), embedded
  goose migrations per engine, sqlc-generated query packages, schema
  meta table, and a unified `Querier` interface. (`10bdae3`)
- **Phase 1e — frontend scaffold.** React 18 + TypeScript + Vite +
  TanStack Router/Query + shadcn/ui + Tailwind app under `web/`,
  including ESLint flat config, Prettier, Storybook, Playwright. (`1b2fb09`)
- **Phase 1d — persistent scheduler.** Cron-driven scheduler in
  `internal/kernel/scheduler` backed by `robfig/cron/v3` and the
  `scheduled_jobs` table. Run history (`last_run_at`, `next_run_at`,
  `last_status`, `last_error`) survives restarts. Idempotent
  `Register`, per-job `TryLock` to prevent overlapping runs,
  bounded-grace shutdown, configurable timezone, and a built-in
  `system.housekeeping` job (`PRAGMA optimize` / `VACUUM ANALYZE`
  every 6 hours). New config keys: `scheduler.enabled`,
  `scheduler.timezone`, `scheduler.shutdown_grace`. Migration
  `0005_scheduled_jobs_status.sql` adds `enabled`, `last_status`,
  `last_error` columns on both engines. ADR-0006.
- **Phase 2a — indexer core.** Pluggable indexer abstraction in
  `internal/indexers`: an `Indexer` interface, a concurrency-safe
  `Registry` with bounded fan-out search, an engine-dispatched
  `Repository` over sqlc-generated SQLite and Postgres queries, a
  `Service` orchestrator with hydration and CRUD, a `HealthChecker`
  hosted by the persistent scheduler, and the first kind
  (`builtin/null`) used to exercise the full surface end-to-end.
  HTTP routes under `/api/v1/indexers/*` (list, create, get,
  replace, patch, delete, caps, test, search) with a structured
  `{error:{code,message}}` envelope. Storage migration adds the
  `indexers` and `indexer_health` tables on both engines. New config
  keys: `indexers.search_timeout`, `indexers.max_parallel`,
  `indexers.health_check_schedule`, `indexers.health_check_timeout`.
  ADR-0007.
- **Phase 2c — Newznab + Torznab outbound client.** First-class
  support for querying existing Newznab and Torznab feeds, registered
  under kinds `newznab` and `torznab`. Single-binary client at
  `internal/indexers/newznab` covers `t=caps`, `t=search`,
  `t=tvsearch`, and `t=movie`, with typed error taxonomy
  (`ErrAuthFailed`, `ErrCapsParse`, `ErrRateLimited`, `ErrUpstream`,
  `ErrTimeout`, `ErrMalformedXML`). Caps documents are cached on
  `indexer_health.last_caps_json` so a restart doesn't blank-state
  every indexer; the cache is wired through a new `CapsCache`
  interface (`internal/indexers/caps_cache.go`) with both engine
  implementations. Storage migration `0007_indexer_caps_cache.sql`
  adds the column on both engines (TEXT on SQLite, JSONB on
  Postgres). Operator slip-ups (trailing `/` on URL, embedded
  `?apikey=`) are tolerated by `parseConfig`. Search routes between
  modes based on the inbound `Query` (imdb/tmdb → movie, tvdb /
  season → tvsearch, otherwise plain search). ADR-0008.
- **Phase 2e — Indexer outbound proxies.** New
  `internal/indexers/proxies` package and `proxies` table
  (migration 0008) with full CRUD at `/api/v1/proxies/*`.
  Supported kinds: `http`, `https`, `socks5`, and `flaresolverr`.
  Indexers gain a nullable `proxy_id` FK; setting it routes all
  outbound traffic from that indexer through the matching proxy
  row. The proxies service implements a new
  `indexers.TransportProvider` interface and caches one
  `http.RoundTripper` per proxy ID; the cache is invalidated on
  any Replace/Patch/Delete. The FlareSolverr transport POSTs
  `request.get` to `<URL>/v1` and synthesises a Go `*http.Response`
  from the solution envelope; opt-in `session_mode: shared`
  reuses one FlareSolverr session per proxy row. `DELETE` on a
  proxy that any indexer still pins returns `409 proxy_in_use`
  with `details.indexer_ids` enumerating the blockers.
  `POST /api/v1/proxies/{id}/test` runs a real probe (HTTP fetch
  for HTTP/HTTPS/SOCKS5; `sessions.list` for FlareSolverr). New
  kernel keys `indexers.proxies.flaresolverr_default_timeout` and
  `indexers.proxies.test_probe_url`. New direct dependency:
  `golang.org/x/net/proxy` (SOCKS5). ADR-0009.
- **Documentation baseline.** `docs/` developer documentation
  (architecture, configuration, observability, storage, API,
  development, deployment, security), per-package `doc.go` comments,
  OpenAPI 3.1 spec stub at `api/openapi/loom.yaml`, public roadmap,
  PR template, code-owners placeholder.

### Changed

- **Scheduler API replaced.** The Phase-1a in-memory ticker scheduler
  has been removed. `scheduler.Register` now takes a unique job name,
  a cron expression, a `HandlerFunc(ctx) error`, and an optional
  payload, and persists through a `Store`. Callers must construct the
  scheduler via `scheduler.New(cfg, store, logger, clock)`; the
  binary wires this in `cmd/loom/scheduler.go`.
- **Newznab/Torznab `httpClientFactory` signature.** The internal
  `httpClientFactory` in `internal/indexers/newznab` now takes
  `(cfg, def)` (was `(cfg)`) so it can resolve a per-definition
  `http.RoundTripper` via `indexers.TransportForDefinition`. Affects
  in-tree callers only; no public API change.

### Fixed

- *(none yet)*

### Security

- *(none yet)*

[Unreleased]: https://github.com/loomctl/loom/compare/HEAD
