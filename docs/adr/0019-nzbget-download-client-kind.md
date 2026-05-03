# ADR-0019: NZBGet download client kind (Phase 3f)

- Status: Accepted
- Date: 2025-02-13
- Deciders: @loom-maintainers

## Context

Phase 3a shipped the `internal/downloads/` abstraction (ADR-0014).
Phase 3e added SABnzbd (ADR-0016) as the first Usenet driver. NZBGet
is the second-most-common open-source Usenet downloader — typically
chosen over SABnzbd by operators who care about resource footprint
(it is a single C++ binary vs SAB's Python+Cherrypy stack) or who
run on NAS appliances where SAB is awkward to package. Phase 3f
ships a first-class NZBGet kind so Loom can drive both Usenet stacks
without relying on the SAB-compatibility shim NZBGet historically
shipped.

NZBGet exposes two RPC surfaces: an XML-RPC endpoint at `/xmlrpc`
and a JSON-RPC endpoint at `/jsonrpc`. The two are functionally
equivalent — same method names, same parameter shapes, same return
types. NZBGet's own documentation prefers JSON-RPC for new
integrations.

## Decision

Implement `internal/downloads/nzbget/` as the registered factory
for `downloads.KindNZBGet`. The package is self-registering via
`init()` and exposes nothing outside the `Client` constructor and
the `Config` shape used by the OpenAPI generator.

### API version targeted

NZBGet **21 and newer**. The 21.0 release (2020) stabilised the
`append` method's positional signature and added the `pp_<name>=<value>`
convention used by the rest of the ecosystem (Sonarr, Radarr).
Pre-21 builds are likely to work for everything except `Add()`
edge cases (post-process scripts) but are not supported.

### Transport: JSON-RPC over HTTP

NZBGet exposes both XML-RPC and JSON-RPC at fixed paths
(`/xmlrpc`, `/jsonrpc`). We chose JSON-RPC because:

1. The Go standard library already speaks JSON; XML-RPC requires a
   third-party encoder/decoder pair we would otherwise pull in for
   nothing.
2. JSON-RPC envelopes are smaller and trivially diffable in test
   fixtures.
3. NZBGet's own documentation now leads with JSON-RPC; XML-RPC is
   maintained for compatibility but is no longer the recommended
   integration path.

The endpoint URL is composed as
`<scheme>://<host>:<port><base_path>/jsonrpc`. `base_path` defaults
to `/` so a vanilla NZBGet hits `http://host:6789/jsonrpc`; under a
reverse-proxy subpath the operator sets `base_path: /nzbget` and the
kind builds `https://host/nzbget/jsonrpc`.

### Authentication: HTTP Basic on every call

Unlike SABnzbd's apikey, NZBGet has no per-token auth — only
`ControlUsername` / `ControlPassword`, exposed exclusively as HTTP
Basic Auth. There is no login flow or session cookie to negotiate.

We pre-stamp `Authorization: Basic ...` on every outgoing request
rather than waiting for the 401 challenge. This avoids a doubled
round trip for every RPC call and matches the behaviour Sonarr and
Radarr ship.

### State-mapping table

NZBGet reports state across two endpoints (`listgroups` for active
items, `history(false)` for completed/failed items) using a
17-string vocabulary. The mapping lives in two switches in
`status.go`: `mapQueueStatus` and `mapHistoryStatus`.

| NZBGet string                                                                                                    | `downloads.ItemStatus`        |
| ---------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| `QUEUED`                                                                                                         | `queued`                      |
| `PAUSED`                                                                                                         | `paused`                      |
| `DOWNLOADING`, `FETCHING`                                                                                        | `downloading`                 |
| `PP_QUEUED`, `LOADING_PARS`, `VERIFYING_SOURCES`, `REPAIRING`, `UNPACKING`, `MOVING`, `EXECUTING_SCRIPT`, `COPYING`, `RENAMING`, `VERIFYING_REPAIRED` | `downloading`                 |
| `PP_FINISHED`                                                                                                    | `completed`                   |
| `DELETED`                                                                                                        | `failed`                      |
| `SUCCESS` (history)                                                                                              | `completed`                   |
| `WARNING` (history)                                                                                              | `completed`                   |
| `FAILURE`, `HEALTH` (history)                                                                                    | `failed`                      |
| _anything else_                                                                                                  | `unknown`                     |

Post-processing states (`UNPACKING`, `REPAIRING`, ...) are reported
as `downloading` for the same reason SABnzbd's are: the job is
still active and the abstraction's downstream consumers (the queue
grabber) treat anything not-`completed` as work-in-progress.

