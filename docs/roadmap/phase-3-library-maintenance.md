# Phase 3: Library Maintenance

### Goal
Automated and manual library health operations — identifying problems, cleaning up, reorganizing, and optimizing media libraries. All 7 check types are user-configurable for scheduled runs. Three auto-fix modes (manual, suggest+confirm, auto-fix safe) as a global setting. All destructive operations have dry-run/preview mode.

### Architecture Decisions
- **ffprobe strategy:** Auto-detect on PATH, default enabled if found. Two Docker images: `loom` (slim, no ffprobe) and `loom-full` (linuxserver base with ffprobe). Config toggle to disable even if detected.
- **Auto-fix modes:** Global setting switchable between: (1) Manual only — always show issues, (2) Suggest+confirm — one-click fix but no auto-execution, (3) Auto-fix safe — auto-delete orphans, auto-rename mismatches, require approval for destructive ops.
- **Dry-run:** All destructive operations (delete, move, rename) have preview mode that shows what would change before executing.
- **Scheduled scope:** User configures which checks run on schedule (any combination of: orphans, duplicates, naming, upgrades, integrity, space, stale). Schedule is configurable (manual/daily/weekly/custom cron).
- **Stale media:** Depends on Phase 4 watch data — stubbed initially, fully functional after Phase 4.
- **Patterns to follow:**
  - Scheduler pattern: service struct with `Start/Stop/Status`, wired in `wire_media.go`
  - Existing `libraries.Scanner.ScanLibrary` for file walking
  - Existing `organizer.Organizer.PreviewAll/OrganizeMovies` for naming checks + fixes
  - Quality profiles `Cutoff` + `UpgradeAllowed` for upgrade detection
  - Event bus for maintenance completion events + notifications

---

### Sub-phase 3A: Core Maintenance Engine

#### Backend

1. **Migration: maintenance tables** — `internal/storage/migrations/sqlite/00XX_maintenance.sql`
   ```sql
   CREATE TABLE maintenance_issues (
     id TEXT PRIMARY KEY,
     library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
     run_id TEXT NOT NULL,
     type TEXT NOT NULL,              -- 'orphan' | 'duplicate' | 'naming' | 'upgrade' | 'integrity' | 'space' | 'stale'
     severity TEXT NOT NULL,          -- 'info' | 'warning' | 'critical'
     status TEXT NOT NULL DEFAULT 'open',  -- 'open' | 'resolved' | 'ignored' | 'auto_fixed'
     title TEXT NOT NULL,
     description TEXT,
     file_path TEXT,
     media_id TEXT,                   -- FK to movies/series if applicable
     media_type TEXT,                 -- 'movie' | 'series'
     metadata JSONB,                  -- type-specific data (duplicate group ID, expected name, quality info, ffprobe output, etc.)
     reclaimable_bytes INTEGER DEFAULT 0,
     suggested_action TEXT,           -- 'delete' | 'rename' | 'move' | 'upgrade' | 'redownload' | 'ignore'
     resolved_at TIMESTAMP,
     resolved_by TEXT,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   CREATE INDEX idx_maintenance_issues_run ON maintenance_issues(run_id);
   CREATE INDEX idx_maintenance_issues_type ON maintenance_issues(type, status);
   CREATE INDEX idx_maintenance_issues_library ON maintenance_issues(library_id, status);

   CREATE TABLE maintenance_runs (
     id TEXT PRIMARY KEY,
     library_id TEXT,                 -- NULL = all libraries
     checks_run TEXT NOT NULL,        -- JSON array of check types run
     status TEXT NOT NULL DEFAULT 'running',  -- 'running' | 'completed' | 'failed' | 'cancelled'
     trigger TEXT NOT NULL,           -- 'manual' | 'scheduled' | 'auto'
     issues_found INTEGER DEFAULT 0,
     issues_auto_fixed INTEGER DEFAULT 0,
     reclaimable_bytes INTEGER DEFAULT 0,
     started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     completed_at TIMESTAMP,
     error TEXT
   );

   CREATE TABLE maintenance_config (
     id TEXT PRIMARY KEY DEFAULT 'default',
     auto_fix_mode TEXT NOT NULL DEFAULT 'suggest',  -- 'manual' | 'suggest' | 'auto_safe'
     schedule_enabled BOOLEAN NOT NULL DEFAULT FALSE,
     schedule_cron TEXT DEFAULT '0 3 * * 0',         -- default: Sunday 3am
     enabled_checks TEXT NOT NULL DEFAULT '["orphans","duplicates","naming","upgrades","space"]',  -- JSON array
     ffprobe_enabled BOOLEAN NOT NULL DEFAULT TRUE,   -- auto-disabled if not detected
     ffprobe_path TEXT,                               -- override path, NULL = auto-detect
     ffprobe_timeout INTEGER DEFAULT 30,              -- seconds per file
     stale_threshold_days INTEGER DEFAULT 90,          -- days unwatched = stale (Phase 4)
     ignore_patterns TEXT DEFAULT '[]',                -- JSON array of glob patterns to skip
     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   INSERT INTO maintenance_config (id) VALUES ('default');
   ```

