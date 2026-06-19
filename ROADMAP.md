# Roadmap

This is the public mirror of Loom's delivery plan. Phases ship
releasable artifacts (alpha → beta → rc → 1.0) and overlap where they
are independent.

Legend: ✅ done · 🚧 in progress · ⏳ planned

> Status reviewed against the codebase. Phases 0–10 are delivered.
> Remaining work is tracked in [GitHub Milestones](https://github.com/Ebenderooock/loom/milestones):
> Plugin SDK (P6), 1.0 hardening (P11), torrent client polish (P12),
> media requests & multi-user (P13), library maintenance tooling (P14),
> and media-server analytics (P15).

## ✅ Beyond the original plan (shipped)

- ✅ Discover section — browse import-list/RSS feeds and add titles manually
  (per-list `auto` vs `discover` mode)
- ✅ Feature-flag system (admin-toggleable capabilities; e.g. Search Log)
- ✅ Global quick-search command palette (live movie/series search + deep links)
- ✅ Wire-compatibility shims (Radarr v3 / Sonarr v3 / Prowlarr v1) so Overseerr,
  Ombi, etc. can talk to Loom
- ✅ Migration importers (`loom migrate import --from {radarr,sonarr,prowlarr}`)

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

## ✅ Phase 3 — Download pipeline

- ✅ Download grab flow (magnet URI + NZB)
- ✅ Automated search decision engine (search → score → filter → grab)
- ✅ Auto-search at episode, season, and series level
- ✅ Download activity tracking (live queue, progress, speed, ETA)
- ✅ Active grab tracking (grabbed status on episodes/movies)
- ✅ Hardlink-only import mode (move / hardlink / hardlink_only)
- ✅ Download client CRUD in settings (add/edit/delete with test connection)
- ✅ Search timeout chain (120s default, frontend → handler → registry → HTTP → FlareSolverr)
- ✅ Full import/post-processing pipeline (detect complete → rename → move/hardlink)
- ✅ Grab cleanup on import completion
- ✅ Remote-path mappings, blocklist, redownload-on-failure
- ✅ Deterministic import behavior
- ✅ Smarter re-import logic
- ✅ Smart stalled/failed download handling

## ✅ Phase 4 — Movies module (Radarr-equivalent)

- ✅ Movies service & CRUD API
- ✅ Root folder management
- ✅ Quality profiles & custom formats with scoring
- ✅ Frontend movies page (poster grid, detail modal, search)
- ✅ RSS sync
- ✅ Collections & lists
- ✅ Calendar integration

## ✅ Phase 5 — Series module (Sonarr-equivalent)

- ✅ Series/season/episode data model & API
- ✅ TMDB metadata integration
- ✅ Frontend series page (season accordion, episode table)
- ✅ Episode status display (downloaded, missing, unaired, unmonitored, grabbed)
- ✅ Series library scanning with season-folder support
- ✅ Auto-search buttons per episode/season (direct API call)
- ✅ Interactive search dialog for manual release selection
- ✅ Anime handling (AniDB/AniList mapping, absolute numbering)
- ✅ Multi-season pack support
- ✅ Specials & mini-series handling
- ✅ Alternate episode ordering (DVD, absolute, etc.)

## ✅ Phase 6 — Notifications & automation

- ✅ Discord, Webhook, Gotify, Ntfy channels
- ✅ Rich notification templates (Go text/template, per-channel overrides)
- ✅ Event bus integration with async fan-out
- ✅ Frontend notifications page (CRUD, test, template editor)
- ✅ Proactive health alerts
- ⏳ Plugin SDK / custom scripts

## ✅ Phase 7 — Download safety

- ✅ Bad release detection (dangerous extensions, suspicious patterns, size anomalies)
- ✅ Post-download validation (video presence, no executables, no password archives)
- ✅ Manual review queue with approve/reject
- ✅ Frontend safety settings + Activity reviews tab

## ✅ Phase 8 — Advanced custom formats

- ✅ AND/OR logic, nested conditions, reusable building blocks
- ✅ Custom format scoring integration with download decisions
- ✅ Metadata-aware post-download matching

## ✅ Phase 9 — Language & international support

- ✅ Language profiles (priority-based, per-library)
- ✅ Subtitle/audio track awareness
- ✅ International release handling (MULTi, dual-audio)

## ✅ Phase 10 — Long-tail search & scale

- ✅ Rolling missing search (scheduled, gradual)
- ✅ Quota-aware API call tracking
- ✅ Old/rare content search strategy
- ✅ RSS sync (scheduled feed polling)

## 🚧 Phase 11 — Deployment, hardening & 1.0

- ✅ Indexer health dashboard (uptime, response times, error rates)
- ✅ Per-media indexer rules
- ✅ Distroless multi-arch images, Helm chart
- ✅ Grafana dashboards, backup/restore CLI
- ✅ Wire-compatibility: Radarr/Sonarr/Prowlarr API shape parity
- ✅ Migration tooling: `loom migrate import --from {radarr,sonarr,prowlarr}`
- ⏳ Performance benchmarks, public documentation site, **1.0 release**

## 🚧 Phase 12 — Native torrent client

- ✅ Embed [anacrolix/torrent](https://github.com/anacrolix/torrent) as built-in BitTorrent engine
- ✅ Magnet resolution, DHT, PEX, piece verification
- ✅ Seeding with configurable ratio/time limits (30s supervisor tick)
- ✅ Singleton engine architecture — one `torrent.Client` per process, shared across definitions
- ✅ Durable resume across restarts (BoltDB piece completion state)
- ✅ Workflow engine integration (monitor treats seeding/completed as import-ready)
- ✅ Storage management (incomplete dir support, FreeSpace via syscall.Statfs)
- ✅ UI: download form with torrent-specific config (dirs, port, DHT/PEX/UPnP, seed limits, speed limits)
- ✅ Active torrents visible in existing Downloads page (pause/resume/remove, progress, speeds, ratio)
- ✅ Removes need for external torrent client entirely
- ⏳ Per-indexer seed ratio overrides
- ⏳ Dedicated torrent detail panel (peers, trackers, file list)

## ⏳ Phase 13 — Media requests (Requestarr/Ombi/Overseerr-equivalent)

- ⏳ User request portal (web UI for friends/family to request movies & series)
- ⏳ Discord & Telegram bot integration for request submission
- ⏳ Approval workflows (auto-approve, manual, quota-based)
- ⏳ Per-user quotas and request limits
- ⏳ Auto-search on approval (triggers workflow engine)
- ⏳ Request status tracking & notifications to requestors
- ⏳ Multi-user auth (invite links, OAuth, or simple PIN-based access)

## 🚧 Phase 14 — Library maintenance (Cleanarr/Maintainerr-equivalent)

- ⏳ Duplicate detection (same media, multiple files/qualities)
- ⏳ Unmatched media identification (files not linked to any library entry)
- ✅ Orphaned download cleanup (completed downloads never imported)
- ⏳ Quality upgrade cleanup (remove lower quality when upgrade exists)
- ⏳ Empty folder pruning
- 🚧 Disk space analysis & recommendations (analysis via health monitor; no recommendations UI)
- ⏳ Dry-run mode with approval queue before destructive actions
- ⏳ Scheduled maintenance runs (configurable frequency)

## 🚧 Phase 15 — Media server analytics (Tautulli-equivalent)

- ✅ Plex / Jellyfin / Emby integration (play session tracking)
- ⏳ User activity & concurrent stream monitoring
- ⏳ Most/least watched content reports
- ⏳ Watch history per user & per media item
- ⏳ Smart library decisions (flag unwatched content for removal)
- ⏳ Bandwidth & transcoding stats
- ⏳ Dashboard widgets (graphs, top content, active streams)
- ⏳ Notification triggers on activity (new user, stream started, etc.)

## ⏳ Phase 16 — Music support (Lidarr-equivalent)

- ⏳ Artist/album/track data model & API
- ⏳ MusicBrainz metadata integration
- ⏳ Audio quality profiles (formats, bitrate, lossless)
- ⏳ Monitored artists & albums, missing detection
- ⏳ Release search, scoring & grab (reuses indexer/download pipeline)
- ⏳ Import, rename & library scanning (artist/album folders)
- ⏳ Music library UI (artist grid, album detail, track table)
- ⏳ Calendar & RSS/list integration for new releases

## ⏳ Phase 17 — Books support (Readarr-equivalent)

- ⏳ Author/book/edition data model & API
- ⏳ Metadata integration (Goodreads / OpenLibrary)
- ⏳ eBook & audiobook quality profiles (epub/mobi/pdf, m4b/mp3)
- ⏳ Monitored authors & books, missing detection
- ⏳ Release search, scoring & grab (reuses indexer/download pipeline)
- ⏳ Import, rename & library scanning (author/book folders)
- ⏳ Books library UI (author grid, book detail, edition selection)
- ⏳ Calendar & RSS/list integration for new releases

## ⏳ Phase 18 — Synology (SynoCommunity) package

- ⏳ `cross/loom` cross-compile recipe (Go, `CGO_ENABLED=0`, all DSM archs)
- ⏳ `spk/loom` package definition (icon, service, ports, AGPL-3.0)
- ⏳ `service-setup.sh` (run `loom serve`, data dir in `${SYNOPKG_PKGVAR}`, DSM TLS)
- ⏳ DSM 7 compliance (non-root user, privilege/resource, firewall ports)
- ⏳ PR to SynoCommunity/spksrc, pass CI & maintainer review
- ⏳ Document Synology install in README/docs
