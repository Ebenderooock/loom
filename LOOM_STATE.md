# Loom Architecture & Implementation Status

**Last Updated:** 2026-05-05  
**Project Status:** Pre-alpha, Phase 1 in progress  
**Current Focus:** Auth redesign + Movies module integration

---

## Project Overview

**Loom** is a unified replacement for the *arr stack (Radarr + Sonarr + Prowlarr) in a single Go binary.

### Vision
- Single binary, multiple applications (movies, TV, indexers, downloaders)
- SQLite by default, Postgres-ready
- Modern OpenTelemetry observability from day one
- Wire-compatible with Overseerr, Jellyseerr, Bazarr, Notifiarr, Plex, Jellyfin, Emby
- Mobile-first React UI

### Core Design Principles
1. **Modular Monolith** - One binary but independently deployed services
2. **Config ≠ Data** - Portable JSON config (credentials, library paths, settings) vs instance-specific SQLite database (media, sessions, downloads)
3. **Server-Authoritative** - Auth state lives on server, not in browser localStorage
4. **Observable** - OpenTelemetry, Prometheus, JSON structured logs, pprof from day 1

---

## Architecture

### Backend Structure

```
cmd/loom/
├── main.go           # Entry point
├── serve.go          # HTTP server orchestration, service wiring
└── auth.go           # Auth service builder

internal/
├── appconfig/        # Config file loading/saving (config/loom.json)
├── auth/             # Authentication & authorization
├── movies/           # Movies module (Radarr-equivalent) ← 80% done, NOT wired
├── series/           # Series module (Sonarr-equivalent) ← skeleton only
├── indexers/         # Indexer aggregation (Prowlarr-equivalent) ✅ working
├── downloads/        # Download client adapters ✅ working
├── rss/              # RSS sync for indexers ✅ working
├── storage/          # SQLite/Postgres layer
├── server/           # HTTP router, middleware, server setup
└── ...other subsystems
```

### Frontend Structure

```
web/
├── src/
│   ├── routes/       # TanStack Router pages
│   ├── components/   # React components (shadcn/ui)
│   ├── hooks/        # React hooks (useAuth, queries)
│   ├── lib/          # API clients, utilities
│   └── styles/       # Tailwind config + globals
└── public/
```

### Config & Data Separation

**config/loom.json** (portable, shareable):
```json
{
  "setup_complete": true,
  "admin": {
    "username": "admin",
    "password_hash": "$argon2id$..." 
  },
  "persistence": { /* media library paths */ },
  "libraries": [
    { "id": "movies", "name": "Movies", "type": "movie", "paths": ["/mnt/movies"] }
  ]
}
```

**SQLite database** (instance-specific):
- Users, sessions, tokens
- Media metadata (movies, series, episodes)
- Download history, completion status
- RSS feed state, scheduler state

---

## Delivery Roadmap (11 Phases)

| Phase | Name | Status | Notes |
|-------|------|--------|-------|
| 0 | Repo & foundations | ✅ Done | CI, release pipeline, ADRs |
| 1 | Platform kernel | 🚧 In Progress | Config, logging, storage, HTTP server, **Auth (redesign complete)** |
| 2 | Indexer subsystem | ⏳ Planned | Prowlarr-equivalent (Cardigann, Newznab/Torznab, aggregation) |
| 3 | Download adapters | ⏳ Planned | qBittorrent, Transmission, Deluge, rTorrent, SABnzbd, NZBGet |
| 4 | Metadata providers | ⏳ Planned | TMDB, TheTVDB, IMDB, Trakt, AniDB, AniList |
| 5 | Movies (Radarr) | ⏳ Planned | Library mgmt, quality profiles, custom formats, import, calendar |
| 6 | Series (Sonarr) | ⏳ Planned | Series/seasons/episodes, anime mapping, calendar |
| 7 | Wire-compatibility | ⏳ Planned | `/api/v3/*` Radarr/Sonarr, `/api/v1/*` Prowlarr, Bazarr compatibility |
| 8 | Migration tooling | ⏳ Planned | Import from existing Radarr/Sonarr/Prowlarr |
| 9 | Frontend parity | ⏳ Planned | Unified library UI, discover, activity, calendar, themes, i18n |
| 10 | Notifications & plugins | ⏳ Planned | Discord/Slack/Telegram, custom scripts, gRPC plugin SDK |
| 11 | Hardening & 1.0 | ⏳ Planned | Distroless images, Helm, Grafana dashboards, backups, SBOM, 1.0 release |