2. **Maintenance types** — `internal/maintenance/types.go`
   - `Issue` struct matching the table
   - `Run` struct for run history
   - `Config` struct for settings
   - `CheckType` enum: `orphans`, `duplicates`, `naming`, `upgrades`, `integrity`, `space`, `stale`
   - `Severity` enum: `info`, `warning`, `critical`
   - `IssueStatus` enum: `open`, `resolved`, `ignored`, `auto_fixed`
   - `AutoFixMode` enum: `manual`, `suggest`, `auto_safe`
   - `SuggestedAction` enum: `delete`, `rename`, `move`, `upgrade`, `redownload`, `ignore`
   - `CheckResult` struct: issues found, time taken, errors

3. **sqlc queries** — `internal/storage/queries/sqlite/maintenance.sql`
   - `CreateRun`, `UpdateRun`, `ListRuns`, `GetRun`
   - `CreateIssue`, `BulkCreateIssues`, `ListIssues` (with type/status/library filters + pagination)
   - `UpdateIssueStatus`, `BulkUpdateIssueStatus`, `CountIssuesByType`, `SumReclaimableBytes`
   - `GetConfig`, `UpdateConfig`

4. **Check interface + engine** — `internal/maintenance/engine.go`
   ```go
   type Checker interface {
       Type() CheckType
       Check(ctx context.Context, library Library, opts CheckOptions) ([]Issue, error)
   }
   type Engine struct { checkers map[CheckType]Checker }
   func (e *Engine) Run(ctx context.Context, libraryID *string, checks []CheckType) (*Run, error)
   ```
   - Runs selected checkers in sequence per library
   - Publishes `maintenance.completed` event on bus
   - Respects cancellation

5. **Orphan checker** — `internal/maintenance/check_orphans.go`
   - Walk library root dirs (reuse `libraries.Scanner` walking logic)
   - Cross-reference each video file against `movie_files`, `episode_files`, `library_files`
   - Any file not tracked = orphan issue
   - Suggested action: `delete` (severity: `warning`)
   - Auto-fix (if `auto_safe`): delete orphan files, clean empty dirs

6. **Duplicate checker** — `internal/maintenance/check_duplicates.go`
   - Group movie files by TMDB ID, series episode files by series+season+episode
   - For each group with >1 file: compare quality scores against quality profile
   - Identify which copy to keep (highest quality score) and which are duplicates
   - Suggested action: `delete` the lower-quality copy (severity: `info`)
   - Metadata: `{ "group_id": "tmdb:12345", "keep": "path/to/best.mkv", "quality_score": 85 }`

7. **Naming checker** — `internal/maintenance/check_naming.go`
   - Use `organizer.Organizer.PreviewAll()` to get expected vs actual paths
   - Any mismatch = naming issue
   - Suggested action: `rename` (severity: `info`)
   - Auto-fix (if `auto_safe`): call `organizer.OrganizeMovies()` / `OrganizeSeries()`

8. **Upgrade checker** — `internal/maintenance/check_upgrades.go`
   - For each movie/episode file: compare current quality against quality profile's `Cutoff`
   - If `UpgradeAllowed` and current quality < cutoff = upgradeable
   - Suggested action: `upgrade` → triggers search for better quality (severity: `info`)
   - Metadata: `{ "current_quality": "HDTV-720p", "cutoff": "Bluray-1080p", "profile": "HD" }`

9. **Integrity checker** — `internal/maintenance/check_integrity.go`
   - **Quick mode** (no ffprobe): check file exists, size > 0, valid extension, size reasonable for runtime estimate
   - **Full mode** (ffprobe): run `ffprobe -v error -show_entries stream=codec_type,codec_name,width,height,duration -of json` on each file
   - Detect: zero-byte files, truncated files (duration much shorter than expected), unreadable streams, codec mismatches
   - Suggested action: `redownload` (severity: `critical` for corrupt, `warning` for mismatch)
   - Metadata: `{ "ffprobe_available": true, "expected_duration": 7200, "actual_duration": 1200, "error": "..." }`

