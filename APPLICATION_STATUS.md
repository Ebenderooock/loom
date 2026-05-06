# Loom — Application Status

**Last Updated**: 2026-05-07  
**Build**: Go backend compiles clean, TypeScript frontend compiles clean  
**Overall**: Core media automation pipeline functional end-to-end; search → grab → download → import working; advanced features (compat APIs, migration) next

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

## What Works Today (✅ Done — 66 items)

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

### Media Pipeline (Phase 3 — COMPLETE)
- [x] Download grab flow (magnet URI + NZB)
- [x] Automated search decision engine (search → score → filter → grab best)
- [x] Auto-search at episode, season, and series level
- [x] Download monitor — polls all clients every 30s, emits completion events
- [x] Import pipeline — download completion → file rename → move/hardlink to library
- [x] Grab-based matching — exact media→download linkage before fuzzy fallback
- [x] Manual grab tracking — interactive search grabs record media context
- [x] Auto-search grab tracking — automated grabs record episode/movie linkage
- [x] Grab staleness cleanup — prunes grabs older than 48h every ~10 minutes
- [x] Stall detection — identifies downloads with no progress for 30 minutes
- [x] Blocklist management — stall handler can blocklist failed releases
- [x] Remote path mappings — API for Docker/remote client path translation
- [x] Activity page with Queue, History, Blocklist, and Reviews tabs
- [x] Live download queue with progress bars, speed, ETA (auto-refresh 5s)
- [x] Grabbed episode/movie status tracking (active_grabs tables + amber badges)
- [x] Series library scanning (season-folder + flat directory layouts)
- [x] Hardlink-only import mode (move / copy / hardlink / hardlink_only)
- [x] Download client CRUD in settings UI (add/edit/delete with test)

### *arr API Compatibility (Overseerr/Ombi Drop-in)
- [x] Radarr v3 API shim — movie CRUD, lookup, root folders, quality profiles, commands, system status
- [x] Sonarr v3 API shim — series/episode CRUD, lookup, root folders, quality/language profiles, commands
- [x] Prowlarr v1 API shim — indexer list/get, search with category/ID filters, stats, health
- [x] Compat routes mounted at /compat/radarr/*, /compat/sonarr/*, /compat/prowlarr/*
- [x] Integer ID ↔ string ID mappers for *arr-compatible numeric IDs

### Connect & Trakt Integration
- [x] Connect service — Plex, Emby, Jellyfin, Trakt provider CRUD
- [x] Trakt OAuth2 flow — authorize URL, callback (token exchange), token refresh
- [x] Trakt sync endpoints — watched status, collections, watchlists
- [x] Frontend Connect panel with provider selector and OAuth flow UI
- [x] Frontend Trakt sync buttons (watched, collection, watchlist) with refresh token

### Migration Tooling (Stream C)
- [x] `loom migrate --from radarr --db /path` — movies, quality profiles, root folders
- [x] `loom migrate --from sonarr --db /path` — series, episodes, seasons, quality profiles, root folders
- [x] `loom migrate --from prowlarr --db /path` — indexer configs with settings
- [x] Duplicate detection (skip existing TMDB/TVDB IDs)
- [x] Import result summary with counts and error reporting

### Operational Hardening (Stream D)
- [x] Indexer search health tracker (success rate, avg response time, error tracking)
- [x] Indexer health dashboard page (frontend with progress bars, status badges)
- [x] Health monitor — disk space, indexer down, download client unreachable → notifications
- [x] `loom backup --action create` — tar.gz of SQLite DB + config with timestamps
- [x] `loom backup --action restore` — extract backup with .bak safety rename

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

## In Progress (🚧 — 0 items)

All core streams are complete. Remaining items are infrastructure polish.

---

## Remaining Work (⏳ — 5 items)

### Infrastructure & Polish
- [ ] Distroless multi-arch Docker images, Helm chart
- [ ] Public documentation site
- [ ] Rolling missing search (scheduled, quota-aware)
- [ ] RSS sync (scheduled feed polling)
- [ ] Remote path mappings settings panel (backend API exists, no frontend UI)

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
