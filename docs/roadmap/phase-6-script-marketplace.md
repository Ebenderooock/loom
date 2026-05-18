# Phase 6: Script Marketplace

### Goal
A community-driven, multi-source marketplace system for discovering, sharing, and installing scripts. Users can add multiple marketplace sources (like Copilot extension marketplaces or Helm chart repos), each providing a browsable catalog of scripts. Built-in client handles fetching, installing, updating, and managing scripts from any registered marketplace.

### Architecture Decisions
- **Multi-marketplace model:** Users can add multiple marketplace sources — each is a URL pointing to an index endpoint. Loom ships with one default official marketplace (GitHub-hosted). Third-party communities can run their own.
- **Marketplace protocol:** Each marketplace exposes a simple REST-like API or static JSON index. Loom's client fetches the index, caches it locally, and refreshes periodically.
- **Install methods:** Four supported — (1) one-click from marketplace browser, (2) direct Git clone from repo URL, (3) ZIP upload via UI, (4) URL download (GitHub release, etc.).
- **Security model:** No verification — user installs at their own risk (like Docker Hub). Clear warnings in UI that scripts run with Loom's permissions. Future: optional verified/reviewed badges per marketplace.
- **Update strategy:** Loom periodically checks installed scripts against their source marketplace for new versions, shows update indicators in UI, user confirms each update manually.
- **Manifest format:** Extends Phase 5's `manifest.json` with marketplace-specific fields (repository URL, categories, tags, screenshots, changelog).
- **Patterns to follow:**
  - Marketplace sources follow the same pattern as indexers — a list of configured sources, each fetched independently
  - Script installation reuses Phase 5's `scripts.Registry.Install()` as the final step
  - Update checking mirrors the download client health-check pattern (periodic background task)

---

### Sub-phase 6A: Marketplace Protocol & Sources

#### Backend

1. **Migration: marketplace tables** — `internal/storage/migrations/sqlite/00XX_marketplace.sql`
   ```sql
   CREATE TABLE marketplace_sources (
     id TEXT PRIMARY KEY,
     name TEXT NOT NULL,
     url TEXT NOT NULL UNIQUE,              -- base URL of the marketplace
     type TEXT NOT NULL DEFAULT 'index',    -- 'index' (static JSON) | 'api' (REST API)
     enabled BOOLEAN NOT NULL DEFAULT TRUE,
     is_official BOOLEAN NOT NULL DEFAULT FALSE,
     api_key TEXT,                           -- optional auth for private marketplaces
     last_synced_at TIMESTAMP,
     last_error TEXT,
     script_count INTEGER DEFAULT 0,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );

   -- Default official marketplace
   INSERT INTO marketplace_sources (id, name, url, is_official)
   VALUES ('official', 'Loom Community Scripts', 'https://raw.githubusercontent.com/loom-media/loom-scripts/main/index.json', TRUE);

   CREATE TABLE marketplace_cache (
     id TEXT PRIMARY KEY,
     source_id TEXT NOT NULL REFERENCES marketplace_sources(id) ON DELETE CASCADE,
     script_id TEXT NOT NULL,              -- unique ID within the marketplace
     name TEXT NOT NULL,
     description TEXT,
     author TEXT,
     version TEXT NOT NULL,
     license TEXT,
     homepage TEXT,
     repository TEXT,                      -- git clone URL
     download_url TEXT,                    -- direct download URL (ZIP/tar.gz)
     min_loom_version TEXT,
     categories TEXT,                      -- JSON array: ["search", "scoring", "organization"]
     tags TEXT,                            -- JSON array: ["anime", "quality", "custom-scoring"]
     hooks TEXT NOT NULL,                  -- JSON array of {hook, mode} registrations
     icon_url TEXT,
     screenshots TEXT,                     -- JSON array of image URLs
     readme TEXT,                          -- full README markdown (fetched on detail view)
     changelog TEXT,                       -- version changelog
     downloads INTEGER DEFAULT 0,
     stars INTEGER DEFAULT 0,
     created_at TIMESTAMP NOT NULL,
     updated_at TIMESTAMP NOT NULL,
     cached_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     UNIQUE(source_id, script_id)
   );
   CREATE INDEX idx_marketplace_cache_source ON marketplace_cache(source_id);
   CREATE INDEX idx_marketplace_cache_name ON marketplace_cache(name);
   CREATE INDEX idx_marketplace_cache_hooks ON marketplace_cache(hooks);

   -- Track installed script origins for update checking
   CREATE TABLE script_origins (
     script_id TEXT PRIMARY KEY REFERENCES scripts(id) ON DELETE CASCADE,
     source_id TEXT REFERENCES marketplace_sources(id) ON DELETE SET NULL,
     marketplace_script_id TEXT,           -- ID in the marketplace
     installed_version TEXT NOT NULL,
     installed_from TEXT NOT NULL,         -- 'marketplace' | 'git' | 'upload' | 'url'
     source_url TEXT,                      -- original install URL
     latest_version TEXT,                  -- latest known version from marketplace
     update_available BOOLEAN DEFAULT FALSE,
     last_update_check TIMESTAMP,
     installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   ```