10. **Space analyzer** — `internal/maintenance/check_space.go`
    - Aggregate reclaimable bytes from all other issue types (orphans, duplicates)
    - Calculate library breakdown: total size, by quality tier, by media type
    - Disk usage trends (compare against previous runs)
    - Suggested action: `delete` for the biggest wins (severity based on disk pressure)

11. **Stale media checker** — `internal/maintenance/check_stale.go`
    - **Stub for Phase 3** — depends on Phase 4 watch data
    - When Phase 4 available: query watch history, flag media not watched in `stale_threshold_days`
    - Suggested action: `delete` or `archive` (severity: `info`)
    - Metadata: `{ "last_watched": null, "days_since_download": 120 }`

12. **ffprobe integration** — `internal/maintenance/ffprobe.go`
    - Auto-detect: check `exec.LookPath("ffprobe")`
    - Config override: custom path via `maintenance_config.ffprobe_path`
    - `Probe(ctx, filePath) (*ProbeResult, error)` — run ffprobe with timeout, parse JSON output
    - `ProbeResult`: streams (codec, resolution, duration), format (size, bitrate), errors
    - Graceful degradation: if not available, integrity checker falls back to quick mode

13. **Fix executor** — `internal/maintenance/executor.go`
    - `Preview(ctx, issueIDs) ([]FixPreview, error)` — dry-run showing what would change
    - `Execute(ctx, issueIDs) ([]FixResult, error)` — actually perform fixes
    - `FixPreview`: issue, action, before path, after path, bytes freed
    - Delegates to: `os.Remove` (orphans/duplicates), `organizer.OrganizeMovie` (naming), search trigger (upgrades)
    - Audit log each action
    - Auto-fix runner: called by engine when `auto_safe` mode, only for safe actions (orphan delete, naming fix)

14. **Maintenance service** — `internal/maintenance/service.go`
    - `RunMaintenance(ctx, libraryID, checks)` — trigger a run
    - `CancelRun(ctx, runID)` — cancel in-progress run
    - `ListRuns(ctx, filter)` — history
    - `ListIssues(ctx, filter)` — paginated issue list
    - `ResolveIssues(ctx, issueIDs, action)` — fix or ignore
    - `IgnoreIssues(ctx, issueIDs)` — mark as ignored
    - `GetConfig(ctx)` / `UpdateConfig(ctx, config)`
    - `GetSummary(ctx, libraryID)` — counts by type, reclaimable bytes, last run

15. **Scheduled maintenance** — `internal/maintenance/scheduler.go`
    - Follows existing scheduler pattern: service struct with `Start/Stop/Status`
    - Reads `maintenance_config.schedule_cron` for timing
    - Runs only `enabled_checks` from config
    - Respects `ignore_patterns` for skipping files/dirs
    - Publishes `maintenance.scheduled.completed` event for notifications

16. **Notification integration** — extend dispatcher
    - New events: `on_maintenance_completed`, `on_maintenance_critical` (when critical issues found)
    - Add `OnMaintenance` boolean to notification `Connection`
    - Summary notification: "Maintenance completed: 5 orphans, 2 duplicates, 1 corrupt file. 12GB reclaimable."

17. **API routes** — `internal/maintenance/handlers.go`
    - `POST   /api/v1/maintenance/run` — trigger run (optional: library_id, checks[])
    - `DELETE /api/v1/maintenance/run/{id}` — cancel run
    - `GET    /api/v1/maintenance/runs` — list runs with pagination
    - `GET    /api/v1/maintenance/runs/{id}` — run detail with issue summary
    - `GET    /api/v1/maintenance/issues` — list issues (filters: type, status, library, severity)
    - `POST   /api/v1/maintenance/issues/preview` — dry-run preview for selected issues
    - `POST   /api/v1/maintenance/issues/fix` — execute fixes for selected issues
    - `POST   /api/v1/maintenance/issues/ignore` — mark issues as ignored
    - `GET    /api/v1/maintenance/summary` — dashboard summary (counts, reclaimable, last run)
    - `GET    /api/v1/maintenance/config` — get config
    - `PUT    /api/v1/maintenance/config` — update config
    - `GET    /api/v1/maintenance/ffprobe/status` — ffprobe detection status + version

