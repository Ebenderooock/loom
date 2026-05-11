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
| `failed`     | An error occurred; will auto-retry if attempts remain.|
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

The workflow engine's self-healing eliminates the "stuck grabbed" problem:

1. Movie/episode grabs that exceed `StaleTimeout` without progressing are
   automatically marked `failed`.
2. On failure, the engine checks if retries remain. If yes, it schedules
   a retry with exponential backoff.
3. On final failure (retries exhausted), the media status is reset to
   `missing`, allowing the auto-search scheduler to pick it up fresh on
   its next cycle.
4. Users can manually cancel stuck workflows via the API/UI, or force an
   immediate retry.