`WARNING` maps to `completed` because in NZBGet terminology a
warning means "downloaded successfully, but at least one
post-processing step (e.g. par2 repair) emitted a non-fatal
message". The bytes are on disk; the import logic should pick them
up.

### Queue + history merge

`Status()` issues `listgroups(0)` and `history(false)` and merges
both into the projected item list. The pair is consistent —
NZBGet moves an NZBID from the queue to the history exactly once,
atomically — so dedup is unnecessary. When the caller filters by
ids and the union is empty, we return `ErrNotFound`.

### Pause/Resume: global vs per-id split

NZBGet exposes both a global pause (`pausedownload` /
`resumedownload`) and a per-group pause (`editqueue` with
`GroupPause` / `GroupResume`). The kind dispatches on the id list:
empty → global; non-empty → per-id. This matches the abstraction
contract (`Pause(ctx)` pauses everything, `Pause(ctx, ids...)`
pauses a subset) without leaking NZBGet vocabulary.

### Remove: GroupDelete vs GroupFinalDelete

`Remove(ids, deleteFiles=false)` issues `editqueue("GroupDelete", ...)`
which removes the items from the queue but preserves history rows
and downloaded bytes on disk. `Remove(ids, deleteFiles=true)` issues
`editqueue("GroupFinalDelete", ...)` which purges history and
removes the staged files.

The split exists because NZBGet itself separates the operations.
Loom does not silently combine them so an operator who explicitly
asks for `deleteFiles=false` is guaranteed history + on-disk bytes
survive.

### Categories: parsed from server config

NZBGet has no dedicated `getcategories` RPC; categories are
defined in the main server config under `Category{N}.Name` /
`Category{N}.DestDir`. `Categories()` issues `config()`, walks the
keyspace, and projects each populated `Category{N}` slot. The
result is cached in-process for 30 seconds (keyed on the `*Client`
pointer) so a hot path that calls `Categories()` per add does not
hit the RPC every time. Cache misses on transient errors fall back
to the prior cached value to ride out flaps.

### Free space

`status()` returns `FreeDiskSpaceMB` as an integer. We multiply by
`1024 * 1024` to project bytes (NZBGet uses binary MB, unlike
SABnzbd which uses string-formatted decimal GB). When the field is
absent we return `-1` per the abstraction contract.

## Consequences

- The kind is self-contained in `internal/downloads/nzbget/`. The
  core `internal/downloads/` package gains zero new types or
  helpers.
- Operators can configure NZBGet via the standard
  `/api/v1/download-clients` POST surface, with a `config` blob
  documented in OpenAPI as `NzbgetConfig`.
- Per-client proxy and rate-limit policies apply automatically via
  `downloads.TransportForDefinition`, identical to the indexer and
  sibling-download kinds.

## Deferred

- **XML-RPC fallback.** The package targets JSON-RPC only.
  Operators on patched-down NZBGet builds (rare) that have JSON
  disabled will need to enable it; we will not maintain two
  transports.
- **Server-side category writes.** Categories are read-only. NZBGet
  itself rewrites `nzbget.conf` when categories change, which
  requires a server reload — a Phase 4+ concern.
- **History retention controls.** NZBGet's
  `HistoryRetention*` settings are not exposed; the operator
  manages them in the NZBGet UI.
- **Speed limits.** NZBGet's `rate(N)` RPC is not wired. Loom's
  indexer-level throttle layer covers most use cases; per-client
  speed control belongs to a future cross-kind PR.
- **HTTPS with self-signed certificates.** Same posture as
  ADR-0016 — operators configure system trust; per-client TLS
  skip is cross-cutting work.