18. **Docker images** — 3 variants
    - `loom:latest` (slim): current image, no ffprobe (~60MB)
    - `loom:ffprobe`: slim base + static ffprobe binary (~130MB)
    - `loom:lsio`: linuxserver base with ffprobe + full media toolchain (~200MB+)
    - CI builds all three; Dockerfiles: `Dockerfile`, `Dockerfile.ffprobe`, `Dockerfile.lsio`

#### Frontend

19. **Maintenance dashboard** — `web/src/pages/maintenance.tsx`
    - Health score ring (% of library without issues)
    - Issue counts by type as cards with icons + colors
    - Total reclaimable space (prominent)
    - Last run time + next scheduled run
    - Quick action: "Run Now" button with check type selection
    - Per-library breakdown (accordion or tabs)

20. **Issue list** — `web/src/components/maintenance/issue-list.tsx`
    - Filterable by: type, severity, status, library
    - Sortable by: severity, reclaimable bytes, date
    - Each issue card: file path, media title (if linked), suggested action, reclaimable space
    - Checkbox selection for bulk actions
    - Inline preview (shows before/after for rename, shows what gets deleted for orphans)

21. **Bulk actions toolbar** — `web/src/components/maintenance/bulk-actions.tsx`
    - "Preview Selected" → dry-run modal showing all changes
    - "Fix Selected" → confirm dialog → execute
    - "Ignore Selected" → dismiss issues
    - "Fix All [type]" — e.g., "Fix All Naming Issues"

22. **Maintenance settings** — `web/src/pages/settings/maintenance.tsx`
    - Auto-fix mode selector (manual / suggest+confirm / auto-fix safe)
    - Schedule toggle + cron builder (simple: day/time picker, advanced: raw cron)
    - Check type toggles (which checks run on schedule)
    - ffprobe status indicator (detected/not detected/disabled) + path override
    - Ignore patterns editor (glob patterns)
    - Stale threshold slider (days)

23. **Storage analytics** — `web/src/components/maintenance/storage-analytics.tsx`
    - Pie/donut chart: library space by quality tier
    - Bar chart: space by media type (movies vs series)
    - Reclaimable space breakdown (orphans vs duplicates vs upgrades)
    - Disk usage over time (if history available from multiple runs)

24. **ffprobe status** — `web/src/components/maintenance/ffprobe-status.tsx`
    - Detection status badge (green/yellow/red)
    - ffprobe version display
    - Toggle to enable/disable
    - Path override input
    - "Test ffprobe" button

---

### Implementation Order

```
3A.1-3    Schema + types + queries              ─┐
3A.4      Check interface + engine               ─┤ Core foundation
3A.12     ffprobe integration                    ─┘
3A.5-8    Orphan/Duplicate/Naming/Upgrade checks ── requires engine (can parallelize)
3A.9-10   Integrity + Space checks               ── requires ffprobe + other checks
3A.11     Stale media (stub)                     ── independent stub
3A.13     Fix executor + dry-run                 ── requires checks
3A.14     Maintenance service                    ── requires engine + executor
3A.15     Scheduled maintenance                  ── requires service
3A.16     Notification integration               ── requires service + event bus
3A.17     API routes                             ── requires service
3A.18     Docker images                          ── independent
3A.19-24  Frontend                               ── requires API
```

### Dependencies
- Existing `libraries.Scanner` for file walking
- Existing `organizer.Organizer` for naming checks + fixes
- Existing `qualityprofiles.Store` for upgrade detection
- Existing `movies.Service` / `series.Service` for media file queries
- Existing `eventbus.Bus` for events + notifications
- Existing `scheduler` pattern for scheduled runs
- Phase 4 for stale media detection (stubbed initially)

### Risks
- **ffprobe performance:** Full integrity scan on large libraries (10K+ files) could take hours. Need progress reporting, cancellation, and incremental scanning (only check new/changed files).
- **Orphan false positives:** Files being actively imported could be flagged as orphans. Need import-awareness (skip files in active download/import directories).
- **Auto-fix safety:** Even "safe" auto-fixes could go wrong (e.g., deleting a file that's a hardlink used by another library). Need hardlink detection.
- **Disk I/O impact:** Running all checks simultaneously on spinning disks could degrade performance. Consider sequential I/O or nice/ionice.
- **Docker image size:** linuxserver base + ffprobe could push `loom-full` past 200MB. Monitor and optimize.
