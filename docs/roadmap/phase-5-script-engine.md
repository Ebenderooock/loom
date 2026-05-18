# Phase 5: Script Engine

### Goal
Allow community-created scripts to hook into, extend, or replace any stage of Loom's pipelines — searching, scoring, grabbing, importing, organizing, scanning, workflow transitions, and notifications. Language-agnostic via JSON stdin/stdout protocol. This is Loom's primary extensibility mechanism.

### Architecture Decisions
- **Execution model:** External process only (no embedded VM). Scripts are standalone executables (any language — Python, Go, Bash, Node, Rust, etc.). Loom spawns the process, pipes JSON to stdin, reads JSON from stdout. Simple, secure, debuggable.
- **No Lua/JS embedding:** Adds complexity, CGO dependencies, and security surface. External process model keeps Loom pure Go and gives script authors full language choice.
- **Hook modes:** Three modes per hook point:
  - `pre_*` — runs before built-in logic, can modify input or abort
  - `post_*` — runs after built-in logic, can modify output or trigger side effects
  - `replace_*` — completely replaces built-in logic (script IS the implementation)
- **Protocol versioning:** All JSON payloads include a `protocol_version` field. Scripts declare which version they support. Loom maintains backward compat.
- **Fail-safe:** Script errors never crash Loom. Timeouts, panics, and bad output all fall back to built-in behavior with a logged warning.
- **Performance:** Scripts are only invoked when registered for a hook. No overhead on pipelines with no active scripts. Async hooks (fire-and-forget) available for `on_event` type hooks.
- **Script discovery:** Scripts live in a `scripts/` directory (configurable). Each script is a directory containing a `manifest.json` + executable(s).

### Hook Surface Map

Based on pipeline analysis, here are all hook points with exact injection locations:

| Hook Point | Pipeline | Injection Location | Input Context | Output |
|------------|----------|-------------------|---------------|--------|
| `pre_search` | Search | Before `Registry.Search` fan-out | `Query` (title, year, categories, indexer IDs) | Modified `Query`, skip indexers, abort |
| `post_search` | Search | After `ScoreResults`, before return | `[]Result` with scores | Filter/re-order/modify results |
| `replace_search` | Search | Replaces `Registry.Search` entirely | `Query` | `[]Result` (script does its own search) |
| `pre_score` | Scoring | Before `ScoreResults` | `Result` (raw fields: seeders, size, age, quality, freeleech) | Modified fields before scoring |
| `post_score` | Scoring | After `ScoreResults` | `Result` with computed score | Modified score, custom ranking |
| `replace_score` | Scoring | Replaces `ScoreResults` entirely | `[]Result` | Scored `[]Result` |
| `pre_grab` | Download | Before `client.Add` in `handleIndexerResult` | `Result`, `AddRequest`, client info | Allow/deny, modify request |
| `post_grab` | Download | After successful `client.Add` | `Result`, `AddRequest`, download ID | Side effects (logging, tagging) |
| `pre_import` | Import | Before `importSingleFile` in pipeline | Download path, parsed release, `MatchResult` | Modify match, skip, change destination |
| `post_import` | Import | After successful import | Imported file path, media metadata | Side effects (post-processing) |
| `replace_import` | Import | Replaces `importSingleFile` | Download path, parsed release | Import result (script handles the file) |
| `pre_organize` | Organizer | Before `BuildTargetPath` | Movie/episode, current path, `NamingConfig` | Modified naming tokens, custom path |
| `post_organize` | Organizer | After file move/link | Old path, new path, media metadata | Side effects (update external systems) |
| `replace_organize` | Organizer | Replaces `organizeFile` | Movie/episode, source path, library root | Target path + move strategy |
| `pre_scan` | Scanner | Before `processFile` per file | File path, size, parsed release | Skip/include, pre-match hint |
| `post_scan` | Scanner | After scan completes | Scan results, matched/unmatched counts | Side effects (notifications, reports) |
| `on_workflow` | Workflow | On state transitions | Workflow state, command, media info | Modify transition (retry strategy, cancel) |
| `on_event` | Event Bus | On any event bus topic | Event topic + payload | Side effects only (async, fire-and-forget) |
| `on_notification` | Notifications | Before notification send | `Notification` (title, message, data) | Modified message, suppress, add channels |

