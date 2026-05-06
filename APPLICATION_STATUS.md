# Loom — Application Status

**Last Updated**: 2026-05-07  
**Build**: Go backend compiles clean, TypeScript frontend compiles clean  
**Overall**: Core media automation pipeline functional end-to-end; search → grab → track working; advanced features (import pipeline, anime, languages) planned

---

## Architecture

Loom is a unified media automation app replacing Radarr, Sonarr, and Prowlarr.

| Layer | Stack |
|-------|-------|
| Backend | Go · chi router · sqlc · SQLite (43 migrations) |
| Frontend | React 19 · TypeScript · Vite · TanStack Router/Query · shadcn/ui · Tailwind |
| Indexers | Cardigann YAML engine (542 bundled definitions) · Newznab/Torznab |
| Search | FlareSolverr proxy support · cookie forwarding · 120s timeout chain |
| Config | `config/loom.json` (portable) + `config/data/loom.db` (instance) |

---

## What Works Today (✅ Done — 34 items)

### Platform Kernel
- [x] Layered config (defaults → file → env → flags) with hot-reload via Viper
- [x] Server-authoritative auth (argon2id, session cookies, setup flow)
- [x] SQLite persistence with goose migrations (43 migrations)
- [x] chi HTTP server with structured logging, health endpoints, CORS
- [x] Event bus (in-process channels) with async fan-out
- [x] React app with full routing, auth guards, sidebar nav, dashboard
- [x] Scheduler with cron patterns and jittered intervals

### Indexer Subsystem (Prowlarr Replacement)
- [x] Cardigann YAML engine — 542 bundled indexer definitions
- [x] Indexer CRUD with proxy & FlareSolverr support
- [x] FlareSolverr cookie forwarding (essential for sites like EZTV)
- [x] Indexer catalogue UI — searchable table with type/category filters
- [x] Search aggregation across configured indexers
- [x] Search result scoring (quality + seeders + age + size, 0–100)
- [x] Search result filtering (indexer, quality, size, seeders, freeleech, title)
- [x] Search diagnostics — per-indexer timing, status, result counts
- [x] Freeleech / tracker intelligence (FL, Internal, Scene badges, +15 score bonus)
- [x] Cardigann template expansion (Go text/template in search paths)

### Media Pipeline
- [x] Download grab flow (magnet URI + NZB)
- [x] Automated search decision engine (search → score → filter → grab best)
- [x] Auto-search at episode, season, and series level
- [x] Download activity tracking (live queue with progress bars, speed, ETA)
- [x] Grabbed episode/movie status tracking (active_grabs tables)
- [x] Series library scanning (season-folder + flat directory layouts)
- [x] Hardlink-only import mode (move / hardlink / hardlink_only)
- [x] Download client CRUD in settings UI (add/edit/delete with test)

### Movies (Radarr Replacement)
- [x] Movies service & CRUD API
- [x] Root folder management
- [x] Quality profiles & custom formats with scoring
- [x] Frontend movies page with poster grid, detail modal, search

### TV Shows (Sonarr Replacement)
- [x] Series/season/episode data model & API
- [x] TMDB metadata integration
- [x] Frontend series page with season accordion, episode table
- [x] Episode status display (downloaded, missing, unaired, unmonitored, grabbed)
- [x] Auto-search buttons per episode and season (direct API, no dialog)
- [x] Interactive search dialog for manual release selection

### Notifications
- [x] Discord, Webhook, Gotify, Ntfy notification channels
- [x] Rich templates with variable substitution (Go text/template)
- [x] Event bus integration — async fan-out to all subscribed channels
- [x] Frontend notifications page (CRUD, test, template editor)

### Download Safety
- [x] Bad release detection (dangerous extensions, suspicious patterns, size anomalies)
- [x] Post-download validation (video files present, no executables, no password archives)
- [x] Manual review queue with approve/reject (REST API + Activity tab)
- [x] Frontend safety settings panel + review count badge

### Settings & Configuration
- [x] Settings page with tabbed panels (General, Libraries, Download Clients, Media Management, etc.)
- [x] Library CRUD (add/edit/delete with folder browser dialog)
- [x] Download client CRUD (add/edit/delete with test connection)
- [x] Filesystem browse API (for folder picker dialogs)
- [x] Naming settings with preview

---

## In Progress (🚧 — 3 items)

- [ ] Import pipeline — post-download file processing (move/hardlink to library)
- [ ] Grab cleanup — remove active_grabs when downloads complete and import
- [ ] Manual grab tracking — interactive search grabs don't record media linkage yet

---

## Remaining Work (⏳ — 30 items)

### Import Pipeline (High Priority)
- [ ] Full import/post-processing pipeline (detect complete → rename → move/hardlink)
- [ ] Wire grab cleanup to import (call `grabStore.Remove*()` on successful import)
- [ ] Deterministic import behavior (fail-safe modes)
- [ ] Smarter re-import (avoid unnecessary work)