---

## Feature Sets & Implementation Status

### 1. Authentication & Authorization

**Status:** 🚧 Phase redesign complete

#### What's Working
- ✅ `/api/v1/auth/login` - Username/password authentication
- ✅ `/api/v1/auth/status` - Check auth state and setup status
- ✅ Session cookies with 30-day TTL, SameSite=Lax
- ✅ CORS configuration with `Access-Control-Allow-Credentials`
- ✅ Argon2id password hashing
- ✅ `/api/v1/auth/initialize` - One-time setup endpoint
  - Creates admin user in database
  - Saves hashed credentials to config/loom.json
  - Only callable when `setup_complete: false`
  - Returns 403 if already initialized

#### What's Missing
- ❌ Frontend setup flow (still calls old `/api/v1/auth/setup`)
- ❌ Library path configuration during setup
- ❌ API key generation
- ❌ OIDC (Authelia, Authentik, Keycloak, Entra)
- ❌ Reverse-proxy header trust
- ❌ Per-user auth logs

#### Architecture
- **Backend:** Credentials in config/loom.json (hashed), auth state server-authoritative
- **Frontend:** Uses React hook `useAuth()` to check `/api/v1/auth/status` on mount
- **Sessions:** HTTP cookie with Go's `http.SetCookie` (browser auto-persists if credentials included)

---

### 2. Movies Module (Radarr-equivalent)

**Status:** 🚧 80% implemented, NOT wired to server

#### What's Built (but not accessible over HTTP)
- ✅ Full CRUD API for movies
  - Add, update, delete, get, list with filtering/sorting
  - Search by title, IMDb ID, TMDb ID
  - Monitoring status (monitored/unmonitored/deleted)

- ✅ Quality management
  - Quality definitions (guide for what different file qualities mean)
  - Quality profiles (user-defined preferences, e.g., "prefer 1080p, accept 720p")
  - Custom formats (regex-based file attributes, e.g., "HDR", "DoVi", "10-bit")

- ✅ Library organization
  - Root folders (where movies are stored on disk)
  - Movie files (metadata about individual files, with monitoring status)

- ✅ Repository pattern
  - Data access layer with in-memory caching (5-minute TTL)
  - Prepared database queries (sqlc-generated)

#### Files Involved
- `internal/movies/service.go` (~500 lines)
- `internal/movies/handlers.go` (~300 lines, handlers not mounted)
- `internal/movies/repository.go` (~200 lines)
- `internal/movies/models.go` (~100 lines)

#### What's Missing
- ❌ **Server wiring** - No `moviesSvc` field on `Server` struct
  - No build function to instantiate movies service
  - No `.Mount()` call in HTTP router
  - Handlers exist but unreachable

- ❌ **Config integration** - Movies doesn't read library paths from config
  - RootFolders hardcoded to database, not config-driven
  - Need to decide: Merge RootFolders into library config, or keep separate?

- ❌ **Library scanning** - No disk scanning to discover media files
  - No background job to walk `/mnt/movies` and populate `MovieFiles`
  - No metadata enrichment (fetch from TMDB, calculate file hash)

- ❌ **Frontend** - No UI for movie management
  - No library browser
  - No add/edit/delete library interface

#### Design Question (BLOCKED)
Should `RootFolders` (movies concept) be replaced by `libraries` (config system)?
- Option A: Use libraries from config for all media types (movies, TV, etc.)
- Option B: Keep RootFolders in database, map to libraries at query time

---

### 3. Series Module (Sonarr-equivalent)

**Status:** ⏳ Skeleton only

#### What's Built
- ✅ Empty service skeleton
- ✅ Repository interface defined
- ✅ Models for Series, Season, Episode