---

### Sub-phase 5A: Core Engine

#### Backend

1. **Migration: scripts tables** — `internal/storage/migrations/sqlite/00XX_scripts.sql`
   ```sql
   CREATE TABLE scripts (
     id TEXT PRIMARY KEY,
     name TEXT NOT NULL,
     description TEXT,
     version TEXT NOT NULL,
     author TEXT,
     homepage TEXT,
     license TEXT,
     executable TEXT NOT NULL,            -- relative path to executable within script dir
     hooks TEXT NOT NULL,                 -- JSON array of {hook, mode} registrations
     config_schema TEXT,                  -- JSON Schema for script configuration
     config_values TEXT,                  -- JSON current config values
     enabled BOOLEAN NOT NULL DEFAULT TRUE,
     priority INTEGER DEFAULT 100,       -- execution order (lower = first)
     timeout_seconds INTEGER DEFAULT 30,
     last_run_at TIMESTAMP,
     last_error TEXT,
     install_path TEXT NOT NULL,          -- absolute path to script directory
     installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );

   CREATE TABLE script_executions (
     id TEXT PRIMARY KEY,
     script_id TEXT NOT NULL REFERENCES scripts(id) ON DELETE CASCADE,
     hook TEXT NOT NULL,
     mode TEXT NOT NULL,                  -- 'pre' | 'post' | 'replace'
     status TEXT NOT NULL,               -- 'success' | 'error' | 'timeout' | 'skipped'
     duration_ms INTEGER NOT NULL,
     input_json TEXT,                    -- truncated input (for debugging)
     output_json TEXT,                   -- truncated output
     error_message TEXT,
     stderr_output TEXT,                 -- captured stderr
     triggered_by TEXT,                  -- what triggered this (media title, search query, etc.)
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   CREATE INDEX idx_script_executions_script ON script_executions(script_id);
   CREATE INDEX idx_script_executions_hook ON script_executions(hook);
   CREATE INDEX idx_script_executions_created ON script_executions(created_at);

   CREATE TABLE script_execution_retention (
     id TEXT PRIMARY KEY DEFAULT 'default',
     max_entries INTEGER DEFAULT 1000,
     max_age_days INTEGER DEFAULT 30
   );
   INSERT INTO script_execution_retention (id) VALUES ('default');
   ```

2. **Script types** — `internal/scripts/types.go`
   - `Script` struct matching the table
   - `HookPoint` enum: all 19 hook points from the surface map
   - `HookMode` enum: `pre`, `post`, `replace`
   - `HookRegistration`: `{ Hook: HookPoint, Mode: HookMode }`
   - `ScriptExecution` struct for execution log
   - `ScriptManifest` struct (parsed from `manifest.json`):
     ```go
     type ScriptManifest struct {
         Name            string              `json:"name"`
         Description     string              `json:"description"`
         Version         string              `json:"version"`
         Author          string              `json:"author"`
         Homepage        string              `json:"homepage"`
         License         string              `json:"license"`
         MinLoomVersion  string              `json:"min_loom_version"`
         Executable      string              `json:"executable"`
         Hooks           []HookRegistration   `json:"hooks"`
         ConfigSchema    json.RawMessage      `json:"config_schema"`
         DefaultConfig   json.RawMessage      `json:"default_config"`
     }
     ```

