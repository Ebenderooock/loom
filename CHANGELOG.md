# Changelog

All notable changes to Loom are documented here.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.1] - 2026-06-09

### Added

- **Music capability (Lidarr parity).** A full Music media domain —
  Artist → Album → Track — alongside Movies and TV, enabled by default. Backed by
  MusicBrainz + Cover Art Archive metadata (no API key). Includes monitored
  artists/albums with track-level completeness, audio quality profiles with
  custom-format scoring, metadata-profile filtering, a music-aware
  scanner/organizer/import pipeline, automated search/grab via the shared indexer
  and download plumbing, per-track cutoff-unmet upgrades, RSS new-release
  discovery, music import lists, a `/music` UI (artist grid + artist/album detail,
  interactive search & grab, bulk monitor/search), calendar release events, and
  artist requests through the requests portal and the Telegram/Discord chat bots
  (with per-user music quota and chat-approval defaults). Database migrations
  0069–0073 (sqlite + postgres).

- **Stateful workflow engine** (`internal/workflows/`). Replaces the old stateless
  "grabs" package with a full state machine tracking search → grab → download →
  import → completion. Includes automatic retry with exponential backoff (up to 3
  attempts), stale-download detection (4h timeout), and self-healing that resets
  media to "missing" on final failure so auto-search can re-acquire. REST API at
  `/api/v1/workflows` (list, get, cancel, retry, delete). Background scheduler
  handles stale detection, retry processing, and completed-workflow pruning.
  See [docs/workflows.md](docs/workflows.md).

- **Phase 5c — Custom formats with Radarr-compatible scoring.** Extends Phase 5b (quality
  definitions/profiles) with flexible release scoring rules. Implements
  `internal/movies/custom_formats.go` service layer with `ValidateCustomFormat()`,
  `EvaluateCustomFormats()`, and stateless filter matching. Eight condition types:
  equals, regex, range, in, gt, gte, lt, lte. Eight allowed fields: codec, source,
  year, bitdepth, resolution, hdr, audio, language. Filters use implicit AND logic
  (all must match). Score formula: FinalScore = (quality_tier_order × 100) +
  sum(matching_custom_format_scores). Database migration `0014_custom_formats.sql`
  adds `custom_formats` and `custom_format_filters` tables with soft-delete support
  and CASCADE delete for atomicity. Repository layer implements full CRUD
  (AddCustomFormat, GetCustomFormat, UpdateCustomFormat, DeleteCustomFormat,
  ListCustomFormats, GetCustomFormatByName). Service integration delegates to
  custom format service. HTTP handlers: 6 endpoints (list, create, get by ID,
  update, delete, test). OpenAPI spec updated with CustomFormat, CustomFormatFilter,
  and request/response schemas. Comprehensive unit tests (15+) cover field
  allowlist, condition validation, filter matching, edge cases, ReDoS protection.
  See [ADR-0026](docs/adr/0026-custom-formats-architecture.md) for design
  rationale and future work (Phase 5d: release-name parser for numeric conditions,
  Phase 5e: OR logic support, performance optimization). All tests pass `-race`.

