# ADR-0016: SABnzbd download client kind (Phase 3e)

- Status: Accepted
- Date: 2025-02-13
- Deciders: @loom-maintainers

## Context

Phase 3a shipped the `internal/downloads/` abstraction (ADR-0014):
interface, registry, repository, service, health checker, HTTP
handlers. With the foundation in place, individual download-client
kinds land one phase per package. Phase 3e covers SABnzbd, the
incumbent free Usenet downloader and the most common pairing for the
Newznab/Torznab indexer kinds shipped in Phase 2.

SABnzbd's API is small and stable: a single
`/api?mode=...&output=json` endpoint authenticated by an `apikey`
query parameter. The challenge is mapping its idiosyncrasies onto
Loom's neutral `downloads.DownloadClient` shape — split queue and
history endpoints, MB-as-string sizes, GB-as-string disk space, and
a "status:false + error" envelope SAB returns with HTTP 200.

## Decision

Implement `internal/downloads/sabnzbd/` as the registered factory for
`downloads.KindSABnzbd`. The package is self-registering via `init()`
and exposes nothing outside the `Client` constructor and the
`Config` shape used by the OpenAPI generator.

### API version targeted

SABnzbd **3.x and newer**. SAB has not broken its JSON envelope since
the 3.0 rewrite (2020); the `mode=version` probe doubles as both the
authentication test and the implicit version check. Older 2.x builds
return XML by default and will fail at the JSON unmarshal — that is
acceptable; Loom does not target SAB before 3.0.

### Authentication: apikey query parameter

SABnzbd offers two auth schemes: `apikey` query parameter and HTTP
Basic Auth. We use **only** the apikey form because:

1. It is the documented primary scheme — Basic Auth exists for
   reverse-proxy compatibility, not as a first-class option.
2. The apikey is single-purpose and revocable from SAB's UI; an
   operator's HTTP Basic password often is not.
3. The same secret authenticates Sonarr/Radarr/etc.; matching their
   convention reduces operator surprise.

The key is sent on every request — both query (`apikey=...`) and POST
form bodies (`addurl`, `addfile`). The OpenAPI shape marks it
`writeOnly: true` so it is never echoed back on reads.

### State-mapping table

The mapping lives in a single switch in `status.go`. Two functions:
`mapQueueStatus` for `mode=queue` slots, `mapHistoryStatus` for
`mode=history` slots. Adding a new SAB state is a one-line review.

| SAB string                                         | `downloads.ItemStatus`        |
| -------------------------------------------------- | ----------------------------- |
| `Downloading`, `Fetching`, `Checking`              | `downloading`                 |
| `Extracting`, `Repairing`, `Verifying`, `Moving`   | `downloading`                 |
| `Running`                                          | `downloading`                 |
| `Paused`                                           | `paused`                      |
| `Queued`, `Grabbing`, `QuickCheck`                 | `queued`                      |
| `Completed`                                        | `completed`                   |
| `Failed`                                           | `failed`                      |
| _anything else_                                    | `unknown`                     |

Multi-stage post-processing states (`Extracting`, `Repairing`,
`Verifying`) are reported as `downloading` because, from Loom's
perspective, the job is still active — surfacing them as `completed`
prematurely would confuse the upcoming queue-aware grabber logic.

### Queue + history merge

`Status()` issues `mode=queue` and `mode=history` in series and
concatenates the projected items. Two endpoints, one round-trip
each, no pagination — SAB returns at most ~1000 history slots by
default, well within HTTP body sizes that don't need streaming.

The two responses never overlap (an `nzo_id` is in exactly one of
them at any moment), so dedup is unnecessary. When the caller
filters by ids and the union is empty, we return `ErrNotFound` so
upstream HTTP handlers can surface a 404 rather than an empty 200.

### Remove: try queue then history

`Remove()` first calls `mode=queue&name=delete` for each id. If SAB
reports the id was not in the queue, we fall back to
`mode=history&name=delete`, honouring `deleteFiles` via SAB's
`del_files=1` parameter. The split exists because SAB itself
distinguishes the two — there is no unified delete endpoint — and
we did not want callers to have to know which side an id is on.

### Free space: incomplete dir

SAB reports `diskspace1` (incomplete) and `diskspace2` (complete) as
decimal-gigabyte strings under `mode=fullstatus`. We pick
`diskspace1` because that is where SAB writes during a download —
the value that determines whether a new job will fit. `-1` is
returned when the field is absent, matching the
`downloads.DownloadClient.FreeSpace` contract.

### Categories: rich first, flat fallback

`mode=get_config&section=categories` returns the per-category
metadata (name, dir, script). Some SAB deployments lock that
endpoint behind "advanced" config; in that case we fall back to
`mode=get_cats`, which returns a flat list of names. The SAB
sentinel `*` is filtered out of both shapes — it is SAB's "use
default" marker, not a real category.

## Consequences

- The kind is self-contained in `internal/downloads/sabnzbd/`. The
  core `internal/downloads/` package gains zero new types or
  helpers.
- Operators can configure SABnzbd via the standard
  `/api/v1/download-clients` POST surface, with a `config` blob
  documented in OpenAPI as `SabnzbdConfig`.
- Per-client proxy and rate-limit policies apply automatically via
  `downloads.TransportForDefinition`, identical to the indexer
  kinds.

## Deferred

- **Scripts / categories management.** We surface SAB-configured
  scripts and categories read-only. Creating or editing them via
  Loom is a Phase 4+ concern; SAB's own UI remains the source of
  truth.
- **Server config.** SAB's per-server (news server) config is not
  exposed. Adding it would multiply the API surface for negligible
  operator benefit — operators set up news servers once.
- **Speed limits.** SAB exposes `mode=config&name=speedlimit`; we
  defer wiring this to Loom because the indexer subsystem already
  has its own throttle layer that covers most rate-control use
  cases.
- **HTTPS with self-signed certificates.** The kind uses the
  standard transport; operators with self-signed certs need to
  configure their system trust store. Per-client TLS skip is a
  cross-cutting concern that should land in a single PR across all
  kinds, not per-kind.
