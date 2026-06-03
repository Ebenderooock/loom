# Work Log

Single source of truth for requested changes, their triage state, and history.

## How to use this

- **Add** new asks under **📥 Inbox** as a one-line bullet — don't worry about format.
- During triage they get promoted to the **Backlog** table with an ID, type, priority and area, and (for bugs) a Details block below.
- Move the **Status** field through the lifecycle as work happens. Keep the ID stable forever so commits/PRs can reference it (e.g. `fix(LW-007): ...`).

**Status legend:** `inbox` → `approved` → `in-progress` → `done` (or `wont-do` / `blocked`)
**Type:** `bug` · `improvement` · `feature`  | **Priority:** `P1` (urgent/broken) · `P2` (important) · `P3` (nice-to-have)

> Scaling note: once this passes ~20 open items, prefer GitHub Issues (the `gh`
> CLI is wired up here). The table below maps 1:1 to issues — I can sync these
> to GitHub Issues with `type`/`priority`/`area` labels on request.

---

## 📥 Inbox (untriaged)

_(none — promote new asks into the Backlog below)_

---

## 🗂 Backlog (approved & triaged)

| ID | Type | Pri | Area | Title | Status |
|--------|-------------|----|-----------|-------------------------------------------------------------|-------------|
| LW-001 | bug         | P1 | Search    | Global quick-search across all platform content             | approved    |
| LW-002 | improvement | P3 | UI        | Rename "Browse" → "Manual Search"                           | done        |
| LW-003 | improvement | P2 | Downloads | UI controls for the built-in torrent downloader             | approved    |
| LW-004 | bug         | P1 | Settings  | Media Preferences page crashes (empty Select.Item value)    | done        |
| LW-005 | improvement | P2 | System    | Configurable Log Level, retention period, etc.              | approved    |
| LW-006 | improvement | P2 | Discover  | Discover filters (list/genre/year) + sorting                | approved    |
| LW-007 | bug         | P1 | Imports   | Post-download import reports failure though file was moved  | done        |
| LW-008 | improvement | P2 | Downloads | Downloads Cleanup Tool / Process                            | approved   |

### Details

#### LW-001 — Global quick-search across all platform content · `bug` · P1 · Search
Quick search is not fully implemented. It should let the user search for **any
content across the entire platform** (movies, series, indexers/releases,
settings, etc.), not just the current page's scope.

#### LW-002 — Rename "Browse" → "Manual Search" · `improvement` · P3 · UI
"Browse" is the wrong term for the manual indexer search action. Rename the
label (and any related copy) to **"Manual Search"**.

#### LW-003 — UI controls for the built-in torrent downloader · `improvement` · P2 · Downloads
Improve control over the built-in torrent downloader. Add a UI area to manage
the downloader (e.g. start/pause/limits/status), rather than it being
headless.

#### LW-004 — Media Preferences page crash · `bug` · P1 · Settings
Settings → **Media Preferences** crashes on render:
```
A <Select.Item /> must have a value prop that is not an empty string.
This is because the Select value can be set to an empty string to clear the
selection and show the placeholder.
```
Likely a `<SelectItem value="">` that needs a sentinel value (e.g. `"none"`)
plus mapping logic.

#### LW-005 — Configurable logging/retention · `improvement` · P2 · System
Add configuration settings for **Log Level**, **Retention Period**, and similar
operational knobs (surface in Settings → System).

#### LW-006 — Discover filtering & sorting · `improvement` · P2 · Discover
Discover should support filtering by **source list**, plus high-level filters
like **Genre** and **Year**, and allow **sorting**.

#### LW-007 — False import-failure after successful move · `bug` · P1 · Imports
Post-download import reports failures even though the file was moved
successfully. The retry loop then fails repeatedly because the source file is
already gone. Observed sequence:
```
import failed:
  remove /media/downloads/<release>/<file>.mkv: no such file or directory
retried → Re-importing after delay
import failed:
  no media files found in /media/downloads/<release>
retried → Re-importing after delay
import failed:
  no media files found in /media/downloads/<release>
```
Root cause (CONFIRMED): on a cross-device import (`ImportModeMove`),
`moveFile` (internal/imports/file_ops.go) does `os.Rename` → falls back to
`copyFile` (succeeds, file is now in the library) → `os.Remove(src)`. A
concurrent/duplicate import had already removed the source, so `os.Remove`
returned `ENOENT`. That error was propagated as the import error → workflow
marked failed → retried against the now-empty source dir → "no media files
found" on every retry.

Fix (commit pending): `moveFile` now ignores `os.IsNotExist` from the
post-copy `os.Remove(src)` — once the copy succeeds and the source is gone,
the move is complete, so it returns success instead of a false failure. Other
remove errors (e.g. permissions) still propagate. Covered by
`file_ops_test.go`.


#### LW-008 — Downloads Cleanup Tool / Process · `improvement` · P2 · Downloads
Downloads Cleanup tool reviews all the contents of the downloads folder against what is downloading, what is missing from the libraries and what is importing, if there are any orphaned items, they go onto a review list for the user to review, otherwise they get automatically deleted after a configurable amount of days (default 7).

---

## 🚧 In Progress

_(none)_

---

## ✔️ Done

| ID | Type | Area | Title | Shipped |
|--------|---------|------------|------------------------------------------------|----------|
| LW-000 | feature | Diagnostics | Feature-toggle system + Search Log enable/disable | v0.1.87 |