2. **Marketplace types** — `internal/scripts/marketplace/types.go`
   - `Source` struct matching `marketplace_sources` table
   - `SourceType` enum: `index` (static JSON), `api` (REST)
   - `CachedScript` struct matching `marketplace_cache` table
   - `ScriptOrigin` struct matching `script_origins` table
   - `MarketplaceIndex` — the top-level JSON structure returned by a marketplace source:
     ```go
     type MarketplaceIndex struct {
         Version     int              `json:"version"`       // index format version
         Name        string           `json:"name"`
         Description string           `json:"description"`
         Scripts     []MarketplaceEntry `json:"scripts"`
         UpdatedAt   time.Time        `json:"updated_at"`
     }
     type MarketplaceEntry struct {
         ID              string            `json:"id"`
         Name            string            `json:"name"`
         Description     string            `json:"description"`
         Author          string            `json:"author"`
         Version         string            `json:"version"`
         License         string            `json:"license"`
         Homepage        string            `json:"homepage"`
         Repository      string            `json:"repository"`
         DownloadURL     string            `json:"download_url"`
         MinLoomVersion  string            `json:"min_loom_version"`
         Categories      []string          `json:"categories"`
         Tags            []string          `json:"tags"`
         Hooks           []HookRegistration `json:"hooks"`
         IconURL         string            `json:"icon_url"`
         Screenshots     []string          `json:"screenshots"`
         Changelog       string            `json:"changelog"`
         Downloads       int               `json:"downloads"`
         Stars           int               `json:"stars"`
         CreatedAt       time.Time         `json:"created_at"`
         UpdatedAt       time.Time         `json:"updated_at"`
     }
     ```
   - `InstallMethod` enum: `marketplace`, `git`, `upload`, `url`
   - `BrowseFilter`: query string, categories, tags, hooks, sort (popular/recent/name)

3. **sqlc queries** — `internal/storage/queries/sqlite/marketplace.sql`
   - Source CRUD: `CreateSource`, `GetSource`, `ListSources`, `UpdateSource`, `DeleteSource`
   - Cache: `UpsertCachedScript`, `ListCachedScripts` (with filters: source, categories, tags, hooks, search, pagination), `GetCachedScript`, `ClearCacheForSource`, `CountCachedScripts`
   - Origins: `CreateOrigin`, `GetOrigin`, `ListOriginsWithUpdates`, `UpdateOriginVersion`, `DeleteOrigin`
   - Search: `SearchCachedScripts` — full-text search across name + description + tags

4. **Marketplace client** — `internal/scripts/marketplace/client.go`
   - `SyncSource(ctx, sourceID) error` — fetch index from source URL, upsert into cache
   - `SyncAll(ctx) error` — sync all enabled sources
   - Supports two source types:
     - `index`: `GET {url}` → parse `MarketplaceIndex` JSON
     - `api`: `GET {url}/api/v1/scripts?page=N` → paginated REST API
   - HTTP client with: User-Agent (`Loom/{version}`), optional API key auth, ETag/If-Modified-Since caching, timeout, retries
   - Cache TTL: configurable (default 6 hours), force refresh on user action
   - `FetchReadme(ctx, sourceID, scriptID) (string, error)` — lazy-load README from `{repository}/README.md`