- **Phase 4e — Metadata orchestration router.** Builds on Phase 4a–4d
  (metadata service + providers TMDB, TVDB, MusicBrainz) with a concurrent
  fan-out layer that returns the first successful metadata match within a
  10-second total timeout (3s per provider, parallel execution). New
  `internal/metadata/router.go` package implements `ResolveMovie()`,
  `ResolveSeries()`, `ResolveEpisode()` methods that try external ID
  lookups first, then fall back to title-based search. Uses `errgroup.Group`
  for concurrent provider queries; avoids sequential bottleneck (3 providers
  at 1s each = ~1s parallel, not 3s sequential). Router integrates with
  downloads module: after successfully queuing a download, calls metadata
  router in a background goroutine (non-blocking) to enrich with movie/series
  metadata, emitting `TopicMetadataEnriched` or `TopicMetadataFailure`
  events to downstream subscribers (search indexes, release tracking).
  New event types: `MetadataEnrichedEvent` (download + matched metadata),
  `MetadataFailureEvent` (download + reason). Configuration via env vars:
  `LOOM_METADATA_PROVIDERS` (comma-separated list, default:
  tmdb,tvdb,musicbrainz), `LOOM_METADATA_TIMEOUT` (default: 10s),
  `LOOM_METADATA_CACHE_ENABLED` (default: true). Downloads router enhanced
  with optional metadata router field; on successful download queue,
  spawns background enrichment goroutine that publishes metadata events.
  All router and downloads changes pass `-race` flag. See
  [ADR-0025](docs/adr/0025-metadata-router-integration.md) for fan-out
  rationale (why concurrent beats sequential, why total timeout beats
  per-provider timeout) and integration pattern (non-blocking background
  enrichment). Docs updated in `docs/metadata.md` with router section.
  Tests: 10+ new tests covering resolve by ID, search fallback, no match,
  timeout handling, partial results (first succeeds, others slow), concurrent
  race-safe operations, config loading from env vars; all pass `-race`.

- **Phase 4a — Metadata service foundation.** Pluggable abstraction layer
  for movie, series, and episode metadata sourced from external providers
  (TMDB, TVDB, MusicBrainz). New `internal/metadata/` package implements
  a three-tier lookup strategy: in-process cache (TTL 30min for search
  results, 7d for full details, cleaned every 5min), persistent SQL
  repository (SQLite/Postgres, append-mostly with soft expiration),
  and pluggable provider interface (`MetadataProvider` with
  `FindMovie()`, `FindSeries()`, `FindEpisode()`). Service is
  non-blocking (3s per provider, 10s total timeout) and returns first
  successful result or nil on timeout. Metadata is immutable once cached;
  updates require explicit refresh. All types: `MovieMetadata`,
  `SeriesMetadata`, `EpisodeMetadata` carry external IDs (TMDB, IMDB,
  TVDB) as optional pointers for flexible lookup. Repository is
  engine-neutral; implementations dispatch to raw SQL for both SQLite
  and Postgres. Cache uses `sync.Mutex` and is safe for concurrent use.
  Migration 0011 creates `metadata_movies`, `metadata_series`,
  `metadata_episodes` tables with indexes on external IDs. See
  [ADR-0021](docs/adr/0021-metadata-abstraction.md) for design rationale
  and [metadata.md](docs/metadata.md) for integration patterns. Tests:
  13 cases passing with `-race` flag (cache TTL, repository CRUD, service
  lookup chain, timeouts, concurrency). Deferred to Phase 5+: Redis
  caching, provider fallback strategies, metadata refresh webhooks,
  confidence scoring.

- **Phase 3g — Download routing and monitoring.** Bridges indexer intake
  pipeline and download clients via two interconnected services. `Router`
  subscribes to indexer results, applies quality filtering (simple
  seeder-based heuristics for Phase 3; full semantic rules deferred to
  Phase 5), and queues high-quality results on configured download clients
  in priority order. `Monitor` periodically polls clients for status
  updates and emits completion events with duplicate prevention. Both
  services emit typed events (`TopicDownloadQueued`, `TopicDownloadFailed`,
  `TopicDownloadCompleted`) on the event bus for downstream consumption.
  Quality filter: rejects torrents with 0 seeders, accepts Usenet results
  (nil seeders), accepts torrents with >0 seeders. New event types:
  `IndexerResultEvent` wraps `indexers.Result` for bus compatibility;
  `DownloadQueuedEvent`, `DownloadFailureEvent`, `DownloadCompletedEvent`
  carry origin result IDs, client IDs, download IDs, and timestamps. See
  [ADR-0020](docs/adr/0020-download-routing-and-monitoring.md) for design
  rationale and limitations (ClientID inference, flat item tracking).