3. **JSON I/O protocol** — `internal/scripts/protocol.go`
   ```go
   // Input sent to script via stdin
   type ScriptInput struct {
       ProtocolVersion int             `json:"protocol_version"`  // currently 1
       Hook            string          `json:"hook"`              // e.g. "pre_search"
       Mode            string          `json:"mode"`              // "pre", "post", "replace"
       Config          json.RawMessage `json:"config"`            // script's config values
       Context         json.RawMessage `json:"context"`           // hook-specific data
   }

   // Output read from script via stdout
   type ScriptOutput struct {
       Action   string          `json:"action"`    // "continue" | "abort" | "modify" | "skip"
       Modified json.RawMessage `json:"modified"`  // modified context (for "modify" action)
       Error    string          `json:"error"`     // error message (for "abort")
       Log      []string        `json:"log"`       // optional log lines
   }
   ```
   - `action: "continue"` — proceed with unmodified input (default if no output)
   - `action: "modify"` — proceed with `modified` data replacing original context
   - `action: "abort"` — stop pipeline with error message
   - `action: "skip"` — skip this item (e.g., skip a search result, skip a file import)

4. **Execution engine** — `internal/scripts/engine.go`
   - `Execute(ctx, script, hook, mode, context) (*ScriptOutput, error)`
   - Spawn process: `exec.CommandContext(ctx, script.Executable)`
   - Set working dir to script's install path
   - Pipe `ScriptInput` JSON to stdin
   - Read `ScriptOutput` JSON from stdout
   - Capture stderr separately (for logging/debugging)
   - Timeout enforcement via context deadline
   - Resource limits: `ulimit` -style via `syscall.SysProcAttr` (max memory, max CPU time)
   - On any error (timeout, bad JSON, crash): log warning, return `action: "continue"` (fail-safe)

5. **Hook dispatcher** — `internal/scripts/dispatcher.go`
   - `Dispatcher` holds reference to script registry + engine
   - `RunHook(ctx, hook, mode, context) (*ScriptOutput, error)`:
     1. Query registry for scripts registered on this hook+mode
     2. Sort by priority (lower first)
     3. Execute sequentially (pre/replace) or in parallel (post/on_event)
     4. For `pre` chain: each script's `modified` output becomes next script's input
     5. For `replace`: only one replace script allowed per hook (first wins, log warning if multiple)
     6. For `post`/`on_event`: all run in parallel, outputs collected but not chained
   - `RunHookAsync(ctx, hook, mode, context)` — fire-and-forget for `on_event` hooks

6. **Script registry** — `internal/scripts/registry.go`
   - `Discover(scriptsDir)` — walk directory, find `manifest.json` files, validate, register
   - `Load(scriptID)` — load from DB, verify executable exists
   - `Install(path)` — copy script dir to scripts directory, validate manifest, insert DB row
   - `Uninstall(scriptID)` — remove from DB + filesystem
   - `Enable(scriptID)` / `Disable(scriptID)`
   - `ListByHook(hook, mode) []Script` — get all scripts for a hook point, ordered by priority
   - `HotReload()` — re-scan directory for changes (called on filesystem watch or API trigger)

7. **sqlc queries** — `internal/storage/queries/sqlite/scripts.sql`
   - Script CRUD: `CreateScript`, `GetScript`, `ListScripts`, `UpdateScript`, `DeleteScript`
   - `ListScriptsByHook` — filter by hook point
   - `UpdateScriptConfig`, `UpdateScriptEnabled`, `UpdateScriptLastRun`
   - Execution log: `CreateExecution`, `ListExecutions` (with filters + pagination), `PruneExecutions`
   - Config: `GetRetentionConfig`, `UpdateRetentionConfig`

---

### Sub-phase 5B: Pipeline Integration

8. **Search hooks** — modify `internal/indexers/service.go` + `scoring.go`
   - Before `Registry.Search`: call `RunHook("pre_search", "pre", query)` → may modify query
   - Check for `replace_search`: if registered, call script instead of `Registry.Search`
   - After `ScoreResults`: call `RunHook("post_search", "post", results)` → may filter/reorder
   - Before `ScoreResults`: call `RunHook("pre_score", "pre", result)` per result
   - After `ScoreResults`: call `RunHook("post_score", "post", result)` per result
   - Check for `replace_score`: if registered, call script instead of `ScoreResults`

