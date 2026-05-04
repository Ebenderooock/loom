# Loom - Project State

**Status:** Pre-alpha | **Last Updated:** 2026-05-05

## Architecture Overview

Loom is a unified replacement for Radarr, Sonarr, and Prowlarr.

### Tech Stack
- **Backend:** Go + Chi router + SQLite (Postgres-ready)
- **Frontend:** React 19 + TypeScript + TanStack Router + Vite
- **Database:** SQLite by default, embedded migrations with sqlc-typed queries
- **Auth:** Server-authoritative session-based with bcrypt password hashing

### Configuration Strategy
- **Config file (`config/loom.json`):** Portable, shareable configuration
  - Setup state, admin credentials (hashed), persistence options
  - Library folder paths (movies, TV shows, etc.)
  - Overrideable defaults for development/production
- **Database (SQLite):** Instance-specific operational data
  - Media metadata and discovery state
  - Release history, downloads, indexer state
  - User activity logs
- **Media storage:** Reference-based (no file copying)
  - Configured library paths are scanned for media
  - DB stores relative paths and file hashes for deduplication

## Feature Status

### Phase 1: Core Infrastructure ✅
- [x] HTTP server with chi router, structured logging, CORS, panic recovery
- [x] Storage layer (SQLite + migrations)
- [x] Health probes (`/healthz`, `/livez`, `/readyz`)
- [x] Prometheus metrics + OpenTelemetry tracing (skeleton)
- [x] Authentication with session cookies + API keys

### Phase 2: Auth & Configuration ✅
- [x] Server-authoritative auth with bcrypt hashing
- [x] `/api/v1/auth/initialize` endpoint (one-time setup)
- [x] `/api/v1/auth/login` endpoint (username/password)
- [x] `/api/v1/auth/status` endpoint
- [x] CORS policy fixed - allows localhost dev ports (5173-5175, 3000)
- [x] Config file loading and defaults
- [x] Session secret auto-generation

### Phase 3: Movies Service 🟡 (80% complete)
- [x] Database schema for movies, quality profiles, custom formats
- [x] All backend handlers implemented:
  - Root folders CRUD (`/api/v1/movies/root-folders`)
  - Movies CRUD (`/api/v1/movies/`)
  - Quality definitions CRUD
  - Quality profiles CRUD
  - Custom formats CRUD + testing
- [x] Frontend movies page (`web/src/pages/movies.tsx`)
  - Library folder add/delete UI
  - Movie list display
  - Error handling and loading states
- [x] Router integration - movies route wired in
- [ ] **TODO:** Library scanning endpoint (discover media files on disk)
- [ ] **TODO:** Movie search and metadata enrichment (TMDB integration)

### Phase 4: Series Service (TV Shows) ⏳
- [ ] Database schema for series, episodes, seasons
- [ ] Backend handlers (CRUD for series, episodes)
- [ ] Frontend series management page
- [ ] Library scanning for TV show folders

### Phase 5: Indexers & Download Clients 🚧
- [x] Backend structure ready (handlers, repository)
- [ ] Newznab indexer support
- [ ] Torrent indexer support
- [ ] Download client integration (NZBGet, Transmission, etc.)
- [ ] Release tracking and matching

### Phase 6: RSS & Monitoring ⏳
- [x] RSS source registration framework
- [x] Scheduler with cron patterns
- [ ] Actual RSS feed parsing and monitoring
- [ ] Release discovery pipeline

## How to Start the App

### Prerequisites
- Go 1.21+
- Node.js 18+
- npm or yarn

### Backend
```bash
cd /Users/eben/Personal/Code/tmp2

# Build
make build

# Run (creates ./run directory with config/data)
mkdir -p run
LOOM_CONFIG_DIR=./run \
LOOM_DATA_DIR=./run \
LOOM_STORAGE_SQLITE_PATH=./run/loom.db \
  ./dist/loom serve
```

The backend listens on `http://localhost:8989`

### Frontend
```bash
cd /Users/eben/Personal/Code/tmp2/web

# Install dependencies (if not done yet)
npm install

# Start dev server
npm run dev
```

The frontend listens on `http://localhost:5173`

### First Run Flow
1. Navigate to `http://localhost:5173`
2. Setup page appears (since no config exists yet)
3. Enter admin username and password
4. Click initialize - creates `./run/loom.json` and sets up auth
5. Redirected to login page
6. Login with credentials just set
7. Navigate to Movies page to manage libraries

## Current Issues & Blockers

### Recently Fixed ✅
- CORS policy was blocking frontend-to-backend communication
  - Solution: Updated `applyDefaults()` in `config.go` to include localhost dev ports
- Frontend auth endpoint was wrong (`/setup` → `/initialize`)
  - Solution: Corrected `setup.tsx` to use `/api/v1/auth/initialize`
- Movies page not routed in frontend
  - Solution: Added movies route to router and navigation links

### Known Limitations
- [ ] Library scanning not yet implemented - users can add root folders but no media discovery
- [ ] No metadata enrichment (TMDB) for movies
- [ ] No file hash calculation for deduplication yet
- [ ] Cannot actually add movies to the database
- [ ] Series service not started

## File Reference

### Key Backend Files
- `cmd/loom/main.go` - Application entry point
- `internal/kernel/config/config.go` - Configuration loading (CORS defaults at line 363-371)
- `internal/auth/handlers.go` - Authentication endpoints
- `internal/movies/handlers.go` - Movies CRUD handlers (line 39-78: Router function)
- `internal/movies/repository.go` - Movies data access

### Key Frontend Files
- `web/src/pages/setup.tsx` - Initial setup page (endpoint fixed at line 31)
- `web/src/pages/movies.tsx` - Movies management page
- `web/src/routes/router.tsx` - Application routing
- `web/src/components/layout/app-layout.tsx` - Navigation and layout
- `web/src/hooks/use-auth.tsx` - Authentication context

### Config Files
- `web/components.json` - shadcn/ui configuration
- `web/tsconfig.json` - TypeScript configuration
- `go.mod` - Go dependencies

## Next Steps

1. **Implement library scanning** (`/api/v1/movies/{rootFolderId}/scan`)
   - Walk directory tree for video files
   - Parse metadata (filename, file hash, size)
   - Insert into database
   
2. **Add movie search/discovery** to UI
   - Search against discovered files or TMDB
   - Ability to mark media as "monitored"

3. **Implement Series service** (follow movies pattern)
   - Database schema for series/episodes/seasons
   - CRUD handlers
   - Frontend UI

4. **Wire up indexers and download clients**
   - Newznab indexer parsing
   - Download client communication

5. **Update documentation** in LOOM_STATE.md as features complete

## Testing Checklist

- [x] Backend builds without errors
- [x] Frontend builds without errors
- [x] CORS headers present on /api/v1/auth/status
- [x] Initialize endpoint creates session
- [x] Login endpoint works
- [x] Movies page loads and displays
- [ ] Add root folder works end-to-end
- [ ] Movies list displays (once scanning implemented)
- [ ] Test with actual media folders