#### What's Missing
- ❌ Full CRUD (add, update, delete, get, list)
- ❌ Episode tracking (aired/unaired/monitored)
- ❌ Season management (season pack detection)
- ❌ Anime mapping (AniList/AniDB integration)
- ❌ Server wiring

---

### 4. Indexers & Proxies (Prowlarr-equivalent)

**Status:** ✅ Working

#### What's Built
- ✅ Cardigann YAML indexer loader
- ✅ Newznab/Torznab aggregation (search across multiple indexers)
- ✅ Rate limiting and retry/backoff
- ✅ FlareSolverr proxy for Cloudflare bypass
- ✅ Manual search UI (React)
- ✅ HTTP API endpoints

#### What's Missing
- ❌ Indexer health monitoring (per-indexer success rates)
- ❌ Advanced proxy rules

---

### 5. Download Clients

**Status:** ✅ Working

#### What's Built
- ✅ qBittorrent adapter (add torrents, check status, remove)
- ✅ Transmission adapter
- ✅ Deluge adapter
- ✅ rTorrent adapter
- ✅ SABnzbd (NZB) adapter
- ✅ NZBGet (NZB) adapter
- ✅ Remote path mappings (translate `/downloads` to `/mnt/downloads`)
- ✅ HTTP API endpoints

#### What's Missing
- ❌ Blocklist support
- ❌ Redownload-on-failure logic

---

### 6. HTTP Server & Infrastructure

**Status:** ✅ Complete (Phase 1)

#### What's Built
- ✅ chi router with middleware (request-ID, structured logs, panic recovery, gzip, ETag)
- ✅ CORS configuration (with support for credentials)
- ✅ Health endpoints: `/healthz`, `/readyz`, `/livez`
- ✅ Prometheus metrics at `/metrics` (Go runtime + process collectors)
- ✅ OpenTelemetry SDK + OTLP exporter
- ✅ pprof debug endpoints (gated by config) at `/debug/pprof/*`
- ✅ Request IDs propagated through logs and traces

---

### 7. Storage & Database

**Status:** ✅ Complete (Phase 1)

#### What's Built
- ✅ SQLite by default, Postgres-ready
- ✅ Embedded goose migrations (version control for schema)
- ✅ sqlc-generated type-safe queries
- ✅ User table (for auth)
- ✅ Movie, Series, Episode tables (with relationships)
- ✅ RootFolder, MovieFile, EpisodeFile tables

#### Migrations
Located in `internal/storage/migrations/`

---

### 8. Frontend

**Status:** ✅ Scaffold complete, feature-specific pages in flight

#### What's Built
- ✅ React 18 + TypeScript + Vite
- ✅ TanStack Router for routing
- ✅ TanStack Query for data fetching & caching
- ✅ shadcn/ui component library + Tailwind CSS
- ✅ Storybook for component development
- ✅ Playwright for E2E testing
- ✅ Auth hook: `useAuth()` checks `/api/v1/auth/status` on mount

#### Pages Implemented
- ✅ Setup page (calls old `/api/v1/auth/setup` — needs updating)
- ✅ Login page (uses `/api/v1/auth/login`)
- ✅ Indexers management page (list, add, edit, delete)
- ✅ Download clients management page

#### Pages Missing
- ❌ Library management (add/edit/delete libraries)
- ❌ Movie browser / library
- ❌ Series browser / library
- ❌ Discover page (search for new media)
- ❌ Activity / download queue page
- ❌ Calendar page
- ❌ Settings page

---

## Quick Reference: How to Start the App

### Backend
```bash
cd /Users/eben/Personal/Code/tmp2
LOOM_CONFIG_DIR=./config \
LOOM_DATA_DIR=./run \
LOOM_STORAGE_SQLITE_PATH=./run/loom.db \
  ./loom serve
```
- Listens on http://localhost:8989
- Config at `./config/loom.json` (auto-created from defaults)

### Frontend
```bash
cd /Users/eben/Personal/Code/tmp2/web
pnpm run dev
```
- Listens on http://localhost:5173

