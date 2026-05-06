# Loom - Project State

**Status:** Pre-alpha | **Last Updated:** 2026-05-07

## Architecture Overview

Loom is a unified replacement for Radarr, Sonarr, and Prowlarr.

### Tech Stack
- **Backend:** Go + Chi router + SQLite (43 migrations, sqlc-typed queries)
- **Frontend:** React 19 + TypeScript + TanStack Router/Query + Vite + shadcn/ui + Tailwind
- **Database:** SQLite via modernc.org/sqlite, MaxOpenConns=1
- **Auth:** Server-authoritative session-based with argon2id password hashing
- **Indexers:** Cardigann YAML engine (542 definitions) + Newznab/Torznab
- **Search proxy:** FlareSolverr with cookie forwarding and shared sessions

### Configuration Strategy
- **Config file (`config/loom.json`):** Portable, shareable configuration
  - Setup state, admin credentials (hashed), persistence options
  - Library definitions with folder paths
  - Overrideable defaults for development/production
- **Database (SQLite `config/data/loom.db`):** Instance-specific operational data
  - Media metadata and discovery state
  - Indexer configurations, download clients
  - Active grabs, download history, release tracking
  - User activity logs
- **Viper defaults (`internal/kernel/config/config.go`):** Timeout and behavior defaults
  - `search_timeout: 120` (seconds), `health_check_timeout: 60`
  - Override with env vars or config file

## Feature Status

### Phase 0: Repo & Foundations ✅
- [x] Go module, AGPL-3.0 license, CI/CD pipeline
- [x] GoReleaser, ADR directory, contributor docs

### Phase 1: Platform Kernel ✅
- [x] HTTP server with chi router, structured logging, CORS, panic recovery
- [x] Storage layer (SQLite + 43 goose migrations)
- [x] Health probes (`/healthz`, `/livez`, `/readyz`)
- [x] Prometheus metrics + OpenTelemetry tracing
- [x] Authentication with session cookies (argon2id)
- [x] Layered config (defaults → file → env → flags) with hot-reload
- [x] Event bus with async fan-out
- [x] Scheduler with cron patterns

### Phase 2: Indexer Subsystem (Prowlarr Replacement) ✅
- [x] Cardigann YAML engine — 542 bundled definitions
- [x] Newznab/Torznab protocol support
- [x] Search aggregation across configured indexers
- [x] FlareSolverr proxy with cookie forwarding
- [x] Search result scoring (0-100) and filtering
- [x] Per-indexer diagnostics, freeleech/tracker intelligence
- [x] Indexer catalogue UI with type/category filters
- [x] Cardigann template expansion in search paths

### Phase 3: Download Pipeline 🟡 (75% complete)
- [x] Download grab flow (magnet URI + NZB)
- [x] Automated search decision engine (search → score → filter → grab best)
- [x] Auto-search at episode, season, and series level
- [x] Download activity tracking (live queue, progress, speed, ETA)
- [x] Active grab tracking with grabbed status on episodes
- [x] Hardlink-only import mode
- [x] Download client CRUD (settings UI with add/edit/delete/test)
- [x] Search timeout chain: 120s through all layers
- [ ] **TODO:** Full import/post-processing pipeline
- [ ] **TODO:** Grab cleanup on import completion
- [ ] **TODO:** Remote-path mappings, blocklist, redownload-on-failure

### Phase 4: Movies Module (Radarr Replacement) ✅
- [x] Movies service & CRUD API
- [x] Root folder management
- [x] Quality profiles & custom formats with scoring
- [x] Frontend movies page (poster grid, detail modal, search)

### Phase 5: Series Module (Sonarr Replacement) ✅
- [x] Series/season/episode data model & API
- [x] TMDB metadata integration
- [x] Frontend series page (season accordion, episode table)
- [x] Episode status (downloaded, missing, unaired, unmonitored, grabbed)
- [x] Auto-search buttons per episode/season (direct API)
- [x] Interactive search dialog for manual release selection
- [x] Series library scanning with season-folder support

### Phase 6: Notifications ✅
- [x] Discord, Webhook, Gotify, Ntfy channels
- [x] Rich templates with Go text/template
- [x] Event bus integration, frontend CRUD page

### Phase 7: Download Safety ✅
- [x] Bad release detection, post-download validation
- [x] Manual review queue, frontend safety settings

