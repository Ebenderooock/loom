# Workflow Engine

The workflow engine (`internal/workflows`) provides end-to-end stateful
tracking for every download Loom manages — from initial search through
grab, download, import, and completion (or failure with automatic retry).

## Motivation

The previous "grabs" package was a simple key-value store that recorded
which media was grabbed and by which download client. It had no concept
of lifecycle, retry, or self-healing. If a download failed silently (e.g.
the torrent disappeared from the client), the grab record would persist
indefinitely, blocking re-searches and leaving media in a "grabbed" limbo.

The workflow engine solves this by modelling each download as a state
machine with built-in timeout detection, retry logic, and media status
synchronisation.

## States

```text
  searching → grabbed → downloading → importing → completed
                 ↓            ↓            ↓
              failed ←────────┴────────────┘
                 ↓
         (retry with backoff, up to MaxRetries)
                 ↓
            cancelled (terminal — user intervention or final failure)
```

| State        | Description                                           |
| ------------ | ----------------------------------------------------- |
| `searching`  | Search initiated; indexers queried.                   |
| `grabbed`    | Release selected and sent to download client.         |
| `downloading`| Client acknowledged; download in progress.            |
| `importing`  | Download complete; import pipeline processing.        |
| `completed`  | Import successful; media marked available.            |
| `failed`     | An error occurred; imports auto-retry, downloads are re-searched.|
| `cancelled`  | Terminal; user cancelled or max retries exhausted.    |

## Components

### Store (`store.go`)

SQLite/Postgres persistence layer. Key methods:

- `Create(ctx, workflow)` — persist a new workflow.
- `UpdateState(ctx, id, state, message)` — transition state.
- `FindActiveForMedia(ctx, mediaType, mediaIDs)` — check if media already has an active workflow.
- `FindByDownload(ctx, clientID, downloadID)` — match a download event to its workflow.
- `ActiveMediaIDs(ctx, mediaType, ids)` — bulk lookup for UI grab indicators.
- `ListRecent(ctx, limit)` — paginated list for the Activity panel.
- `GetItems(ctx, workflowID)` — get media items linked to a workflow.

### Engine (`engine.go`)

State machine orchestrator. Key methods:

- `StartSearch(ctx, workflowType, mediaType, query, mediaIDs)` — create workflow in `searching` state.
- `MarkGrabbed(ctx, workflowID, clientID, downloadID, releaseName)` — transition to `grabbed`.
- `MarkCompleted(ctx, workflowID, message)` — terminal success.
- `MarkFailed(ctx, workflowID, reason)` — fail with auto-retry logic.
- `Cancel(ctx, workflowID)` — user-initiated cancellation.
- `Retry(ctx, workflowID)` — manual retry (resets attempt counter).

The engine implements `MediaStatusUpdater` integration: on grab it sets
the movie to "downloading"; on final failure it resets to "missing".

### Scheduler (`scheduler.go`)

Background goroutine (`RunLoop`) that periodically:

1. **Detects stale workflows** — grabbed/downloading workflows older than
   `StaleTimeout` (default 4h) are transitioned to `failed`.
2. **Processes retries** — failed workflows with remaining attempts get
   retried after exponential backoff (base 5m, max 1h).
3. **Prunes completed** — workflows in terminal states older than
   `PruneAfter` (default 7d) are deleted.

Loop interval is configurable (default 60s).

### API (`api.go`)

REST endpoints mounted at `/api/v1/workflows`:

| Method | Path                          | Description              |
| ------ | ----------------------------- | ------------------------ |
| GET    | `/api/v1/workflows`           | List recent (limit 50).  |
| GET    | `/api/v1/workflows/{id}`      | Get single workflow.     |
| POST   | `/api/v1/workflows/{id}/cancel` | Cancel a workflow.     |
| POST   | `/api/v1/workflows/{id}/retry`  | Manual retry.          |
| DELETE | `/api/v1/workflows/{id}`      | Delete a workflow.       |

## Integration Points

The workflow engine is consumed by:

- **Autosearch** — checks `FindActiveForMedia` before searching (skip if
  active workflow exists); creates workflows on successful grab.