---

### Sub-phase 6B: Installation Engine

#### Backend

5. **Installer** — `internal/scripts/marketplace/installer.go`
   - `InstallFromMarketplace(ctx, sourceID, scriptID) (*Script, error)`:
     1. Get cached entry → determine download method (repository or download_url)
     2. Download → extract to temp dir → validate manifest → copy to `scripts/` dir
     3. Register with Phase 5 `scripts.Registry.Install()`
     4. Create `script_origins` record
   - `InstallFromGit(ctx, repoURL, ref) (*Script, error)`:
     1. `git clone --depth 1 --branch {ref} {repoURL}` to temp dir
     2. Validate manifest → copy to scripts dir → register → create origin
   - `InstallFromURL(ctx, downloadURL) (*Script, error)`:
     1. Download ZIP/tar.gz → extract to temp dir → validate → install
   - `InstallFromUpload(ctx, reader io.Reader) (*Script, error)`:
     1. Save uploaded ZIP to temp → extract → validate → install
   - **Validation pipeline** (all methods):
     1. `manifest.json` exists and is valid
     2. Executable specified in manifest exists and is... executable
     3. Hook registrations reference valid hook points
     4. No path traversal in file paths (security)
     5. Version compatibility: `min_loom_version` check against running Loom version
   - **Collision handling:** If script with same ID already installed → error (use update instead)
   - **Temp dir cleanup:** Always clean up temp dirs, even on error

6. **Updater** — `internal/scripts/marketplace/updater.go`
   - `CheckForUpdates(ctx) ([]UpdateAvailable, error)`:
     1. For each installed script with a marketplace origin: compare `installed_version` vs `latest_version` in cache
     2. For git-installed scripts: `git ls-remote {repoURL}` to check for new tags/commits
     3. Update `script_origins.latest_version` and `update_available` flag
   - `UpdateScript(ctx, scriptID) (*Script, error)`:
     1. Get origin → determine source → download new version → validate
     2. Backup current script dir (rename to `{id}.bak`)
     3. Install new version in place
     4. Update origin record with new version
     5. Hot-reload script in registry
     6. If install fails → restore backup
   - `UpdateAvailable` struct: script ID, name, current version, latest version, changelog, source
   - Background check: periodic task (configurable, default every 24h)
   - Notification: publish `script.update_available` event → shows in UI + optional notification channel

7. **Version compatibility** — `internal/scripts/marketplace/compat.go`
   - Parse `min_loom_version` from manifest using semver
   - `IsCompatible(scriptMinVersion, loomVersion) bool`
   - On Loom upgrade: re-check all installed scripts for compatibility warnings
   - UI shows compatibility badge: ✅ compatible, ⚠️ untested (no min_loom_version), ❌ incompatible

---

### Sub-phase 6C: Service & API

#### Backend