9. **Grab hooks** — modify `internal/downloads/router.go`
   - Before `client.Add`: call `RunHook("pre_grab", "pre", {result, addRequest, client})` → may deny or modify
   - After successful `client.Add`: call `RunHookAsync("post_grab", "post", {result, downloadID})`

10. **Import hooks** — modify `internal/imports/pipeline.go`
    - Before `importSingleFile`: call `RunHook("pre_import", "pre", {path, release, match})` → may skip or modify
    - Check for `replace_import`: if registered, script handles entire import
    - After successful import: call `RunHookAsync("post_import", "post", {importedPath, media})`

11. **Organizer hooks** — modify `internal/organizer/organizer.go`
    - Before `BuildTargetPath`: call `RunHook("pre_organize", "pre", {movie, file, config})` → may override path
    - Check for `replace_organize`: script returns target path + strategy
    - After file move: call `RunHookAsync("post_organize", "post", {oldPath, newPath, media})`

12. **Scanner hooks** — modify `internal/scanner/scanner.go`
    - Before `processFile`: call `RunHook("pre_scan", "pre", {path, size, parsed})` → skip/include
    - After scan completes: call `RunHookAsync("post_scan", "post", {results, stats})`

13. **Workflow hooks** — modify `internal/workflows/orchestrator.go`
    - On state transitions: call `RunHook("on_workflow", "post", {workflow, command, state})` → may modify retry strategy

14. **Event bus hooks** — `internal/scripts/events.go`
    - Subscribe to `*` (all topics) on event bus
    - For each event: check for `on_event` scripts → call `RunHookAsync`
    - Scripts can filter by topic in their logic

15. **Notification hooks** — modify `internal/notifications/dispatcher.go`
    - Before `sender.Send`: call `RunHook("on_notification", "pre", {notification})` → may modify message or suppress

---

### Sub-phase 5C: Script Service + API

16. **Script service** — `internal/scripts/service.go`
    - `Install(ctx, sourcePath) (*Script, error)` — validate manifest, copy to scripts dir, register
    - `Uninstall(ctx, scriptID) error` — disable, remove from DB + filesystem
    - `Enable(ctx, scriptID)` / `Disable(ctx, scriptID)`
    - `Configure(ctx, scriptID, config json.RawMessage)` — validate against config schema, save
    - `TestRun(ctx, scriptID, hook, sampleInput)` — execute with sample data, return output
    - `ListScripts(ctx, filter)` — paginated list with execution stats
    - `GetScript(ctx, scriptID)` — detail with recent executions
    - `ListExecutions(ctx, filter)` — execution log with filters
    - `RefreshRegistry(ctx)` — re-scan scripts directory
    - `GetHookSurface(ctx)` — return all hook points with active script counts

17. **API routes** — `internal/scripts/handlers.go`
    - `GET    /api/v1/scripts` — list installed scripts
    - `GET    /api/v1/scripts/{id}` — script detail + recent executions
    - `POST   /api/v1/scripts/install` — install from upload or path
    - `DELETE /api/v1/scripts/{id}` — uninstall
    - `PUT    /api/v1/scripts/{id}/enable` — enable
    - `PUT    /api/v1/scripts/{id}/disable` — disable
    - `PUT    /api/v1/scripts/{id}/config` — update configuration
    - `POST   /api/v1/scripts/{id}/test` — test run with sample data
    - `GET    /api/v1/scripts/{id}/executions` — execution log
    - `POST   /api/v1/scripts/refresh` — re-scan scripts directory
    - `GET    /api/v1/scripts/hooks` — list all hook points with active script counts
    - `GET    /api/v1/scripts/executions` — global execution log (all scripts)

