# Loom Application Status & Roadmap

**Last Updated**: Current Session  
**Status**: Movies feature 90% complete, E2E testing infrastructure ready

---

## Architecture Overview

Loom is a unified media management application replacing Radarr, Sonarr, and Prowlarr.

**Stack**:
- **Backend**: Go (chi router, sqlc for type-safe SQL)
- **Frontend**: React + TypeScript + Vite
- **Database**: SQLite
- **Testing**: Playwright E2E + MCP integration
- **Config**: JSON config file + SQLite database

### Config & Data Separation

```
config/loom.json (portable)           → Core configuration
  ├── setup_complete: bool
  ├── admin: {username, password_hash}
  ├── persistence: {engine, path}
  └── libraries: [{name, type, path}]

config/data/loom.db (instance-specific) → Operational data
  ├── media metadata
  ├── download history
  ├── session state
  └── indexer state
```

---

## Feature Implementation Status

### ✅ Phase 1: Auth & Config (100%)
- [x] Server-authoritative auth (no frontend localStorage)
- [x] Login endpoint with credential verification
- [x] Setup flow creates `config/loom.json`
- [x] Session management via HTTP cookies
- [x] Auth guard on protected endpoints

**Key Files**:
- `internal/auth/` - Auth handlers
- `internal/kernel/config/` - Config loading & validation
- `web/src/hooks/use-auth.tsx` - Frontend auth state

---

### ⚠️ Phase 2: Movies (Radarr) - 80% Complete

#### Completed:
- [x] Movies service & API endpoints
- [x] Root folder management (CRUD)
- [x] Database schema for libraries and root folders
- [x] Frontend movies page with Plex-style modal UI
- [x] Modal workflow: Choose type → Choose input → Enter path
- [x] Error display in modal (improves UX)
- [x] Auth check (shows login message if unauthenticated)

#### Missing:
- [ ] **Library scanning**: Endpoint to crawl disk and discover media files
  - Need `/api/v1/libraries/{libraryId}/scan` endpoint
  - Walk filesystem, parse metadata, create media records
- [ ] **Movie discovery UI**: Display discovered movies/shows in library
- [ ] **File browser**: UI component for browsing filesystem (currently stub)
- [ ] **Edit/Delete libraries**: Full CRUD on frontend
- [ ] **Library deletion cascade**: Delete all media when removing library

**Key Files**:
- `web/src/pages/movies.tsx` - UI with modal workflow
- `internal/movies/service.go` - Service layer
- `internal/movies/handlers.go` - API endpoints
- `db/migrations/` - Database schema

**Next Task**: Implement library scanning to discover media files on disk

---

### ⏳ Phase 3: Series (Sonarr) - 0% Complete

Needs to parallel Movies implementation:
- [ ] Series service & API endpoints
- [ ] Season/episode management
- [ ] Series discovery in library scanning
- [ ] Frontend UI for series management

---

### ⏳ Phase 4: Indexers (Prowlarr) - 0% Complete

- [ ] Indexer configuration endpoints
- [ ] Indexer search capability
- [ ] Result parsing and filtering

---

### ⏳ Phase 5: Download Clients - 0% Complete

- [ ] Download client configuration
- [ ] Download history tracking
- [ ] Integration with indexer search results

---

## Testing Infrastructure

### Playwright E2E Tests

**Status**: ✅ Configured and working

**Available Tests** (run with `pnpm test:e2e` in `web/`):
1. **smoke.spec.ts** - Dashboard loads with UI elements
2. **movies.spec.ts** - Movies page modal workflow (8 test cases)

**Test Coverage**:
- ✅ Modal flow for adding libraries
- ✅ Library type selection
- ✅ Manual path entry
- ✅ Navigation and back buttons
- ✅ Empty movie list display
- ⚠️ No auth flow tests (stretch goal)
- ⚠️ No actual movie scanning/discovery tests

### MCP Server Integration

**Status**: ✅ Ready for test execution

**Location**: `mcp-playwright/server.js`

**Tools Available**:
1. `run_tests` - Run tests with optional filtering
2. `list_tests` - List all available tests
3. `run_specific_test` - Run a specific test file

**Configuration**: `.mcp.json` registers playwright server

**Usage**:
```bash
# Run all movies tests
run_tests(filter: "movies")

# List available tests
list_tests()

# Run specific test file
run_specific_test(testFile: "smoke.spec.ts")
```

---

## Known Issues

### 🐛 Backend Issues
1. **API inconsistency**: Root folders returns array, movies returns paginated object
   - Remedy: Standardize all endpoints to paginated response format
2. **Missing library scanning**: No way to discover media files on disk
   - Remedy: Implement `/api/v1/libraries/{id}/scan` endpoint

### 🐛 Frontend Issues
1. **File browser stub**: Currently redirects to manual entry
   - Remedy: Implement filesystem browser UI component
