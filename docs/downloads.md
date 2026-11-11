# Download clients

Loom Phase 3a introduces a pluggable download-client abstraction that
mirrors the indexer subsystem. Every download client — qBittorrent,
Transmission, Deluge, SABnzbd, NZBGet, the built-in `builtin/torrent`
Rain-backed driver, and the built-in `builtin/null` no-op driver — is
registered into a single in-process registry, persists config and
last-known health to the database, and is reachable through the HTTP API
at `/api/v1/download-clients`.

This document is the operator-facing guide. For the design rationale and
the deviations from the original phase brief, see
[ADR-0014](adr/0014-download-clients-abstraction.md).

## API surface

| Method | Path                                                    | Description                              |
| ------ | ------------------------------------------------------- | ---------------------------------------- |
| GET    | `/api/v1/download-clients/`                             | List all clients with last-known health. |
| POST   | `/api/v1/download-clients/`                             | Create a download client.                |
| GET    | `/api/v1/download-clients/{id}`                         | Fetch one client + health.               |
| PUT    | `/api/v1/download-clients/{id}`                         | Replace a client.                        |
| PATCH  | `/api/v1/download-clients/{id}`                         | Update a subset of fields.               |
| DELETE | `/api/v1/download-clients/{id}`                         | Delete a client.                         |
| POST   | `/api/v1/download-clients/{id}/test`                    | Probe and persist health.                |
| GET    | `/api/v1/download-clients/{id}/categories`              | List categories advertised.              |
| GET    | `/api/v1/download-clients/{id}/free-space`              | Bytes free; `-1` if unknown.             |
| GET    | `/api/v1/download-clients/{id}/items`                   | List active items (`?ids=a,b` to limit). |
| POST   | `/api/v1/download-clients/{id}/items`                   | Submit a magnet/torrent/NZB.             |
| POST   | `/api/v1/download-clients/{id}/pause`                   | Pause items (empty body = all).          |
| POST   | `/api/v1/download-clients/{id}/resume`                  | Resume items (empty body = all).         |

Creating, updating, deleting, or testing a download-client configuration
requires an administrator role. List/detail responses omit usernames,
passwords, and credential-like values inside `config`; submitting blank or
omitting those fields during an edit keeps the stored credentials unchanged.
Download-use routes such as listing items and submitting a grab remain
available to authenticated users.
Edit-form tests include the saved client `id` so blank/masked credentials
can be restored for the test without persisting unsaved changes.

Movie and episode imports may replace a file registered to the same matched
item when its quality is equal or better and the assigned quality profile
accepts the incoming quality. Downgrades and disallowed qualities are rejected;
manual matching and reimport do not bypass these checks. If a profile or the
incoming quality cannot be resolved, an assigned profile cannot be verified
and the import is rejected. Without a profile, comparisons use available
filename quality information; an unknown existing quality cannot establish a
downgrade. Unrelated or unsafe destinations are not replaced. A recycle bin
is optional: when enabled, the old file is kept there.

File copies are written to hidden `.loom-import-*.partial` staging files in the
destination directory, synced and closed, then published without overwriting
collisions. For a replacement, Loom stages and verifies the incoming file
before preserving the old file in a private `.loom-replacement-*` backup and
replacing the live pathname in one rename. The old pathname remains available
while staging and backing up; a crash does not leave a gap between removing it
and publishing the new file. Backups use hardlinks where possible and otherwise
require a verified copy. Registration failures restore the old file where
possible; if restoration or recycling fails, the error identifies the preserved
backup. A crash can leave an orphaned backup in that directory, but retries
can verify the live file and finish registration. Filesystems lacking safe
no-replace publication report an error rather than blindly overwriting.
If a complete copy was published before history was recorded, retries verify
its content and finish registration. Unregistered mismatched destinations,
including possible partial files from older versions, are preserved for
inspection instead of being assumed to belong to the matched item.

### Importing completed files

In **Downloads > Imports**, scan the completed-download folder, select several
files, then choose **Import selected**, or use **Import all matched** to queue
every eligible match across all result pages. Search and pagination keep large
season-pack scans manageable. Historical imports are hidden by default;
enable **Show already imported files** to inspect them.