- **Phase 3f — NZBGet download client.** Second Usenet driver on the
  Phase 3a download-client abstraction. New
  `internal/downloads/nzbget/` package speaks NZBGet's JSON-RPC 2.0
  endpoint at `<base_path>/jsonrpc` (NZBGet 21+), authenticating
  with HTTP Basic (`ControlUsername`/`ControlPassword`) on every
  request and surfacing JSON-RPC `error` envelopes as typed errors
  (`ErrAuth`, `ErrServer`, `ErrNotFound`, `ErrUpstream`,
  `ErrMissingNZBID`, `ErrMalformedNZB`, `ErrConfig`). `Add` covers
  both URL fetches (URL passed as `NZBFilename` with empty
  `NZBContent`, server-side fetch on NZBGet 17+) and raw `.nzb`
  uploads (base64-encoded `NZBContent`), via NZBGet's positional
  `append(NZBFilename, NZBContent, Category, Priority, AddToTop,
  AddPaused, DupeKey, DupeScore, DupeMode, PostProcessParameters)`
  signature; tag conventions cover `priority`, `add_to_top`,
  `add_paused`, `dupekey`, `dupescore`, `dupemode`, and
  `pp_<name>=<value>` post-process pass-through. `Status` merges
  `listgroups(0)` and `history(false)` into a single
  `[]downloads.Item`, mapping NZBGet's 17-string queue + history
  vocabulary onto Loom's `ItemStatus` enum in two audited tables;
  `Pause`/`Resume` dispatch on the id list (global
  `pausedownload`/`resumedownload` for empty, `editqueue` with
  `GroupPause`/`GroupResume` for specific ids); `Remove`
  distinguishes `GroupDelete` (preserves history + on-disk bytes)
  from `GroupFinalDelete` (purges both) on `deleteFiles`;
  `Categories` parses `Category{N}.Name`/`Category{N}.DestDir` from
  NZBGet's `config()` RPC with a 30s in-process TTL cache that
  rides out transient errors; `FreeSpace` reads
  `status().FreeDiskSpaceMB` (binary MB→bytes) returning `-1` when
  absent; `Test` issues `version()`. Outbound HTTP composes the
  same proxy + throttle stack as indexers via
  `downloads.TransportForDefinition`. OpenAPI adds an
  `NzbgetConfig` schema documenting the recognised `config` blob.
  XML-RPC fallback, server-side category writes, history retention
  controls, speed-limit control, and TLS-skip are deferred. See
  [`docs/downloads-nzbget.md`](docs/downloads-nzbget.md) and
  [ADR-0019](docs/adr/0019-nzbget-download-client-kind.md).

- **Phase 3b — qBittorrent download client.** First real driver on the
  Phase 3a download-client abstraction. New
  `internal/downloads/qbittorrent/` package speaks the qBittorrent v2
  Web API (qBittorrent 4.1+ and 5.x), authenticates via cookie-based
  `/auth/login`, persists the SID in an `http.CookieJar`, and
  transparently re-logs in once on a 403. Supports magnet / raw
  `.torrent` / URL adds via `/torrents/add` (multipart), with the v1
  infohash returned client-side as the item id. `Status` maps the
  qBittorrent state vocabulary onto Loom's `ItemStatus` enum in a
  single audited table; `Pause`, `Resume`, `Remove(deleteFiles)`,
  `Categories`, `FreeSpace`, and `Test` round-trip the matching v2
  endpoints. Outbound HTTP composes the same proxy + throttle stack
  as indexers via `downloads.TransportForDefinition`. `OpenAPI` adds
  a `QbittorrentConfig` schema documenting the recognised `config`
  blob. See [`docs/downloads-qbittorrent.md`](docs/downloads-qbittorrent.md)
  and [ADR-0015](docs/adr/0015-qbittorrent-download-client.md).

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

[Unreleased]: https://github.com/ebenderooock/loom/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/ebenderooock/loom/compare/v0.1.0...v0.1.1