### Phases 8-11: Planned ⏳
- [ ] Advanced custom format engine (AND/OR logic)
- [ ] Language profiles & international support
- [ ] Rolling missing search, RSS sync
- [ ] Anime handling, multi-season packs
- [ ] Wire-compatibility with Radarr/Sonarr/Prowlarr APIs
- [ ] Migration tooling, Helm chart, 1.0 release

## How to Start the App

### Prerequisites
- Go 1.23+
- Node.js 18+
- npm

### Full Build (embedded frontend)
```bash
cd /Users/eben/Personal/Code/tmp2

# Build frontend + backend
cd web && npm install && npm run build && cd ..
go build -tags embed -o dist/loom ./cmd/loom

# Run
./dist/loom serve --log-level debug
```

The app listens on `http://localhost:8989` (frontend + API)

### Development Mode (hot reload)
```bash
# Terminal 1: Backend
go build -tags embed -o dist/loom ./cmd/loom && ./dist/loom serve

# Terminal 2: Frontend dev server
cd web && npm run dev
```

Frontend dev server on `http://localhost:5173` proxies API to `:8989`

### First Run Flow
1. Navigate to `http://localhost:8989`
2. Setup page appears (no config exists yet)
3. Enter admin username and password
4. Click initialize — creates config and sets up auth
5. Redirected to login page
6. Login and start configuring indexers, download clients, libraries

## Recent Changes (Latest Session - 2026-05-07)

### Bugs Fixed
- **Download client CRUD:** Settings page now has full add/edit/delete with test connection
- **Search URL templates:** Cardigann Go templates now expand before URL resolution
- **FlareSolverr timeouts:** Search timeout increased 15s → 120s (was hidden in Viper defaults)
- **FlareSolverr cookies:** Cookie forwarding from HTTP requests to FlareSolverr API
- **Filesystem browse null:** Go nil slice → JSON null crash fixed (`make([]T, 0)`)
- **Auto-search buttons:** Episode/season search buttons now call autosearch API directly

### Features Added
- **Grabbed episode tracking:** New `active_grabs` tables track which episodes have been sent to download clients; TV detail page shows amber "Grabbed" badge
- **Automated search engine:** Full search → score → filter → grab pipeline
- **Per-episode/season auto-search:** One-click search + grab for individual episodes or full seasons

## Known Issues & Next Steps

### High Priority
- [ ] Import pipeline not yet wired (downloaded files aren't auto-imported to library)
- [ ] Manual grabs (via interactive search) don't record grab linkage
- [ ] Grab records not cleaned up when downloads complete

### Medium Priority
- [ ] No distinction between "grabbed" and "downloading" (needs live queue check)
- [ ] Grab staleness cleanup (periodic job to prune old grabs)
- [ ] RSS sync not implemented

### Lower Priority
- [ ] Anime handling (AniDB/AniList)
- [ ] Language profiles
- [ ] Wire-compatibility with arr APIs
- [ ] Migration tooling from existing arr stacks

## File Reference

### Key Backend Files
- `cmd/loom/serve.go` — App wiring (creates stores, passes to engine/server)
- `internal/kernel/config/config.go` — Viper config defaults (timeouts live here!)
- `internal/server/server.go` — HTTP server, routing, middleware
- `internal/autosearch/engine.go` — Automated search decision engine
- `internal/grabs/store.go` — Active grab tracking (grabbed status)
- `internal/series/handlers.go` — Series/episode API handlers
- `internal/indexers/cardigann/engine.go` — Cardigann YAML engine
- `internal/indexers/proxies/flaresolverr.go` — FlareSolverr client
- `internal/downloads/` — Download client adapters

### Key Frontend Files
- `web/src/pages/settings.tsx` — Settings page (libraries, download clients, etc.)
- `web/src/pages/series.tsx` — Series listing page
- `web/src/components/series/series-detail-sheet.tsx` — TV detail with episodes
- `web/src/components/series/types.ts` — Episode interface (includes `grabbed`)
- `web/src/lib/autosearch-api.ts` — Auto-search API client

### Config Files
- `config/loom.json` — Runtime configuration
- `config/data/loom.db` — SQLite database
- `sqlc.yaml` — sqlc query generation config
- `web/components.json` — shadcn/ui configuration

