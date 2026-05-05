# Loom — Application Status

**Last Updated**: 2025-05-05  
**Build**: Go backend compiles clean, TypeScript frontend compiles clean  
**Overall**: Core media automation pipeline functional; advanced features in progress

---

## Architecture

Loom is a unified media automation app replacing Radarr, Sonarr, and Prowlarr.

| Layer | Stack |
|-------|-------|
| Backend | Go · chi router · sqlc · SQLite |
| Frontend | React 18 · TypeScript · Vite · TanStack Router/Query · shadcn/ui · Tailwind |
| Indexers | Cardigann YAML engine (542 bundled definitions) · Newznab/Torznab |
| Config | `config/loom.json` (portable) + `config/data/loom.db` (instance) |

---

## What Works Today (✅ Done — 20 items)

### Platform Kernel
- [x] Layered config (defaults → file → env → flags) with hot-reload
- [x] Server-authoritative auth (argon2id, session cookies, setup flow)
- [x] SQLite persistence with goose migrations (21 migrations)
- [x] chi HTTP server with structured logging, health endpoints
- [x] Event bus (in-process channels)
- [x] React app skeleton with full routing, auth guards, sidebar nav

### Indexer Subsystem (Prowlarr Replacement)
- [x] Cardigann YAML engine — 542 bundled indexer definitions
- [x] Indexer CRUD with proxy & FlareSolverr support
- [x] Indexer catalogue UI — searchable table with type/category filters
- [x] Search aggregation across configured indexers
- [x] Search result scoring (quality + seeders + age + size, 0–100)
- [x] Search result filtering (indexer, quality, size, seeders, freeleech, title)
- [x] Search diagnostics — per-indexer timing, status, result counts
- [x] Freeleech / tracker intelligence (FL, Internal, Scene badges, +15 score bonus)

### Media Pipeline
- [x] Download grab flow (magnet URI + NZB)
- [x] Auto-search on add (fires background search when adding movies/series)
- [x] Download activity tracking (live queue with progress bars, speed, ETA)
- [x] Series library scanning (season-folder + flat directory layouts)
- [x] Hardlink-only import mode (move / hardlink / hardlink_only)

### Movies (Radarr Replacement)
- [x] Movies service & CRUD API
- [x] Root folder management
- [x] Quality profiles & custom formats with scoring
- [x] Frontend movies page with poster grid, detail modal, search

### TV Shows (Sonarr Replacement)
- [x] Series/season/episode data model
- [x] TMDB metadata integration
- [x] Frontend series page with season accordion, episode table
- [x] Episode status display with progress bars

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

---

## In Progress (🚧 — 1 item)

- [ ] Movies service full wiring (finishing edge cases)

---

## Remaining Work (⏳ — 36 items)

### Phase 4 — Custom Format Engine (power-user automation)
- [ ] Custom format engine with AND/OR logic, nested conditions
- [ ] Custom format scoring integration with download decisions
- [ ] Metadata-aware post-download matching

### Phase 5 — Language & International Support
- [ ] Language profiles (priority-based, per-library)
- [ ] Subtitle/audio track awareness
- [ ] International release handling (MULTi, dual-audio)

### Phase 6 — Import Pipeline
- [ ] Full import/post-processing pipeline
- [ ] Deterministic import behavior (fail-safe modes)
- [ ] Smarter re-import (avoid unnecessary work)

### Phase 7 — Long-Tail Search
- [ ] Rolling missing search (scheduled, quota-aware)
- [ ] Quota-aware API call tracking
- [ ] Old/rare content search strategy

### Phase 8 — Complex TV Handling
- [ ] Anime handling (AniDB/AniList mapping, absolute numbering)
- [ ] Multi-season pack support
- [ ] Specials & mini-series handling
- [ ] Alternate episode ordering (DVD, absolute, etc.)

### Phase 9 — Post-Download Validation
- [ ] Full release validation pipeline (extends current safety checks)

### Phase 10 — Automation & Hooks
_(Notification channels/templates/bus are done; remaining items below)_
- [ ] Proactive notifications & health alerts

### Phase 11 — Indexer Intelligence
- [ ] Indexer health dashboard (uptime, response times, error rates)
- [ ] Per-media indexer rules (which indexers to use for which media)
- [ ] Jackett definition import compatibility

### Infrastructure & Polish
- [ ] Setup flow redesign (config initialization wizard)
- [ ] Frontend setup integration
- [ ] Library schema improvements
- [ ] Library management API endpoints
- [ ] Library scanning (filesystem discovery)
- [ ] Library management UI

### Feature Requests (from Sonarr/Prowlarr community)
- [ ] Alternate/original title handling
- [ ] Powerful API/automation endpoints
- [ ] Better download client integration
- [ ] Powerful filtering/organization
- [ ] Better import list behavior
- [ ] Archive/lifecycle management
- [ ] Smart stalled/failed download handling
- [ ] Smarter import/re-import workflows
- [ ] Subtitle/media metadata rules

---

## Key Technical Details

| Topic | Detail |
|-------|--------|
| Server binary | `./loom serve` on port 8989 |
| Frontend dev | `cd web && pnpm dev` on port 5173 |
| Auth | Session cookies (`loom_session`), default admin/admin |
| Database | SQLite via modernc.org/sqlite, MaxOpenConns=1 |
| IDs | Slug-based string IDs throughout |
| API convention | All list endpoints wrap in `{ "data": [...] }` |
| Migrations | `db/migrations/0001–0021`, next is 0022+ |
| Cardigann defs | `internal/indexers/cardigann/definitions/` (542 YAML files) |

---

## Development

```bash
# Build & run backend
go build -o loom ./cmd/loom/ && ./loom serve

# Run frontend dev server
cd web && pnpm dev

# Run Go tests
go test ./...

# Type-check frontend
cd web && npx tsc --noEmit
```

---

## File Map

```
internal/
├── auth/              # Auth handlers & middleware
├── kernel/config/     # Layered config loader
├── movies/            # Movies service (Radarr replacement)
├── series/            # Series service (Sonarr replacement)
├── indexers/          # Indexer subsystem (Prowlarr replacement)
│   ├── cardigann/     # Cardigann YAML engine + 542 definitions
│   ├── scoring.go     # Search result scoring (0-100)
│   ├── handlers.go    # REST API handlers
│   └── registry.go    # Multi-indexer search aggregation
├── downloads/         # Download client adapters
├── notifications/     # Notification channels & templates
├── safety/            # Release validation & review queue
├── scanner/           # Library filesystem scanner
├── eventbus/          # In-process event bus
└── server/            # HTTP server, routing, middleware

web/src/
├── pages/             # Route pages (dashboard, movies, series, indexers, etc.)
├── components/        # UI components (search dialog, indexer catalogue, etc.)
├── lib/               # API clients & utilities
└── hooks/             # React hooks (auth, etc.)
```