Scans run in the background so slow disks and large folders do not hold a
request open until a proxy returns 524.
The page shows discovery and matching progress and displays results as they
become available; you can queue those results before the scan finishes or cancel
the scan. Scans never hash full media payloads. **Previously imported** means
the current source and destination size/modification times match the snapshots
saved after a successful import; it is a historical hint, not content proof.
Hardlinks can be identified as the same underlying file without reading bytes.
Changed or missing files are shown again. Older history without destination
snapshots is not assumed to be a metadata match. Re-queueing those files performs
full verification before skipping identical content and saves snapshots for
subsequent fast scans; it does not blindly copy them again.

Enable **Show already imported files** to explicitly queue a historical copy
for verification. Normal **Import all matched** does not queue hidden historical
matches. Before skipping an actual import or deleting its source, Loom still
requires same-file identity or a fresh full SHA-256 comparison of both files.
Size/mtime matches and previous hash results alone never authorize deletion.

At most two scans run at once, and repeated submissions for the same running
folder reuse that scan. Unlike the durable import queue, scan previews are
temporary: they do not survive a restart, expire ten minutes after completion,
and only the eight most recent sessions are retained. An expired or missing
scan is reported explicitly; start a new scan to refresh its results.

The import queue is persistent, not a two-file admission limit. Workers process
queued files with bounded concurrency, and interrupted work resumes after a
restart. The queue shows file counts, attempts, failures, and scheduled retry
times. Expand **Show file outcomes** on a folder import to see exactly which
episodes succeeded or failed and their destinations/errors. Fix a terminal
failure's cause before choosing **Retry**. For safety-held imports, approve the
item in **Activity > Reviews**, then choose **Retry after approval** in the
import queue. Neither automatic retries nor re-submitting the path bypasses
pending or rejected safety reviews.

An unavailable source path during validation is an operational failure with
bounded retries, not a suspicious-content review. Restore the download path or
storage mount before retrying; a missing file is never treated as successfully
imported merely because a destination filename exists.

Success is tracked per source file, with destination verification, not per
torrent. A partially imported season can be retried without repeating its
successful episodes. Imports register each file with the movie or episode in
the library; the UI refreshes library status as queued imports finish.

Migration 0103 automatically consolidates old duplicate episode-file registrations
on startup, keeping the newest record for each episode/path. It does not delete
media files or collapse distinct file paths or separate episodes sharing a file.
New registrations update the existing record and episode availability together,
so repeated or concurrent imports cannot recreate those duplicates.

| Method | Path | Description |
| ------ | ---- | ----------- |
| POST | `/api/v1/imports/scans` | Start a background preview with `{"path":"/downloads"}`; returns 202 immediately. |
| GET | `/api/v1/imports/scans/{id}?offset=0&limit=1000` | Poll progress and read the next page of results; continue from `next_offset`. |
| DELETE | `/api/v1/imports/scans/{id}` | Cancel a preview without cancelling imports already queued from its results. |
| POST | `/api/v1/imports/batch` | Queue selected files with `{"paths":["/downloads/file.mkv"]}`. |
| GET | `/api/v1/imports/jobs` | List active jobs and recent outcomes, including progress and retry deadlines. |
| POST | `/api/v1/imports/jobs/{id}/retry` | Retry a failed job after correcting its cause. |

Existing `/api/v1/imports/manual`, `/manual-match`, and `/reimport` actions use
the same queue. An accepted response means queued, not imported successfully.
Manual matching imports media files individually, with their subtitles and
extras; it does not move unrelated loose NFO or image files with the folder.

The full download-client schema is in [`api/openapi/loom.yaml`](../api/openapi/loom.yaml)
under the `download-clients` tag.

## Downloads / Imports

**Scan & Import** previews video and audio files recursively without changing
them. Music matches use embedded artist, album and track tags against albums
already added to Loom, including standalone albums and compilations. Ambiguous
matches are not imported automatically. **Manual Match → Music** lets you
select an existing album and its specific track when automatic matching fails.
Add the album to Music first if it is not already in your library.

Imports transfer the physical media, not just its database association:

| Media | Destination |
| --- | --- |
| Movie | Assigned library / movie title and year |
| Episode | Assigned series library / series / season |
| Music | Owner's library (or standalone album's library) / artist / album and year / numbered track |

