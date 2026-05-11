# Roadmap

This is the public mirror of Loom's delivery plan. Phases ship
releasable artifacts (alpha → beta → rc → 1.0) and overlap where they
are independent.

Legend: ✅ done · 🚧 in progress · ⏳ planned

---

## ✅ Phase 0 — Repo & engineering foundations

- ✅ Go module, repo skeleton, AGPL-3.0 license
- ✅ CI: GitHub Actions for `go test`, `golangci-lint`, `govulncheck`,
  frontend lint/build
- ✅ Release pipeline: GoReleaser → multi-arch binaries + Docker images
- ✅ ADR directory with architectural decision records
- ✅ Contributor docs: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SECURITY.md`, issue templates

## ✅ Phase 1 — Platform kernel

- ✅ Config: layered (defaults → file → env → flags) with hot-reload
- ✅ Logging: `slog` JSON with PII redaction
- ✅ Telemetry: OpenTelemetry SDK, OTLP, Prometheus `/metrics`, pprof
- ✅ Storage: SQLite with goose migrations (43 migrations), sqlc queries
- ✅ Scheduler: in-memory cron + jittered intervals + one-shot tasks
- ✅ Event bus: in-process channels with async fan-out
- ✅ HTTP server: chi router, request-ID, structured access logs, gzip, ETag
- ✅ Health endpoints: `/healthz`, `/readyz`, `/livez`
- ✅ React app: Vite + TS + TanStack Router/Query + shadcn/ui + Tailwind
- ✅ Auth: forms login (argon2id), session cookies, setup flow

## ✅ Phase 2 — Indexer subsystem (Prowlarr-equivalent)

- ✅ Cardigann YAML engine — 542 bundled indexer definitions
- ✅ Newznab/Torznab protocol support
- ✅ Search aggregation across multiple indexers
- ✅ Proxy + FlareSolverr support (with cookie forwarding)
- ✅ Manual search UI with release dialog
- ✅ Indexer catalogue — searchable table with type/category filters
- ✅ Search result scoring (quality + seeders + age + size, 0–100)
- ✅ Search result filtering (indexer, quality, size, seeders, freeleech)
- ✅ Per-indexer search diagnostics (timing, status, result counts)
- ✅ Freeleech / tracker intelligence (FL/Internal/Scene detection)
- ✅ Cardigann template expansion (Go text/template in search URL paths)

## 🚧 Phase 3 — Download pipeline

- ✅ Download grab flow (magnet URI + NZB)
- ✅ Automated search decision engine (search → score → filter → grab)
- ✅ Auto-search at episode, season, and series level
- ✅ Download activity tracking (live queue, progress, speed, ETA)
- ✅ Active grab tracking (grabbed status on episodes/movies)
- ✅ Hardlink-only import mode (move / hardlink / hardlink_only)
- ✅ Download client CRUD in settings (add/edit/delete with test connection)
- ✅ Search timeout chain (120s default, frontend → handler → registry → HTTP → FlareSolverr)
- ⏳ Full import/post-processing pipeline (detect complete → rename → move/hardlink)
- ⏳ Grab cleanup on import completion
- ⏳ Remote-path mappings, blocklist, redownload-on-failure
- ⏳ Deterministic import behavior
- ⏳ Smarter re-import logic
- ⏳ Smart stalled/failed download handling

## ✅ Phase 4 — Movies module (Radarr-equivalent)

- ✅ Movies service & CRUD API
- ✅ Root folder management
- ✅ Quality profiles & custom formats with scoring
- ✅ Frontend movies page (poster grid, detail modal, search)
- ⏳ RSS sync
- ⏳ Collections & lists
- ⏳ Calendar integration

## ✅ Phase 5 — Series module (Sonarr-equivalent)

- ✅ Series/season/episode data model & API
- ✅ TMDB metadata integration
- ✅ Frontend series page (season accordion, episode table)
- ✅ Episode status display (downloaded, missing, unaired, unmonitored, grabbed)
- ✅ Series library scanning with season-folder support
- ✅ Auto-search buttons per episode/season (direct API call)
- ✅ Interactive search dialog for manual release selection
- ⏳ Anime handling (AniDB/AniList mapping, absolute numbering)
- ⏳ Multi-season pack support
- ⏳ Specials & mini-series handling
- ⏳ Alternate episode ordering (DVD, absolute, etc.)

## ✅ Phase 6 — Notifications & automation

- ✅ Discord, Webhook, Gotify, Ntfy channels
- ✅ Rich notification templates (Go text/template, per-channel overrides)
- ✅ Event bus integration with async fan-out
- ✅ Frontend notifications page (CRUD, test, template editor)
- ⏳ Proactive health alerts
- ⏳ Plugin SDK / custom scripts

## ✅ Phase 7 — Download safety

- ✅ Bad release detection (dangerous extensions, suspicious patterns, size anomalies)
- ✅ Post-download validation (video presence, no executables, no password archives)
- ✅ Manual review queue with approve/reject
- ✅ Frontend safety settings + Activity reviews tab

## ⏳ Phase 8 — Advanced custom formats

- ⏳ AND/OR logic, nested conditions, reusable building blocks
- ⏳ Custom format scoring integration with download decisions
- ⏳ Metadata-aware post-download matching

## ⏳ Phase 9 — Language & international support

- ⏳ Language profiles (priority-based, per-library)
- ⏳ Subtitle/audio track awareness
- ⏳ International release handling (MULTi, dual-audio)

## ⏳ Phase 10 — Long-tail search & scale

- ⏳ Rolling missing search (scheduled, gradual)
- ⏳ Quota-aware API call tracking
- ⏳ Old/rare content search strategy
- ⏳ RSS sync (scheduled feed polling)

## ⏳ Phase 11 — Deployment, hardening & 1.0

- ⏳ Indexer health dashboard (uptime, response times, error rates)
- ⏳ Per-media indexer rules
- ⏳ Distroless multi-arch images, Helm chart
- ⏳ Grafana dashboards, backup/restore CLI
- ⏳ Wire-compatibility: Radarr/Sonarr/Prowlarr API shape parity
- ⏳ Migration tooling: `loom migrate import --from {radarr,sonarr,prowlarr}`
- ⏳ Performance benchmarks, public documentation site, **1.0 release**

## ⏳ Phase 12 — Native torrent client

- ⏳ Embed [anacrolix/torrent](https://github.com/anacrolix/torrent) as built-in BitTorrent engine
- ⏳ Magnet resolution, DHT, PEX, piece verification
- ⏳ Seeding with configurable ratio/time limits
- ⏳ Per-torrent bandwidth control
- ⏳ Durable resume across restarts (piece state persisted)
- ⏳ Workflow engine integration (torrent progress → workflow states)
- ⏳ Storage management (incomplete → complete directory promotion)
- ⏳ UI: active torrents panel (speed, peers, progress, seed status)
- ⏳ Optional — removes need for external torrent client entirely