8. **Marketplace service** — `internal/scripts/marketplace/service.go`
   - **Sources:**
     - `AddSource(ctx, name, url, sourceType) (*Source, error)` — validate URL, initial sync
     - `RemoveSource(ctx, sourceID) error` — remove source + clear cache (doesn't uninstall scripts)
     - `EnableSource(ctx, sourceID)` / `DisableSource(ctx, sourceID)`
     - `SyncSource(ctx, sourceID) error` — manual refresh
     - `ListSources(ctx) ([]Source, error)`
   - **Browsing:**
     - `Browse(ctx, filter BrowseFilter) ([]CachedScript, int, error)` — paginated, filtered
     - `GetScriptDetail(ctx, sourceID, scriptID) (*CachedScript, error)` — with README
     - `GetCategories(ctx) ([]CategoryCount, error)` — all categories with script counts
     - `GetTags(ctx) ([]TagCount, error)` — all tags with counts
   - **Installation:**
     - `Install(ctx, method InstallMethod, source string) (*Script, error)` — unified entry point
     - `Uninstall(ctx, scriptID) error` — delegates to Phase 5 + cleans origin
     - `Update(ctx, scriptID) (*Script, error)` — update to latest
     - `CheckUpdates(ctx) ([]UpdateAvailable, error)`
   - **Update scheduler:**
     - Background goroutine, checks every 24h (configurable)
     - Publishes events for available updates

9. **API routes** — `internal/scripts/marketplace/handlers.go`
   - **Sources:**
     - `POST   /api/v1/marketplace/sources` — add marketplace source
     - `GET    /api/v1/marketplace/sources` — list sources with sync status
     - `PUT    /api/v1/marketplace/sources/{id}` — update source
     - `DELETE /api/v1/marketplace/sources/{id}` — remove source
     - `POST   /api/v1/marketplace/sources/{id}/sync` — force sync
   - **Browse:**
     - `GET    /api/v1/marketplace/scripts` — browse all scripts (filters: source, category, tag, hook, search, sort, pagination)
     - `GET    /api/v1/marketplace/scripts/{sourceID}/{scriptID}` — script detail with README
     - `GET    /api/v1/marketplace/categories` — category list with counts
     - `GET    /api/v1/marketplace/tags` — tag list with counts
   - **Install:**
     - `POST   /api/v1/marketplace/install` — install from marketplace (body: `{source_id, script_id}`)
     - `POST   /api/v1/marketplace/install/git` — install from git URL (body: `{url, ref}`)
     - `POST   /api/v1/marketplace/install/url` — install from download URL (body: `{url}`)
     - `POST   /api/v1/marketplace/install/upload` — install from uploaded ZIP (multipart form)
   - **Updates:**
     - `GET    /api/v1/marketplace/updates` — list available updates
     - `POST   /api/v1/marketplace/updates/{scriptID}` — update specific script
     - `POST   /api/v1/marketplace/updates/check` — force update check
   - **Compat:**
     - `GET    /api/v1/marketplace/compat` — compatibility status for all installed scripts

10. **Notification integration** — extend dispatcher
    - New event: `on_script_update_available`
    - Optional notification: "3 script updates available: Custom Scorer v2.1, Anime Renamer v1.5, ..."
    - `OnScriptUpdates` boolean on notification `Connection`

---

### Sub-phase 6D: Official Marketplace Index

11. **Index specification** — `docs/marketplace/INDEX_SPEC.md`
    - JSON schema for `MarketplaceIndex` format
    - How to host your own marketplace (static JSON on GitHub Pages, S3, or any HTTP server)
    - API mode specification (for dynamic marketplaces)
    - Versioning: `version: 1` in the index, Loom checks and handles accordingly

12. **Official Loom Scripts repo** — `github.com/loom-media/loom-scripts`
    - Repository structure:
      ```
      loom-scripts/
      ├── index.json              # auto-generated marketplace index
      ├── scripts/
      │   ├── custom-scorer/
      │   │   ├── manifest.json
      │   │   ├── scorer.py
      │   │   └── README.md
      │   ├── anime-renamer/
      │   │   ├── manifest.json
      │   │   ├── renamer.sh
      │   │   └── README.md
      │   └── ...
      ├── templates/
      │   ├── python/             # starter template for Python scripts
      │   ├── bash/               # starter template for Bash scripts
      │   ├── go/                 # starter template for Go scripts
      │   └── node/               # starter template for Node.js scripts
      └── .github/
          └── workflows/
              └── build-index.yml  # CI: validate manifests, build index.json
      ```
    - CI pipeline: on PR merge → validate all manifests → rebuild `index.json` → commit
    - Contribution guide: how to submit a script (PR with script dir + manifest)

13. **Starter templates** — included in the official repo and linked from Loom's script docs
    - Python template: `manifest.json` + `main.py` with argparse, JSON I/O boilerplate, example hook
    - Bash template: `manifest.json` + `script.sh` with jq-based JSON parsing
    - Go template: `manifest.json` + `main.go` with encoding/json I/O
    - Node.js template: `manifest.json` + `package.json` + `index.js` with readline JSON I/O
    - Each template includes: protocol handling, error handling, logging to stderr, config parsing, example for 2-3 common hooks

---

### Sub-phase 6E: Frontend

14. **Marketplace browser** — `web/src/pages/marketplace.tsx`
    - Source selector tabs/dropdown (All / Official / per-custom-source)
    - Search bar with real-time filtering
    - Category sidebar: search, scoring, importing, organizing, notifications, etc.
    - Script grid cards: icon, name, author, description snippet, hook badges, star count, "Install" button
    - Sort: popular / recently updated / name / stars
    - Pagination or infinite scroll
    - "Installed" badge on already-installed scripts
    - "Update available" badge with version diff

15. **Script detail page** — `web/src/pages/marketplace/[sourceID]/[scriptID].tsx`
    - Hero: icon, name, author, version, license, star count, download count
    - Action button: "Install" / "Installed ✓" / "Update Available"
    - Tabs: README (markdown rendered), Hooks (which hooks + modes), Changelog, Configuration (JSON Schema preview)
    - Screenshots carousel (if provided)
    - Compatibility badge (compatible / untested / incompatible with current Loom version)
    - Source info: which marketplace, repository link

16. **Marketplace sources settings** — `web/src/pages/settings/marketplace.tsx`
    - List of configured sources with status (synced / error / disabled)
    - "Add Source" dialog: name, URL, type (index/api), optional API key
    - Per-source: enable/disable toggle, last synced time, script count, force sync button, remove button
    - Official source cannot be removed (only disabled)

17. **Update center** — `web/src/components/marketplace/update-center.tsx`
    - Shows all scripts with available updates
    - Per-script: current version → new version, changelog excerpt, "Update" button
    - "Update All" button for batch updates
    - Compatibility warnings for scripts that may not work with current Loom version
    - Accessible from: scripts page header badge, settings, system tray notification

18. **Install methods dialog** — `web/src/components/marketplace/install-dialog.tsx`
    - Tabs: "From Marketplace" (search + one-click), "From Git" (URL + ref input), "From URL" (download URL), "Upload" (file drop zone)
    - Validation feedback: manifest check results, compatibility status, hook registrations preview
    - Progress indicator during download + install

---

### Implementation Order

```
6A.1-3   Schema + types + queries              ─┐
6A.4     Marketplace client                     ─┘ Core foundation
6B.5     Installer (all 4 methods)              ── requires client + Phase 5 registry
6B.6     Updater                                ── requires installer + origins tracking
6B.7     Version compatibility                  ── requires manifest parsing
6C.8     Marketplace service                    ── requires client + installer + updater
6C.9     API routes                             ── requires service
6C.10    Notification integration               ── requires updater events
6D.11    Index specification docs               ── independent (can start early)
6D.12    Official loom-scripts repo             ── requires index spec + starter templates
6D.13    Starter templates                      ── requires Phase 5 protocol finalized
6E.14-18 Frontend                               ── requires API
```

### Dependencies
- **Phase 5 (Script Engine):** Must be fully implemented — marketplace installs into the script engine
- Phase 5's `scripts.Registry`, `scripts.Service`, `manifest.json` format, execution engine
- Git binary on PATH for git-clone installs
- HTTP connectivity for marketplace sync + downloads

### Risks
- **Stale indexes:** Static JSON indexes can go stale if the CI pipeline breaks. Mitigate: show "last synced" time prominently, handle fetch failures gracefully.
- **Large indexes:** Popular marketplaces could have hundreds of scripts. Need pagination in the protocol and efficient local caching.
- **Git clone failures:** Private repos, rate limits, network issues. Mitigate: clear error messages, retry logic, fallback to download URL.
- **Script conflicts:** Two marketplaces offering scripts with the same ID. Mitigate: scope script IDs by source (`official/custom-scorer` vs `community/custom-scorer`).
- **Update rollbacks:** Failed updates could leave scripts in broken state. Mitigate: backup before update, restore on failure.
- **Disk space:** Many installed scripts could consume significant space. Show total scripts disk usage in settings.
- **Marketplace trust:** Users adding random marketplace URLs. Mitigate: clear warning when adding non-official sources ("Scripts from third-party sources run with Loom's permissions. Only add sources you trust.")