Multi-artist albums and compilations use the `Various Artists` directory.
The album's selected edition determines its track list and numbering.
An unrelated same-title album is never selected solely by track position.

`media_management.import_mode` controls source handling. The default `move`
mode transfers media into the library and removes the source. Automatically
matched imports and re-imports verify and register their staged destinations
before removing source files. `copy`, `hardlink`, and `hardlink_only`
deliberately retain the source; hardlink mode falls back to copying across
filesystems, while hardlink-only mode reports an error. Download-triggered
moves retain sources until the workflow reaches its durable cleanup stage.
Automatically unmatched files remain at the source, and partial failures are
reported rather than counted as a completed import. If an explicitly matched
video folder was already moved before registration failed, its files remain
at the destination for recovery.

**Import queued** means the server accepted background work, not that files
have already moved. Check the workflow and import history for the completed
destination or failure. Scanning alone never transfers files.

## Acquisition failure history and recovery

Activity → History includes acquisitions that fail before a download or
workflow exists. Each admitted attempt records media/release identity, indexer,
stage, timestamp, a sanitized reason and retry eligibility. Raw provider/client
errors, signed URLs, credentials and response bodies are not stored as reasons.

Manual grabs, automated movie/episode/music grabs and event-driven downloads
share the same admission gate. A release is identified by its torrent infohash
(including magnet hashes), or indexer plus normalized release title when no hash
is available. Changing a signed URL does not reset the retry budget.

Transient provider fetch and client failures get at most **three total automatic
attempts**, using workflow backoff (five minutes, then fifteen minutes). The next
normal search or grab can retry only after the persisted deadline; there is no
separate replay worker storing stale authenticated URLs. Invalid releases with
no payload stop immediately. Unknown client rejections are conservatively
treated as transient rather than permanently blocklisting a potentially good
release. Outages never add automatic blocklist entries.

Successful and in-progress admissions reject duplicate attempts. If Loom exits
after admission but before recording the outcome, the attempt stays pending:
check the download client's queue before authorizing another attempt. This
avoids blindly duplicating a request whose client outcome is unknown. Client
cancellations/timeouts also remain pending. Recovery actions on pending attempts
are refused for the first fifteen minutes to avoid racing an in-flight add.

Administrators can **Enable retry** (reset the budget, then search again with
fresh links) or **Blocklist** a failed/interrupted acquisition. Retry does not
remove an existing blocklist entry; remove that entry in Activity → Blocklist
first. Older superseded attempts cannot change current eligibility.
Clearing completion history retains acquisition records and budgets.

Automated movie/episode/music provider searches also persist failure budgets,
keyed by media, indexer and query, even before any release exists. Healthy
providers continue when another provider is deferred. Skipped queries preserve
the previous failure budget; a successful query resets it. Provider search
failures can be retried explicitly, but cannot be release-blocklisted because
they do not identify a bad release.

The existing search diagnostics, provider circuit breakers/API budgets and
post-download workflow retries remain separate. This is not a replacement for
individual HTTP transport retries. Interactive browsing searches retain their
existing behavior; acquisition from their results still uses the shared gate.

## Configuration

Download-client behaviour is governed by a top-level `downloads:` block
in the Loom config:

```yaml
downloads:
  operation_timeout: 15        # seconds; per-client request timeout
  max_parallel: 8              # ceiling on concurrent fan-out probes
  health_check_schedule: "*/5 * * * *"
  health_check_timeout: 10     # seconds; per-client health probe timeout
```

All four keys have sane defaults. The health check job is registered as
`downloads.health` and runs through Loom's standard cron scheduler.

## Built-in `null` client

The `builtin/null` kind is always registered. It accepts any input,
reports `protocol: torrent`, and returns empty `Status`/`Categories`
results plus `FreeSpace = -1`. It exists so that fresh installs and
test environments can exercise CRUD and health endpoints without
configuring a real client. Most operators will never need it in
production.

```bash
curl -sS -H "X-Api-Key: $LOOM_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"id":"null","name":"Null","kind":"builtin/null","protocol":"torrent","enabled":true}' \
     http://localhost:7878/api/v1/download-clients/
```

## Built-in torrent client (`builtin/torrent`)

