# Request history and current availability

A request's `available` status records **fulfillment history**. The web portal
labels it **Fulfilled** and bots report it as historical. Deleting a media file,
its import row or its catalog entry does not reset that status, remove the
requester or change `fulfilled_at`. Migration 0080 backfills existing fulfilled
requests from their previous `updated_at`, not an invented historical import time.

Current observations are separate:

| Field | Meaning |
|---|---|
| `availability: present` | All currently due requested files physically verified on connected storage. |
| `availability: missing` | A previously physically verified requested file is now absent, and storage identity still matches. For series, this can mean partial availability. |
| `availability: unknown` | Storage, file scope, metadata or observation freshness cannot establish current presence. This is not confirmed deletion. |
| `availability_checked_at` | Observation time. Snapshots older than five minutes read as unknown. |
| `monitored` | Current linked catalog monitoring; omitted if the catalog entry no longer exists. |
| `media_server_availability` | Separate server observation. Currently always unknown; local files do not prove server playback. |

Movies need a live file. Series need files for all currently aired monitored
episodes in monitored seasons. Artists need all monitored tracks on selected
releases of currently released monitored albums. Future episodes/albums do not
block fulfillment; unknown dates are considered due. Missing track metadata or
incomplete scan records block fulfillment, but alone do not prove deletion.
Changing monitoring can change the current scope; it never resets past fulfillment.

## Re-request missing media

An owner or administrator can use **Re-request** on fulfilled, confirmed-missing
history. The API is the existing create endpoint with an explicit history link:

```json
{
  "media_type": "movie",
  "tmdb_id": "603",
  "title": "The Matrix",
  "re_request_of": "original-request-id"
}
```

The media identity must match the original. Loom rechecks storage before
admission and again before approval. Present or unknown media cannot authorize
re-acquisition. Recovery before approval links the now-present media without
starting an unnecessary replacement download.

Each submission creates a **new pending request** and preserves the old one.
Only one active (`pending`, `approving` or `approved`) request can exist globally
per media identity. A duplicate returns 409 even for an administrator. The new
request consumes its own rolling quota slot; the original fulfilled request
still counts in its original window. Quota exhaustion returns 429. Administrators
may bypass quotas and re-request another user's history, but cannot bypass
storage verification or active duplicates.

Approval reuses an existing catalog entry, resumes its top-level monitoring and
starts acquisition under that entry's existing profile/library. Only verified
missing episodes or albums containing verified missing tracks are searched;
present episodes/albums are not reacquired. Episode/album monitoring scope is
retained. If the catalog entry was deleted, normal trusted
metadata lookup and the administrator's selected target are used to add it again.
No old request is reopened. Explicit administrator **Clear history** remains a
separate destructive operation; routine deletion/reconciliation never calls it.

## Safe storage observations

On its first applied observation with a physically verified imported file,
Loom registers a random `.loom-request-storage` marker in the library root and
stores the matching token/path in SQLite/Postgres. The process needs write access
to register that marker once, and read access thereafter. Retain the marker in
backups and alongside the library; it is not a media file or a credential.

A missing, unreadable or mismatched marker, inaccessible root or changed root
identity produces **unknown**, never bulk missing/deletion. Loom does not
recreate a lost marker automatically. An empty mountpoint left after an unmount
therefore cannot masquerade as a complete scan. Imported paths outside the root,
unresolved symlinks and nested media volumes also remain unknown. Configure each
separately mounted volume as its own library root.

File evidence is retained for requests after file/catalog rows disappear.
Only paths previously physically verified under that root can prove absence.
A migrated request with already-deleted files stays unknown until it has
verified file evidence; stale database records are not enough to establish
deletion. Unknown state may require operator inspection rather than permitting
an automatic re-request.

For mount recovery, restore the original mounted volume and marker, then run
reconciliation. Do not create a replacement marker in an empty mountpoint.
If a library is deliberately relocated, inspect the new volume and known files
first, back up the database and marker, and stop Loom before repairing its
`request_storage_anchors` pairing. There is intentionally no blind "trust empty
storage" API. An unregistered existing marker is reported as an error requiring
administrator inspection rather than adopted silently.

## Preview, apply and audit

Administrators can preview/apply from **Requests → Availability reconciliation**
or call:

```http
POST /api/v1/requests/reconcile
Content-Type: application/json

{"dry_run": true}
```

Use `false` to apply a fresh observation. A preview is not a reusable approval
token: apply always rechecks the filesystem. Both return an audit ID, actor,
timestamp and per-request `before`, `after`, `reason`, `changed` and optional
`error`. A preview writes only its audit; it never changes requests, anchors or
files. An initial preview can show presence verified from a live file without
writing the marker that apply would register.

Applied batches atomically commit request observations and their audit. They
never delete media, import records, files, directories or request history.
Repeated observations preserve `fulfilled_at` and semantic `updated_at`, while
refreshing `availability_checked_at`. Per-item observation errors produce
unknown with an explicit error outcome; a batch persistence failure rolls back
request changes. Initial storage-marker registration is external to that database
transaction; if registration is interrupted, the unmatched marker requires
inspection rather than automatic adoption.

`GET /api/v1/requests/reconciliation` returns the latest 50 batch audits.
Reconciliation also runs on startup, after approval and every minute. Automatic
runs audit changed/error outcomes; unchanged automatic refreshes do not append
redundant audits. No media-server health or quality-cutoff guarantee is implied.
