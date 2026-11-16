# Roadmap

Loom is **alpha**. This roadmap records implemented capabilities and remaining
work, not production-readiness or complete parity with the apps named below.
Phase numbers are historical groupings, not a strict delivery order.

**Reviewed: 2026-11-17**, against `main` at `a2c53497` and the published
[`v0.4.1` tag](https://github.com/Ebenderooock/loom/tree/v0.4.1)
(`00e94edc`). Release notes alone are not a capability inventory: later changes
on `main` are not retroactively included in that release.
The Synology workflow entry also records additional work in the
`prelease/0.4.2` stack.

Legend: ✅ implemented in the reviewed source · 🚧 partial / experimental ·
⏳ planned. A checkmark does not mean every provider, platform, or edge case
has been verified in a live deployment.

**Usenet/NNTP has not been tested at all.** Newznab/NZB and external
SABnzbd/NZBGet integrations exist in source, but are unverified, not known-working
workflows. A built-in Usenet/NNTP downloader is still planned.

## What can be tested?

| Area | Published `v0.4.1` | Additional work on reviewed `main` (not in `v0.4.1`) |
|---|---|---|
| Movies and TV | Library management, indexer search, grabs, download monitoring, imports, quality profiles, calendar | Show/season/specials monitoring fixes; durable per-file import queue, asynchronous import scans, collision/replacement and quality-profile wiring fixes |
| Requests and collections | Authenticated movie/series/artist requests, quotas and approval; Discord/Telegram bots; mixed movie/TV collections and discovery | Collection live-status and request-availability reconciliation refinements; music-request acquisition improvements |
| Music | Feature-gated artist/album/track catalog, MusicBrainz lookups, search and scanning; acquisition/import integration is incomplete | Music download imports, track-level acquisition, expanded profile management, standalone albums, manual audio imports and richer artwork/details |
| Library health and integrations | Read-only health reports; opt-in media analytics, JavaScript event plugins and Rain-sidecar torrent integration | External-client polling/backoff fixes; see the limitations in each phase below |
| Deployment and UI | Linux/macOS release archives, container deployment and basic Synology recipe | Native service installers and Windows build targets, expanded Synology packaging, global palettes/background uploads and navigation animation |

For useful end-to-end import and music feedback, use a build containing the
relevant `main` fixes and record its exact commit. Do not assume a `latest`
container or an old release includes them. Use a separate test instance and
disposable/copied media; imports and cleanup can change files. Back up state
before upgrades, and never point two automation tools at the same writable
library during testing. **Disable orphaned-download auto-delete before testing
cleanup**: it defaults to enabled with seven-day retention, and manual approval
also deletes immediately (recycling only when configured).

See [deployment](docs/deployment.md), [music](docs/music.md),
[requests](docs/requests.md), and the [unreleased changelog](CHANGELOG.md#unreleased)
for setup and release boundaries. Use
[Discussions Q&A](https://github.com/Ebenderooock/loom/discussions/categories/q-a)
for questions and [issues](https://github.com/Ebenderooock/loom/issues/new/choose)
for reproducible bugs. [Milestones](https://github.com/Ebenderooock/loom/milestones)
and [design proposals](docs/roadmap/README.md) track follow-on work, but an open
tracking issue does not mean its entire feature is absent.

## ✅ Beyond the original plan (implemented)

- ✅ Discover section — browse import-list/RSS feeds and add titles manually
  (per-list `auto` vs `discover` mode)
- ✅ Feature-flag system (admin-toggleable capabilities; e.g. Search Log)
- ✅ Global quick-search command palette (live movie/series search + deep links)
- 🚧 Selected compatibility endpoints for Radarr v3 / Sonarr v3 / Prowlarr v1;
  not full downstream-client compatibility (see Phase 11)
- 🚧 SQLite migration importers (`loom migrate --from {radarr,sonarr,prowlarr}
  --db <source.db>`); limited entity coverage, not a lossless stack migration
- ✅ Persistent mixed movie/TV collections with source imports, explicit
  membership management and explained library-expansion suggestions
- 🚧 Experimental Android TV companion: device pairing and request/browse
  flows, not a media playback client

---

## ✅ Phase 0 — Repo & engineering foundations

- ✅ Go module, repo skeleton, AGPL-3.0 license
- ✅ CI: GitHub Actions for `go test`, `golangci-lint`, `govulncheck`,
  frontend lint/build
- ✅ Release artifacts: binaries, source archive and multi-arch container images;
  current publishing authority lives in the separate
  [`loom-publisher`](https://github.com/Ebenderooock/loom-publisher) repository
- ✅ ADR directory with architectural decision records
- ✅ Contributor docs: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SECURITY.md`, issue templates

## ✅ Phase 1 — Platform kernel

- ✅ Config: layered (defaults → file → env → flags) with hot-reload
- ✅ Logging: `slog` JSON with PII redaction
- ✅ Telemetry: OpenTelemetry SDK, OTLP, Prometheus `/metrics`, pprof
- ✅ Storage: SQLite (default) and Postgres, goose migrations, sqlc queries
- ✅ Scheduler: in-memory cron + jittered intervals + one-shot tasks
- ✅ Event bus: in-process channels with async fan-out
- ✅ HTTP server: chi router, request-ID, structured access logs, gzip, ETag
- ✅ Health endpoints: `/healthz`, `/readyz`, `/livez`
- ✅ React app: Vite + TS + TanStack Router/Query + shadcn/ui + Tailwind
- ✅ Auth: forms login (argon2id), sessions, API keys, users/invites, OIDC and
  opt-in trusted reverse-proxy authentication

## ✅ Phase 2 — Indexer subsystem (Prowlarr-equivalent)

- ✅ Cardigann YAML engine and bundled indexer catalog (individual sites still
  require compatible definitions, credentials and live testing)
- ✅ Newznab/Torznab protocol implementation; Usenet/Newznab remains untested
- ✅ Search aggregation across multiple indexers
- ✅ Proxy + FlareSolverr support (with cookie forwarding)
- ✅ Manual search UI with release dialog
- ✅ Indexer catalogue — searchable table with type/category filters
- ✅ Search result scoring (quality + seeders + age + size, 0–100)
- ✅ Search result filtering (indexer, quality, size, seeders, freeleech)
- ✅ Per-indexer search diagnostics (timing, status, result counts)
- ✅ Persistent provider budgets, shared-account groups and rate/reset signals
- ✅ Freeleech / tracker intelligence (FL/Internal/Scene detection)
- ✅ Cardigann template expansion (Go text/template in search URL paths)

## 🚧 Phase 3 — Download pipeline

- ✅ Download grab flow (magnet URI + NZB); the Usenet/NZB path is untested
- ✅ Automated search decision engine (search → score → filter → grab)
- ✅ Auto-search at episode, season, and series level
- ✅ Download activity tracking (live queue, progress, speed, ETA)
- ✅ Active grab tracking (grabbed status on episodes/movies)
- ✅ Hardlink-only import mode (move / hardlink / hardlink_only)
- ✅ Download client CRUD in settings (add/edit/delete with test connection)
- ✅ Search timeout chain (120s default, frontend → handler → registry → HTTP → FlareSolverr)
- ✅ Wired import/post-processing pipeline (detect complete → rename → move/hardlink)
- ✅ Grab cleanup on import completion
- ✅ Remote-path mappings, blocklist, redownload-on-failure
- ✅ Collision-aware import decisions and replacement checks
- ✅ Smarter re-import logic
- ✅ Smart stalled/failed download handling
- ✅ Durable acquisition failures, bounded retry history and import recovery
- ✅ On reviewed `main`: persistent per-file import jobs, retry/progress UI and
  asynchronous scans; these and quality-profile/replacement fixes postdate `v0.4.1`
- 🚧 End-to-end reliability across client outages, restarts, storage layouts and
  unusual releases remains an alpha-testing focus, not a guarantee

## ✅ Phase 4 — Movies module (Radarr-equivalent)

- ✅ Movies service & CRUD API
- ✅ Root folder management
- ✅ Quality profiles & custom formats with scoring
- ✅ Frontend movies page (poster grid, detail modal, search)
- ✅ RSS sync
- ✅ Import Lists, TMDB movie collections, and explained library-expansion suggestions
- ✅ Calendar integration

## 🚧 Phase 5 — Series module

- ✅ Series/season/episode data model & API
- ✅ TMDB metadata integration
- ✅ Frontend series page (season accordion, episode table)
- ✅ Episode status display (downloaded, missing, unaired, unmonitored, grabbed)
- ✅ Series library scanning with season-folder support
- ✅ Auto-search buttons per episode/season (direct API call)
- ✅ Interactive search dialog for manual release selection
- 🚧 Anime parser and mapping storage/API exist; automatic AniDB/AniList
  catalog-to-import mapping is not a complete wired workflow
- 🚧 Season-pack detection and per-file episode matching; automatic search
  deliberately rejects ambiguous multi-season / complete-series packs
- ✅ Specials & mini-series handling
- ⏳ End-to-end alternate episode ordering (DVD, absolute, etc.); resolver/store
  helpers exist but are not wired into the production acquisition/import flow

## 🚧 Phase 6 — Notifications & automation

- ✅ Discord, Webhook, Slack, Email, Telegram, Gotify, Pushover, Ntfy and Apprise channels
- ✅ Rich notification templates (Go text/template, per-channel overrides)
- ✅ Event bus integration with async fan-out
- ✅ Frontend notifications page (CRUD, test, template editor)
- ✅ Proactive health alerts
- ✅ Admin-managed JavaScript event plugins: editor/type definitions, event
  subscriptions, test runs and run history (`plugins` feature flag)
- ⏳ Formal external developer SDK, packaged plugin discovery/distribution
  and script marketplace; the event-script runtime is not a general shell runner

## ✅ Phase 7 — Download safety

- ✅ Release heuristics (dangerous extensions, suspicious patterns, size anomalies)
- ✅ Post-download checks for media presence, executable extensions and suspicious
  archive names; not antivirus scanning or proof that an archive has no password
- ✅ Manual review queue with approve/reject
- ✅ Frontend safety settings + Activity reviews tab

## 🚧 Phase 8 — Advanced custom formats

- ✅ Grouped AND/OR specifications, required/negated conditions and presets
- ✅ Custom format scoring integration with download decisions
- 🚧 File-metadata helpers currently derive properties from filenames and stat;
  full media-probed post-download custom-format enforcement is not implemented
- ⏳ Arbitrarily nested condition trees

## 🚧 Phase 9 — Language & international support

- ✅ Language-profile management and release-name language/MULTi/dual-audio parsing
- ✅ Language specifications in release custom formats
- 🚧 Standalone language scoring helpers exist; per-library language-profile
  enforcement is not wired throughout acquisition/import
- ⏳ Full subtitle/audio stream probing and enforcement (release-name hints are
  not proof of actual tracks)

## ✅ Phase 10 — Long-tail search & scale

- ✅ Rolling missing search (scheduled, gradual)
- ✅ Quota-aware API call tracking
- ✅ Old/rare content search strategy
- ✅ RSS sync (scheduled feed polling)

## 🚧 Phase 11 — Deployment, hardening & 1.0

- ✅ Indexer health dashboard (uptime, response times, error rates)
- ✅ Per-media indexer rules
- ✅ Distroless multi-arch images, Helm chart
- ✅ Grafana dashboards and SQLite/config backup/restore CLI; stop the server
  for offline copies/restores and check the documented path/layout limits
- 🚧 Selected `/compat/radarr`, `/compat/sonarr`, `/compat/prowlarr` routes;
  command and indexer/application-sync endpoints include acknowledgment-only
  handlers, so API shape does not imply the requested action is performed
- 🚧 SQLite migration tooling for selected catalog/profile/root-folder/indexer
  records; not all settings/history are imported, no dry-run mode or lossless
  cutover guarantee (see [migration guide](docs/migration.md))
- ✅ On reviewed `main`: native service installers and expanded deployment guides
- ⏳ Performance benchmarking, documentation coverage and **1.0 hardening/release**

## 🚧 Phase 12 — Built-in torrent workflow (Rain sidecar)

- ✅ `builtin/torrent` routed through internal Rain JSON-RPC sidecar
- ✅ Magnet/torrent add, pause/resume/remove, reannounce/recheck integration
- ✅ Workflow engine integration (monitor treats seeding/completed as import-ready)
- ✅ UI: torrent management on existing Downloads page
- ✅ Helm support for Rain sidecar deployment
- ✅ Torrent detail panel: overview, peers, trackers and file list
- 🚧 Requires the feature flag and a reachable Rain JSON-RPC sidecar; release
  images do not bundle the Rain executable, so the optional Helm sidecar needs
  a separately built Rain-capable image
- ⏳ Per-indexer seed-ratio enforcement
- 🚧 Runtime speed-limit/per-item control enforcement needs validation; the
  speed-limit setter currently updates cached values, not the Rain RPC server

## 🚧 Phase 13 — Media requests & multi-user

- ✅ Authenticated movie, series and artist request portal
- ✅ Discord and Telegram request bots with account linking and admin review
- ✅ Administrator approval/rejection and per-user rolling quotas
- ✅ Approval adds media and starts acquisition; music integration is expanded
  on `main` after `v0.4.1`
- ✅ Request status and separate file-presence/fulfillment history, with
  explicit missing-media re-requests and storage-aware reconciliation
- ✅ Password users, admin user management, invite links and configured OIDC
- ⏳ General auto-approval policies; quotas currently limit requests, not approve them
- ⏳ Proactive requestor decision notifications; the optional notifier is not
  connected in production wiring
- ⏳ Guest requests, comments and bulk decisions
- ⏳ External media-server availability checks; current availability is based
  on Loom's imported files, not Plex/Jellyfin/Emby playback readiness

## 🚧 Phase 14 — Library maintenance (Cleanarr/Maintainerr-equivalent)

- ⏳ Independent permanent version tracks — [proposed ADR-0027](docs/adr/0027-independent-version-tracks.md)
  for concurrent 1080p/4K ownership and migration; design only, no shipped tracks
- ✅ Read-only library-health reports: untracked files, missing paths, possible
  duplicates and disk-space evidence; findings are not automatic reconciliation
- 🚧 Duplicate/unmatched-file findings need review; no general automatic resolution
- ✅ Orphaned-download scan/review/ignore/approve and retention-based cleanup
- 🚧 Orphan auto-delete defaults to enabled with seven-day retention; approval
  deletes immediately, using the recycle bin only when configured. Test only
  disposable download paths with auto-delete disabled
- ✅ Explicit file management/recycling/deletion from movie/show/season details
- ⏳ General scheduled quality-upgrade cleanup (separate from import replacement)
- ⏳ Empty folder pruning
- 🚧 Disk-space analysis exists; broader storage recommendations remain
- 🚧 Download-orphan review exists, not a general dry-run/action-approval system
- 🚧 Fixed six-hour orphan-cleanup sweep exists; configurable schedules and a
  general maintenance engine remain planned

## 🚧 Phase 15 — Media server analytics (Tautulli-equivalent)

- ✅ Feature-gated Plex/Jellyfin/Emby active-session polling and concurrent
  stream/user visibility
- ✅ Analytics page: active streams, top users, most watched, plays per day
  and recent observed history
- 🚧 Least-watched and per-user/media history have backend support but not
  complete dedicated browsing UI
- ⏳ Smart library decisions (flag unwatched content for removal)
- 🚧 Direct/transcode counts and reported bitrate exist; not comprehensive
  bandwidth accounting or transcoder diagnostics
- ✅ Playback started/stopped notification events; new-user triggers remain planned
- 🚧 Samples sessions every 30 seconds; reports reflect observed sessions,
  not a media server's complete historical log. Aggregate plays require at
  least 60 seconds, and short/missed sessions may be absent

## 🚧 Phase 16 — Music support

- ✅ Artist/album/release/track model, MusicBrainz metadata and audio profiles
- ✅ Artist/album monitoring, release search/scoring/grab and explicit rescans
- ✅ On reviewed `main`: completed-download and manual audio import with
  physical library transfers, per-track monitoring/search and upgrades
- ✅ On reviewed `main`: standalone/compilation albums with credited artists,
  edition selection, audio/metadata profile management and richer detail UI
- ✅ On reviewed `main`: configurable MusicBrainz-compatible metadata source
  and optional fanart.tv/TheAudioDB artwork
- ✅ Music calendar entries, MusicBrainz collection-list artist imports and
  scheduled release refresh; these are not a dedicated music RSS feed
- 🚧 Feature-gated (`music`, enabled by default); tagged `v0.4.1` has core
  catalog/search/scanning but not the newer music import integration
- 🚧 Tag/catalog matching, provider rate limits and ambiguous files need
  real-world validation; use explicit rescans and manual resolution when needed
- ⏳ Dedicated music RSS acquisition workflow and broader coverage validation

## ⏳ Phase 17 — Books support

- ⏳ Author/book/edition data model & API
- ⏳ Metadata integration (Goodreads / OpenLibrary)
- ⏳ eBook & audiobook quality profiles (epub/mobi/pdf, m4b/mp3)
- ⏳ Monitored authors & books, missing detection
- ⏳ Release search, scoring & grab (reuses indexer/download pipeline)
- ⏳ Import, rename & library scanning (author/book folders)
- ⏳ Books library UI (author grid, book detail, edition selection)
- ⏳ Calendar & RSS/list integration for new releases

## 🚧 Phase 18 — Synology (SynoCommunity) package

- ✅ `cross/loom` build recipe and `spk/loom` package definition, service, icon
  and firewall configuration
- ✅ On reviewed `main`: DSM service state/permission checks, shared-folder
  install/upgrade wizard and documented manual `.spk` / Container Manager setup
- ✅ On the `prelease/0.4.2` stack: separate GitHub Actions workflow for x64
  and aarch64 package/build-input artifacts; these are workflow artifacts,
  not attached release assets
- 🚧 NAS architecture/DSM matrix validation and upstream catalog publication
  remain separate; not all DSM architectures are supported
- ⏳ Ready-made `.spk` assets in Loom releases; the existing release provides a
  source tarball, not a native package download
- ✅ [Synology installation guide](docs/synology.md) and
  [packaging/build instructions](packaging/synology/README.md)

## Evidence and review boundaries

The review followed production wiring, mounted routes, UI and existing test
cases, and compared the tagged source with `main`. It did not establish live
provider credentials, NAS permissions, tracker coverage or production safety.
Key implementation entry points:

- [Service startup](cmd/loom/serve.go),
  [media wiring](cmd/loom/wire_media.go),
  [download/import wiring](cmd/loom/wire_downloads.go),
  [integration wiring](cmd/loom/wire_infra.go) and
  [mounted API routes](internal/server/server.go).
- [Release matching](internal/autosearch/engine.go),
  [custom-format semantics](internal/customformats/engine.go),
  [file-metadata limits](internal/customformats/fileinfo.go) and
  [compatibility handlers](internal/compat/).
- [Requests](internal/requests/), [music](internal/music/),
  [library health](internal/libraries/health.go), [analytics](internal/analytics/)
  and [custom scripts](internal/plugins/).