### API Endpoints (Current)
- `POST /api/v1/auth/initialize` - One-time setup (create admin, write config)
- `POST /api/v1/auth/login` - Login with username/password
- `GET /api/v1/auth/status` - Check auth state
- `GET /api/v1/indexers/*` - Indexer management
- `GET /api/v1/downloads/*` - Download client management
- `GET /api/v1/system/status` - Health check
- `GET /healthz`, `/livez`, `/readyz` - Kubernetes probes
- `GET /metrics` - Prometheus metrics
- `GET /debug/pprof/*` - Go profiling

### Database
- SQLite at `./run/loom.db`
- View schema: `sqlite3 ./run/loom.db ".schema"`
- Query movies: `sqlite3 ./run/loom.db "SELECT * FROM movies;"`

---

## Known Issues & Quirks

### Movies Module
- CustomFormat operations create a new service instance each time (inefficient but works)
- Should be refactored for connection pooling, but low priority

### Frontend
- Setup page still hardcoded to use old `/api/v1/auth/setup` endpoint
- Doesn't support library path configuration
- No error handling for failed auth/setup

### Database
- Migrations exist but some tables may be unused
- Consider cleanup pass after feature parity reached

---

## Implementation Phases & Current Focus

### Phase 1 (In Progress)
**Platform Kernel** - Foundation for all future features

#### Completed ✅
- Config layering, logging, telemetry
- Storage (SQLite/Postgres + migrations)
- HTTP server, health probes, CORS
- Auth redesign (server-authoritative, config persistence)

#### In Flight 🚧
- Frontend setup flow update (to use `/api/v1/auth/initialize`)

#### Blocked 🚫
- Movies wiring (depends on library-config decision)

### Phase 5 (Unblocked - Ready to Wire)
**Movies (Radarr-equivalent)** - Movie library management

**Architecture Decision Made:** Use Option B
- RootFolders stored in database (not config)
- Config.json stays minimal (just bootstrap info)
- Libraries managed via `/api/v1/movies/root-folders` CRUD API
- No refactoring needed—movies module works as-is

**Next steps:**
1. Add `moviesSvc` to Server struct
2. Wire `/api/v1/movies/*` routes
3. Add library management UI
4. Implement library scanning

#### Blocked on...
Nothing—ready to implement!

---

## Decision Log

### Auth Architecture (Phase 1 - COMPLETE)
- **Decision:** Credentials stored as hashed values in config/loom.json, not localStorage
- **Rationale:** Enables portable config sharing, server-authoritative auth
- **Implementation:** `/api/v1/auth/initialize` persists credentials, session cookies for browser

### Config vs Data Split (PHASE 1 - COMPLETE)
- **Decision:** config/loom.json for portable setup state, database for instance-specific data
- **Rationale:** Configs can be shared between installations after removing password_hash field
- **Files:** appconfig/config.go handles load/save; appconfig.Config struct typed

### RootFolders vs Libraries (PHASE 5 - DECIDED ✅)
- **Decision:** Option B - Keep RootFolders in database, map to config at query time
- **Rationale:** Config file is bare minimum to boot the app (host, port, credentials). All library/media state lives in database. Config is derived from runtime state, not the source of truth.
- **Impact:** Movies module can wire as-is, no refactoring needed. Libraries managed via UI after setup.
- **Implementation:** Movies CRUD API unchanged, frontend UI creates RootFolders via HTTP API, config.json stays minimal

---

## How to Use This Document

This document is the **persistent state** for the Loom project. Use it to:

1. **Onboard yourself** - Read Overview + Architecture sections
2. **Understand phase progress** - Check Delivery Roadmap and Feature Sets
3. **Find what's built vs missing** - Review each feature section
4. **Know the current focus** - Read Implementation Phases section
5. **Reference APIs** - Check Quick Reference section
6. **Make decisions** - Review Decision Log and Architecture Decisions
7. **Start developing** - Check how to start app, key files, known issues

Update this document when:
- Completing a major phase
- Making architectural decisions
- Finishing a feature set
- Encountering a design blocker
- Moving to next phase