2. **No actual media display**: Movies page shows empty list even if media exists
   - Remedy: Implement library scanning on backend first

### 🐛 Testing Issues
1. **No auth flow tests**: Tests assume authentication exists
   - Remedy: Add `beforeEach` hook to log in before tests
2. **No integration tests**: Only E2E tests exist
   - Remedy: Add integration tests for API endpoints

---

## Development Workflow

### Starting the Application

```bash
# Terminal 1: Backend
make dev

# Terminal 2: Frontend
cd web && pnpm dev

# Access at http://localhost:5173
```

### Running Tests

```bash
# All E2E tests
cd web && pnpm test:e2e

# Specific test file
cd web && pnpm exec playwright test movies.spec.ts

# Interactive UI mode
cd web && pnpm exec playwright test --ui

# Debug mode
cd web && pnpm exec playwright test --debug

# Via MCP (from Copilot CLI)
run_tests(filter: "movies")
```

### Building for Production

```bash
cd web && pnpm build
```

---

## File Structure

```
loom/
├── api/                          # API route definitions
├── cmd/                          # CLI commands
├── config/                       # Configuration directory
│   └── loom.json                # Generated by setup
├── db/
│   ├── migrations/              # SQL migrations
│   └── queries/                 # SQLC query files
├── deploy/                      # Deployment files
├── docs/                        # Documentation
├── internal/
│   ├── auth/                   # Authentication handlers
│   ├── kernel/                 # Core app setup
│   ├── movies/                 # Movies service (Radarr)
│   ├── server/                 # HTTP server setup
│   └── ...
├── web/                        # React frontend
│   ├── src/
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── pages/
│   │   │   ├── setup.tsx      # Setup/Init page
│   │   │   ├── auth.tsx       # Auth/Login page
│   │   │   ├── dashboard.tsx  # Main dashboard
│   │   │   └── movies.tsx     # Movies management
│   │   └── App.tsx
│   ├── e2e/                   # Playwright E2E tests
│   │   ├── smoke.spec.ts
│   │   └── movies.spec.ts
│   └── playwright.config.ts
├── mcp-playwright/            # MCP server for Playwright
│   └── server.js
├── .mcp.json                  # MCP servers registration
├── PLAYWRIGHT_SETUP.md        # This setup guide
└── LOOM_STATE.md             # (Archived) Old state tracking
```

---

## Next Steps (Priority Order)

### Immediate (Next Session)
1. **Implement library scanning** `/api/v1/libraries/{id}/scan`
   - Walk filesystem, discover video files
   - Parse metadata, create media records
   - Return list of discovered media
   
2. **Update movies UI to display discovered media**
   - Fetch and display media list after scanning
   - Show movie cards with poster/metadata

3. **Run Playwright tests**
   - Verify all tests pass against current implementation
   - Add auth setup to `beforeEach` if tests fail on auth

### Short Term (Next 2 Sessions)
4. **Implement file browser UI component**
   - Replace stub with filesystem navigation
   - Allow selecting library path via UI

5. **Add auth flow E2E tests**
   - Test login/setup flow
   - Test auth gate on movies page

6. **Standardize API response format**
   - All endpoints should return paginated format or all return arrays (pick one)
   - Update affected endpoints and frontend parsers

### Medium Term (Next Month)
7. **Implement Series service** (parallel to movies)
   - Service, handlers, database schema
   - Frontend UI for series management

8. **Add indexers support** (Prowlarr replacement)
   - Indexer configuration CRUD
   - Search integration with discovery

9. **Add download client integration**
   - Download client configuration
   - Download history tracking
   - Integration with search results

10. **Add CI/CD**
    - GitHub Actions for tests
    - Automated Docker builds

---

## Configuration Reference

### `config/loom.json` Example

```json
{
  "setup_complete": true,
  "admin": {
    "username": "admin",
    "password_hash": "$2a$12$..."
  },
  "persistence": {
    "engine": "sqlite",
    "path": "config/data/loom.db"
  },
  "libraries": [
    {
      "name": "Movies",
      "type": "movies",
      "path": "/media/movies"
    },
    {
      "name": "TV Shows",
      "type": "series",
      "path": "/media/tv"
    }
  ],
  "cors": {
    "allowed_origins": [
      "http://localhost:5173",
      "http://localhost:5174",
      "http://localhost:5175",
      "http://localhost:3000"
    ]
  }
}
```

---

## Contributing

When implementing new features:
1. Update this file to reflect status
2. Add corresponding E2E tests in `web/e2e/`
3. Use MCP tools to verify tests pass
4. Commit changes with test verification

---

## Resources

- **Playwright Docs**: https://playwright.dev/docs/intro
- **MCP Protocol**: https://modelcontextprotocol.io/
- **Go Chi Router**: https://github.com/go-chi/chi
- **SQLC**: https://sqlc.dev/