18. **Script SDK documentation** — `docs/scripts/README.md`
    - Protocol specification (JSON I/O format, versioning)
    - All hook points with input/output schemas
    - Example scripts in Python, Bash, Go, Node.js
    - `manifest.json` specification
    - Config schema guide (JSON Schema)
    - Best practices (error handling, logging, performance)
    - Template project scaffolding

---

### Sub-phase 5D: Frontend

19. **Scripts page** — `web/src/pages/scripts.tsx`
    - Grid of installed scripts with: name, author, version, hook badges, enabled toggle, last run status
    - "Install Script" button (upload ZIP or enter path)
    - "Refresh" button to re-scan directory
    - Filter by hook point, status

20. **Script detail view** — `web/src/pages/scripts/[id].tsx`
    - Info card: name, description, version, author, homepage, license
    - Hook registrations: which hooks, which modes, priority
    - Configuration form: auto-generated from JSON Schema (like Overseerr's settings)
    - Execution log: recent runs with status, duration, expandable input/output JSON
    - "Test Run" panel: select hook, provide sample input, see output

21. **Hook point visualization** — `web/src/components/scripts/hook-map.tsx`
    - Visual pipeline diagram showing all hook points
    - Each hook shows count of active scripts (badge)
    - Click hook → shows which scripts are registered + their priority order
    - Drag-and-drop priority reordering

22. **Execution log viewer** — `web/src/components/scripts/execution-log.tsx`
    - Global log across all scripts
    - Filters: script, hook, status (success/error/timeout), date range
    - Expandable rows: full JSON input/output, stderr, duration
    - Auto-refresh toggle for real-time monitoring

23. **Script SDK page** — `web/src/pages/scripts/docs.tsx`
    - In-app documentation for script authors
    - Protocol reference, hook point catalog, example code
    - Link to full docs / GitHub template

---

### Implementation Order

```
5A.1-2   Schema + types                          ─┐
5A.3     JSON I/O protocol                        ─┤ Core foundation
5A.4     Execution engine                         ─┤
5A.5     Hook dispatcher                          ─┤
5A.6-7   Script registry + queries                ─┘
5B.8     Search hooks                             ─┐
5B.9     Grab hooks                               ─┤
5B.10    Import hooks                             ─┤ Pipeline integration (can parallelize)
5B.11    Organizer hooks                          ─┤
5B.12    Scanner hooks                            ─┤
5B.13    Workflow hooks                           ─┤
5B.14    Event bus hooks                          ─┤
5B.15    Notification hooks                       ─┘
5C.16    Script service                           ── requires engine + registry
5C.17    API routes                               ── requires service
5C.18    SDK documentation                        ── requires protocol finalized
5D.19-23 Frontend                                 ── requires API
```

### Dependencies
- All pipeline packages (indexers, downloads, imports, organizer, scanner, workflows, notifications) need hook injection points
- Existing `eventbus.Bus` for `on_event` hooks
- Phases 1-4 should be stable before adding hooks (to avoid churn in hook contracts)

### Risks
- **Performance:** Spawning processes on every search/grab adds latency (~10-50ms per script). Mitigate with: only invoke when scripts are registered, async for post/event hooks, optional caching.
- **Script crashes:** Malformed output, panics, hangs. Mitigate with: strict timeout, fail-safe fallback to built-in behavior, stderr capture.
- **Security:** Scripts can access filesystem and network. Mitigate with: configurable `scripts/` directory, optional `chroot`/`seccomp` profiles for Linux, clear documentation that scripts run with Loom's permissions.
- **Protocol stability:** Changing hook input schemas breaks existing scripts. Mitigate with: `protocol_version` field, semantic versioning, backward-compat layer for old versions.
- **Replace mode conflicts:** Multiple scripts registering `replace_*` for the same hook. Mitigate with: first-by-priority wins, log warning, UI shows conflict indicator.
- **Debugging difficulty:** JSON-over-stdin is hard to debug interactively. Mitigate with: test runner in UI, example scripts with logging, `--debug` flag that dumps full I/O to stderr.