- **Import pipeline** — matches completed downloads to workflows via
  `FindByDownload`; marks completed or failed.
- **Download service** — records manual grabs (interactive "add download"
  from the UI) as workflows.
- **Movies/Series handlers** — uses `ActiveMediaIDs` to show grab
  indicators (spinning icon) in the UI.
- **Server** — mounts workflow API routes; wires engine to all consumers.

## Configuration

The workflow engine uses sensible defaults and does not require explicit
configuration. Advanced tuning is available via the scheduler constants:

| Parameter     | Default | Description                               |
| ------------- | ------- | ----------------------------------------- |
| MaxRetries    | 3       | Auto-retry attempts before giving up.     |
| StaleTimeout  | 4h      | Time before a grabbed workflow is stale.  |
| RetryBaseWait | 5m      | Base for exponential backoff.             |
| RetryMaxWait  | 1h      | Cap on retry backoff.                     |
| PruneAfter    | 7d      | Time to keep completed/cancelled records. |
| LoopInterval  | 60s     | Scheduler tick interval.                  |

## Self-Healing Behaviour

### Download import safeguards

Downloads have one import owner: the orchestrator. Completed downloads without
an existing workflow get a tracked import workflow with the same recovery
behavior. The download event subscriber does not import the same files in
parallel or bypass settling and seeding requirements.

Before import, Loom pauses the download and caches the client's paths. It
removes the download from the client queue only after successful import,
without deleting downloaded data. Move imports stage a hardlink (or a copy
across filesystems), verify the destination, register it in the library, and
persist successful import history before removing the source media file.
For download-triggered moves, source removal is deferred to post-import cleanup;
library destinations are persisted in workflow metadata so interrupted imports
and cleanup can resume after a restart.
Verification or library-registration failures retain the source for retry.

A batch is not complete if any non-sample media file fails. Review-required
downloads remain failed/pending review rather than triggering successful
cleanup. Copy and hardlink retries recognize previously imported files only
when the destination still exists and matches the source; equal sizes alone
are not treated as proof of identity.

Missing paths, unresolved download metadata, and transient filesystem/database
errors retry the existing import with the configured retry limit, rather than
immediately starting another download. Failed imports retain their errors and
cached paths; manual retry reuses those paths even if the client item has since
been removed. Safety checks and explicit destination-conflict handling still
apply: recovery never means silently overwriting another file.

### Download lifecycle

The orchestrator never leaves a title stuck on "Downloading" and never
discards a download it simply can't see:

1. **Stalled or failed downloads.** The download monitor reports abandoned
   downloads via `NotifyDownloadFailed`. The workflow fails terminally
   (`failed_from` is recorded in metadata), the media status is reset, and
   a replacement search is triggered through the `ResearchFunc` hook. The
   stalled release is blocklisted, so the search picks a different one.
2. **Download vanished from the client.** If a grabbed/downloading item is
   missing from a *reachable* client for `MissingDownloadGrace` (15m), it is
   imported if it was last seen complete, otherwise abandoned as above.
3. **Client unreachable.** `ActiveDownloads` returns a
   `PartialDownloadStatusError` listing unreachable clients. Stale checks,
   boot reconciliation, missing-download checks and manual retry all leave
   downloads on those clients untouched until the client is back.
4. **Late completion.** If an abandoned download finishes anyway (e.g. the
   pause stall action), the failed workflow resumes into post-download —
   unless another workflow has already taken over the media.
5. **Failures in searching/grabbed/downloading are terminal.** Re-opening the
   workflow in `searching` would block autosearch while nothing services it;
   instead the media is released and re-searched. Import failures still retry
   the import; unrecoverable import errors fail immediately.
6. **Orphaned status.** Movies left `downloading` with no active workflow are
   reset to `missing` at boot and hourly. A failed/cancelled *upgrade* never
   downgrades an already-available movie.
7. Users can manually cancel stuck workflows via the API/UI, or retry. A
   retry that needs a new release triggers a fresh search instead of
   re-opening the failed workflow.