### Download Enhancements
- [ ] Live download queue cross-reference (distinguish "grabbed" from "downloading")
- [ ] Grab staleness cleanup (periodic job to prune old grabs)
- [ ] Remote-path mappings
- [ ] Blocklist management
- [ ] Redownload-on-failure logic
- [ ] Smart stalled/failed download handling

### Custom Format Engine
- [ ] AND/OR logic, nested conditions, reusable building blocks
- [ ] Metadata-aware post-download matching

### Language & International Support
- [ ] Language profiles (priority-based, per-library)
- [ ] Subtitle/audio track awareness
- [ ] International release handling (MULTi, dual-audio)

### Long-Tail Search
- [ ] Rolling missing search (scheduled, quota-aware)
- [ ] Quota-aware API call tracking
- [ ] Old/rare content search strategy
- [ ] RSS sync (scheduled feed polling)

### Complex TV Handling
- [ ] Anime handling (AniDB/AniList mapping, absolute numbering)
- [ ] Multi-season pack support
- [ ] Specials & mini-series handling
- [ ] Alternate episode ordering (DVD, absolute, etc.)

### Indexer Intelligence
- [ ] Indexer health dashboard (uptime, response times, error rates)
- [ ] Per-media indexer rules
- [ ] Jackett definition import compatibility

### Infrastructure & Polish
- [ ] Proactive health alerts and notifications
- [ ] Wire-compatibility with Radarr/Sonarr/Prowlarr APIs
- [ ] Migration tooling (`loom migrate import --from {radarr,sonarr,prowlarr}`)
- [ ] Distroless multi-arch Docker images, Helm chart
- [ ] Public documentation site

### Community Feature Requests
- [ ] Alternate/original title handling
- [ ] Better import list behavior
- [ ] Archive/lifecycle management
- [ ] Subtitle/media metadata rules

---

## Key Technical Details

| Topic | Detail |
|-------|--------|
| Server binary | `./dist/loom serve` on port 8989 |
| Frontend dev | `cd web && npm run dev` on port 5173 |
| Build | `cd web && npm install && npm run build` then `go build -tags embed -o dist/loom ./cmd/loom` |
| Auth | Session cookies (`loom_session`), set during setup flow |
| Database | SQLite via modernc.org/sqlite, MaxOpenConns=1 |
| IDs | Slug-based string IDs throughout |
| API convention | All list endpoints wrap in `{ "data": [...] }` |
| Migrations | `internal/storage/migrations/sqlite/0001–0043` |
| Cardigann defs | `internal/indexers/cardigann/definitions/` (542 YAML files) |
| Search timeout | 120s (Viper default), full chain: frontend → handler → registry → HTTP client → FlareSolverr |
| FlareSolverr | Cookie forwarding enabled; shared session mode; 90s internal timeout |

---

## Development

```bash
# Full build (frontend + backend embedded)
cd web && npm install && npm run build && cd ..
go build -tags embed -o dist/loom ./cmd/loom

# Run
./dist/loom serve --log-level debug

# Frontend dev server (hot reload, proxies to :8989)
cd web && npm run dev

# Go tests
go test ./...

# Type-check frontend
cd web && npx tsc --noEmit
```

---

## File Map

```
internal/
├── auth/              # Auth handlers & middleware
├── autosearch/        # Automated search decision engine
├── grabs/             # Active grab tracking (grabbed status)
├── kernel/config/     # Layered config loader (Viper defaults here)
├── movies/            # Movies service (Radarr replacement)
├── series/            # Series service (Sonarr replacement)
├── indexers/          # Indexer subsystem (Prowlarr replacement)
│   ├── cardigann/     # Cardigann YAML engine + 542 definitions
│   ├── proxies/       # FlareSolverr integration + cookie forwarding
│   ├── scoring.go     # Search result scoring (0-100)
│   ├── handlers.go    # REST API handlers
│   └── registry.go    # Multi-indexer search aggregation
├── downloads/         # Download client adapters (qBittorrent, etc.)
├── imports/           # Import pipeline (in progress)
├── notifications/     # Notification channels & templates
├── safety/            # Release validation & review queue
├── scanner/           # Library filesystem scanner
├── libraries/         # Library management
├── qualityprofiles/   # Quality profile service
├── customformats/     # Custom format engine
├── metadata/          # Metadata orchestration (TMDB, TVDB)
├── scheduler/         # Cron scheduler
├── storage/           # SQLite + migrations (0001–0043)
└── server/            # HTTP server, routing, middleware

web/src/
├── pages/             # Route pages (dashboard, movies, series, settings, etc.)
├── components/        # UI components (search dialog, series detail, etc.)
├── lib/               # API clients & utilities (autosearch-api, etc.)
└── hooks/             # React hooks (auth, etc.)
```