`builtin/torrent` now uses a local
[Rain](https://github.com/cenkalti/rain) sidecar over JSON-RPC. Loom
owns auth and orchestration; Rain stays internal-only in the same pod.

### Feature parity

| Feature                        | Supported |
| ------------------------------ | --------- |
| Magnet URIs                    | ✅        |
| Raw `.torrent` bytes           | ✅        |
| Torrent URL (HTTP)             | ✅        |
| Pause / Resume (per item)      | ✅        |
| Pause / Resume all             | ✅        |
| Remove                         | ✅        |
| Data verify (recheck)          | ✅        |
| DHT re-announce                | ✅        |
| Global download speed limit    | ✅        |
| Global upload speed limit      | ✅        |
| Per-item speed limits          | ⏳ planned |
| Queue priority (top/bottom)    | ⏳ planned |
| Force-start                    | ⏳ planned |
| Seeding lifecycle detection    | ✅        |
| `FreeSpace` reporting          | ✅        |
| Per-item detail (peers/files)  | ✅        |

### Configuration

The `config` JSON blob accepts the following keys:

| Key                    | Type   | Default         | Description                                          |
| ---------------------- | ------ | --------------- | ---------------------------------------------------- |
| `download_dir`         | string | **required**    | Where Rain writes downloaded files.                  |
| `rpc_host`             | string | `127.0.0.1`     | Rain RPC host.                                       |
| `rpc_port`             | int    | `7246`          | Rain RPC port.                                       |
| `rpc_url`              | string | derived          | Optional full Rain RPC URL override.                |
| `request_timeout_secs` | int    | `10`            | RPC timeout per call.                                |
| `enable_dht`           | bool   | `true`          | Reflected in engine summary metadata.                |
| `enable_pex`           | bool   | `true`          | Reflected in engine summary metadata.                |
| `download_speed_limit` | int64  | `0`             | UI-stored global cap intent (bytes/s).               |
| `upload_speed_limit`   | int64  | `0`             | UI-stored global cap intent (bytes/s).               |

### Example: create via API

```bash
curl -sS -X POST \
     -H "X-Api-Key: $LOOM_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{
           "kind": "builtin/torrent",
           "name": "Built-in Torrent",
           "protocol": "torrent",
           "enabled": true,
           "config": {
             "download_dir": "/data/downloads",
             "rpc_host": "127.0.0.1",
             "rpc_port": 7246,
             "request_timeout_secs": 10,
             "enable_dht": true,
             "enable_pex": true,
             "download_speed_limit": 0,
             "upload_speed_limit": 0
           }
         }' \
     http://localhost:7878/api/v1/download-clients/
```

### Engine status endpoint

Once a `builtin/torrent` client is created you can query the live engine
summary:

```bash
GET /api/v1/download-clients/{id}/torrent/status
```

Returns aggregate counts (downloading, seeding, paused, queued) plus the
current aggregate transfer rates and configured speed limits.

### Speed limit endpoint

```bash
POST /api/v1/download-clients/{id}/torrent/speed-limits
Content-Type: application/json

{ "download_limit": 10485760, "upload_limit": 5242880 }
```

Sets rates immediately and persists them to the stored config so they
survive restarts. Pass `0` for unlimited.

### Pause/resume all torrents

```bash
POST /api/v1/download-clients/{id}/torrent/pause-all
POST /api/v1/download-clients/{id}/torrent/resume-all
```

### Implementation notes

- `builtin/torrent` is now a Rain RPC adapter; no in-process BitTorrent
  engine is embedded in Loom.
- For Kubernetes, deploy Rain as a sidecar and keep its RPC endpoint
  internal to the pod.
- `POST /torrent/speed-limits` persists UI-configured limits on the Loom
  client definition; Rain runtime changes are driven through Rain config.

## Health checking

Each `POST /test` call probes the client (`Test()`), and on success
opportunistically calls `Categories()` and `FreeSpace()` to enrich the
persisted health row. The scheduled `downloads.health` job runs the
same probe in the background for every enabled client, capping
parallelism at `downloads.max_parallel`. Health rows include:

- `status` — `unknown`, `ok`, `warning`, or `error`
- `last_checked_at`
- `last_error`
- `consecutive_failures`
- `last_free_space_bytes` (omitted when the client cannot report it)

## Authoring a new download-client driver

1. Create a package under `internal/downloads/<your-kind>/` (or under
   `internal/downloads/builtin/` for in-tree built-ins).
2. Implement [`downloads.DownloadClient`](../internal/downloads/types.go).
3. In an `init()` function, register a factory:

   ```go
   func init() {
       downloads.RegisterKind(KindFoo, factoryFn)
   }
   ```

4. Add the package to a blank import list (the same pattern indexers
   use) so `init()` runs.

The registry/service/health/HTTP layers do not change. Persistence is
handled by `internal/downloads/repository.go` for both SQLite and
Postgres via sqlc-generated row types.

## Storage

Migration `0010` adds two tables on both engines:

- `download_clients` — config, defaults, transport metadata.
- `download_client_health` — last probe outcome, with cached
  categories JSON and free-space bytes.

Credentials in the `config` JSON blob are stored at rest in plaintext,
matching the existing indexers/proxies convention. ADR-0014 captures
the rationale and the future plan to add at-rest encryption uniformly
across the three subsystems.

## SABnzbd

The `sabnzbd` kind ships in-tree as the first Usenet driver on this
abstraction. It speaks the SABnzbd 3.x+ JSON API, authenticates with
the operator's apikey, and maps SAB's queue and history vocabularies
onto Loom's `ItemStatus` enum. See
[`docs/downloads-sabnzbd.md`](downloads-sabnzbd.md) and
[ADR-0016](adr/0016-sabnzbd-download-client-kind.md) for the operator
guide and design rationale.

## Supported kinds

| Kind                  | Protocol | Status        | Docs                                                       |
| --------------------- | -------- | ------------- | ---------------------------------------------------------- |
| `builtin/null`        | torrent  | ✅ shipped    | (in-tree no-op; see [ADR-0014](adr/0014-download-clients-abstraction.md)) |
| `builtin/torrent`     | torrent  | ✅ shipped    | [Built-in torrent client](#built-in-torrent-client-builtintorrent) |
| `qbittorrent`         | torrent  | ✅ shipped    | [docs/downloads-qbittorrent.md](downloads-qbittorrent.md) |
| `transmission`        | torrent  | ⏳ planned    |                                                            |
| `deluge`              | torrent  | ⏳ planned    |                                                            |
| `sabnzbd`             | usenet   | ⏳ planned    |                                                            |
| `nzbget`              | usenet   | ✅ shipped    | [docs/downloads-nzbget.md](downloads-nzbget.md)            |

## Routing & Monitoring (Phase 3g)

Loom Phase 3g introduces the download routing and monitoring layer, which bridges the indexer intake pipeline and download clients.

### Router Service

The `Router` service subscribes to indexer result events and automatically queues high-quality results on configured download clients. It implements a simple quality filter (seeder-based) and attempts clients in priority order, falling back to any available client on failure.

**Quality filtering (Phase 3):**
- Reject torrents with 0 seeders
- Accept Usenet results (no seeders field)
- Accept torrents with >0 seeders

Full semantic filtering (resolution, codec, release groups, language) is deferred to Phase 5.

**Event emission:**
- `TopicDownloadQueued` — result successfully queued on a client
- `TopicDownloadFailed` — all clients unavailable or all failed to queue
- Events include origin result ID, client ID, download ID, and timestamp for traceability

### Monitor Service

The `Monitor` service periodically polls all registered download clients for status updates and emits completion events. It tracks completed items to avoid duplicate event emission across sweeps.

**Periodic invocation:** The Monitor is designed to be called by the scheduler (kernel phase integration pending). Each `Run()` call fans out Status queries across all clients and emits `TopicDownloadCompleted` events for newly-completed items.

**Event emission:**
- `TopicDownloadCompleted` — item transitioned to completed status in this sweep (not previously seen as completed)

### Event Topics

The download orchestration layer defines three new event bus topics:

- `downloads.queued` — result successfully routed to a download client
- `downloads.failed` — result failed to route (no clients or all clients failed)
- `downloads.completed` — download item completed

See [ADR-0020](adr/0020-download-routing-and-monitoring.md) for the full design rationale.
