# API

## Provider/account budgets

`GET /api/v1/indexers/provider-budgets` returns `{ "budgets": [...] }` with
configured allowance, consumed calls, local/provider reset Unix timestamps,
nullable provider remaining calls, and explicit member IDs.
`PUT /api/v1/indexers/provider-budgets/{budgetID}` atomically configures
`allowance`, `window_seconds`, and the complete `indexer_ids` list; success is
204, invalid configuration is 400, accounting unavailable is 503.
Both routes require an administrator when authentication is enabled.
They never accept consumed/reset fields or clear provider exhaustion.
See [provider API budgets](indexers-provider-budgets.md) for curl examples,
signal handling and manual/privileged behavior.

Budget-denied search diagnostics and streaming events use `status: "skipped"`,
a reason containing the UTC resume time (or unknown reset), and optional
`resume_at` Unix seconds. A skipped provider is not a circuit-breaker failure.

The canonical machine-readable specification of Loom's HTTP API lives in
[`api/openapi/loom.yaml`](../api/openapi/loom.yaml) (OpenAPI 3.1). This
page is the human summary.

## Default quality profiles

New movie and series additions, including Discovery, use the **Any** quality
profile when `quality_profile_id` (movie) or `qualityProfileId` (series) is omitted,
empty or whitespace. Explicit selections are preserved. The server resolves the
actual stored Any ID, preferring current profiles and falling back to an active
legacy profile; it does not select the first profile in the list. If Any is
unavailable, adding the title fails with an actionable error rather than saving
an unsearchable title. Existing titles are not silently reassigned: library cards,
rows and details identify missing profiles, which can be repaired using the
quality-profile editor.

Import lists no longer insert the literal `default` placeholder for an omitted
selection. Existing lists with that placeholder also resolve to Any when adding
a title, unless an active profile actually has the ID `default`. Movie and series
responses expose `qualityProfileMissing`, computed against both current and
active legacy profiles, so existing blank, deleted or dangling selections are
highlighted without incorrectly flagging valid legacy profiles.

## Versioning policy

| Path prefix | Owner | Stability |
|---|---|---|
| `/api/v1/*` | Loom-native | Active. Breaking changes only at major version bumps. |
| `/api/v3/*` | Wire-compat: Radarr v3 / Sonarr v3 + v4 | Lands in Phase 7. We track upstream shape; we do not extend it. |
| `/api/v1/*` (Prowlarr namespace) | Wire-compat: Prowlarr v1 | Lands in Phase 7. Disambiguated by the routes Prowlarr clients call. |

Wire-compat surfaces are explicitly _shape-compatible_, not
_semantically-extended_: clients that work against the upstream apps
should work against Loom unchanged.

## Routes (current)

These are the routes implemented today. They are the routes documented
in `api/openapi/loom.yaml`.

| Method | Path | Auth | Response | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/media-files/{mediaType}/{id}` | admin | `200 {"files":[TitleFile],"deletionMode":"recycle"\|"permanent"}` | List every regular file in the server-resolved movie or series folder; optional `season` query scopes series files. |
| `POST` | `/api/v1/media-files/{mediaType}/{id}/delete` | admin | `200 {"deleted":["relative/file.mkv"]}` | Delete selected files with explicit mode and `X-Confirm-Delete: true`; optional `season` query restricts scope. |
| `POST` | `/api/v1/autosearch/evaluate` | apiKey/bearer/cookie | `200 application/json` | Production-engine dry run with decision evidence; optional unsaved policy preview. |
| `POST` | `/api/v1/blocklist/check` | apiKey/bearer/cookie | `200 {"blocked":[bool]}` | Positional blocklist flags for up to 2000 candidate releases (`title`, `infohash`, `magnet`). |
| `GET` | `/healthz` | none | `200 application/json` | Liveness probe. |
| `GET` | `/livez` | none | `200 application/json` | Same shape; conventional alias. |
| `GET` | `/readyz` | none | `200` or `503 application/json` | DB ping + ready-flag check. |
| `GET` | `/metrics` | none | `200 text/plain; version=0.0.4` | Prometheus exposition. |
| `GET` | `/api/v1/system/status` | none (today) | `200 application/json` (`SystemStatus`) | Build metadata. |
| `GET` | `/api/v1/discover/{media}/{id}` | apiKey/bearer/cookie | `200 application/json` | Read-only full video metadata + credits; `media=movie\|series`, optional `provider=tmdb\|imdb\|tvdb`. Exact identity resolution only. |
| `GET` | `/api/v1/discover/artist/{id}` | apiKey/bearer/cookie | `200 application/json` | MusicBrainz artist metadata and albums, without adding. |
| `GET` | `/api/v1/discover/album/{id}` | apiKey/bearer/cookie | `200 application/json` | MusicBrainz album metadata and candidate editions. |
| `GET` | `/api/v1/discover/release/{id}` | apiKey/bearer/cookie | `200 application/json` | MusicBrainz edition metadata and full tracks. |
| `POST` | `/api/v1/import-lists/discover/track` | apiKey/bearer/cookie | `200 application/json` | Mark `{item_id}` added only after verifying library membership; never creates media or changes list settings. |
| `GET` | `/api/v1/libraries/{id}/health` | apiKey/bearer/cookie | `200 application/json` (`LibraryHealthReport` or `null`) | Fetch latest read-only health report snapshot. |
| `POST` | `/api/v1/libraries/{id}/health` | apiKey/bearer/cookie | `200 application/json` (`LibraryHealthReport`) | Run a bounded, cancellable read-only health check for one library. |
| `POST` | `/api/v1/libraries/{id}/health/dismiss` | apiKey/bearer/cookie | `200 application/json` (`LibraryHealthReport`) | Dismiss or restore one finding in the latest report. |
| `GET` | `/api/v1/discover/recommendations` | identified user | `200 application/json` | TMDB-backed movie/series acquisition suggestions, reasons and cache/source warnings. |
| `POST` | `/api/v1/discover/dismissals` | identified user | `204` | Persist a movie/series suggestion dismissal for the current user. |
| `GET` | `/api/v1/discover/collections` | identified user | `200 application/json` | TMDB movie sets represented in the catalog, with coverage counts. |
| `GET` | `/api/v1/discover/collections/{id}` | identified user | `200 application/json` | Release-ordered members with catalog/file/monitoring status; `404` for a missing collection. |
| `GET` | `/api/v1/collections/` | apiKey/bearer/cookie | `200 {"items":[LoomCollection]}` | List persistent Loom-owned collections using existing library access. |
| `POST` | `/api/v1/collections/` | admin | `200 LoomCollection` | Create a local collection or import an exact TMDB/IMDb/TVDB source. |
| `POST` | `/api/v1/collections/discover` | admin | `200 CollectionScanStatus` | Queue an optional background rescan; never calls providers in the request. |
| `GET` | `/api/v1/collections/scan` | apiKey/bearer/cookie | `200 CollectionScanStatus` | Read durable progress, pending counts and retryable discovery issues. |
| `GET` | `/api/v1/collections/{id}` | apiKey/bearer/cookie | `200 LoomCollection` | Read members and current library/file state; merged IDs resolve to the retained collection. |
| `PATCH` | `/api/v1/collections/{id}` | admin | `200 LoomCollection` | Edit collection metadata with an optimistic revision. |
| `POST` | `/api/v1/collections/{id}/sources` | admin | `200 LoomCollection` | Attach and refresh another exact provider source. |
| `POST` | `/api/v1/collections/{id}/refresh` | admin | `200 LoomCollection` | Reconcile attached sources while preserving local additions and exclusions. |
| `POST` | `/api/v1/collections/{id}/merge` | admin | `200 LoomCollection` | Explicitly merge collections with exact-ID deduplication and provenance. |
| `POST` | `/api/v1/collections/{id}/members` | admin | `200 LoomCollection` | Add a local movie/show member without acquisition or monitoring. |
| `PATCH` | `/api/v1/collections/{id}/members/{member}` | admin | `200 LoomCollection` | Exclude/restore a member or override its local title. |
| `GET\|POST` | `/debug/pprof/*` | gated by `debug.pprof` | varies | Standard `net/http/pprof` handlers. |
| `GET` | `/api/v1/indexers/` | apiKey/bearer/cookie | `200 application/json` (`[]DefinitionWithHealth`) | List configured indexers with health; credential fields are omitted. |
| `POST` | `/api/v1/indexers/` | admin | `201 application/json` (`Definition`) | Create an indexer. |
| `GET` | `/api/v1/indexers/{id}` | apiKey/bearer/cookie | `200 application/json` (`DefinitionWithHealth`) | Fetch one indexer + health; credential fields are omitted. |
| `PUT` | `/api/v1/indexers/{id}` | admin | `200 application/json` (`Definition`) | Replace an existing indexer's full definition. |
| `PATCH` | `/api/v1/indexers/{id}` | admin | `200 application/json` (`Definition`) | Partially update an existing indexer. |
| `DELETE` | `/api/v1/indexers/{id}` | admin | `204` | Remove an indexer. |
| `GET` | `/api/v1/indexers/{id}/caps` | apiKey/bearer/cookie | `200 application/json` (`Caps`) | Per-indexer capability descriptor. |
| `POST` | `/api/v1/indexers/{id}/test` | admin | `200 application/json` (`Health`) | Run an immediate health check; persists the result. |
| `POST` | `/api/v1/indexers/test` | admin | `200 application/json` (`TestResult`) | Test an unsaved indexer configuration. |
| `POST` | `/api/v1/indexers/search` | apiKey/bearer/cookie | `200 application/json` (`AggregatedResults`) | Fan-out search across all (or a subset of) enabled indexers. |
| `GET` | `/api/v1/proxies/` | apiKey/bearer/cookie | `200 application/json` (`{"proxies":[Proxy]}`) | List configured outbound proxies. |
| `POST` | `/api/v1/proxies/` | admin | `201 application/json` (`Proxy`) | Create a proxy (HTTP/HTTPS/SOCKS5/FlareSolverr). |
| `GET` | `/api/v1/proxies/{id}` | apiKey/bearer/cookie | `200 application/json` (`Proxy`) | Fetch one proxy. |
| `PUT` | `/api/v1/proxies/{id}` | admin | `200 application/json` (`Proxy`) | Replace an existing proxy's full definition. |
| `PATCH` | `/api/v1/proxies/{id}` | admin | `200 application/json` (`Proxy`) | Partially update a proxy. |
| `DELETE` | `/api/v1/proxies/{id}` | admin | `204` or `409 application/json` | Remove a proxy. Returns 409 `proxy_in_use` (with `details.indexer_ids`) when any indexer still references it. |
| `POST` | `/api/v1/proxies/{id}/test` | admin | `200 application/json` (`ProxyTestResult`) | Probe the proxy: HTTP/SOCKS5 fetch the configured probe URL; FlareSolverr issues `sessions.list`. |
| `POST` | `/api/v1/requests` | authenticated | `201 MediaRequest` | New request, or explicit owner/admin re-request with `re_request_of`. Active duplicates and unverified absence return 409; quotas return 429. |
| `GET` | `/api/v1/requests` | admin | `200 {"data":[MediaRequest]}` | All request history; optional `status` filter. |
| `GET` | `/api/v1/requests/mine` | authenticated | `200 {"data":[MediaRequest]}` | Own history with independent current file presence and monitoring. |
| `POST` | `/api/v1/requests/reconcile` | admin | `200 RequestReconciliation` | Required `{"dry_run":true}` previews; `false` applies. Both are audited with per-item outcomes. |
| `GET` | `/api/v1/requests/reconciliation` | admin | `200 {"data":[RequestReconciliation]}` | Latest 50 batch audits, including previews. |

Request `status: "available"` means **historically fulfilled**, never a guarantee
of current availability. `fulfilled_at` is preserved across file/catalog
deletion and recovery. `availability` is independently `present`, `missing` or
`unknown`, with a reason and observation time; observations older than five
minutes are returned as `unknown`. `monitored` describes the linked catalog entry.
`media_server_availability` remains `unknown` until a server-observation source
is implemented; local file presence does not imply Plex/Jellyfin/Emby playback.

Re-requests create new pending rows linked to their history, require freshly
verified missing files and retain normal rolling quota accounting. Administrators
bypass quotas, not duplicate or storage checks. Reconciliation never deletes
media/import records. See [the request lifecycle guide](requests.md) for storage
markers, conservative unknown states, and operator recovery.

## Title file management

On movie and show detail pages, administrators can open **Manage files**; seasons
have their own file manager. The list includes videos, samples, subtitles, `.nfo`
files and other extras. Each row includes a title-relative `path`, byte `size`,
`tracked` marker, and `quality` / `format` when known. Select files, choose delete,
and review the confirmation listing every selected path. Nothing is deleted until
the confirmation is accepted.

`mediaType` is `movie` or `series`. A series can be scoped with `?season=1`
(including `0` for specials). Season scope includes files tracked for that season,
all extras in its `Season N` / `SN` folder (or a custom folder whose tracked files
belong to only that season), and flat episode-named files that parse
as that season. Unassigned show-level extras remain available in the whole-show
manager. Directories, symlinks and special files are never selectable.

For example, after listing `/api/v1/media-files/movie/movie-id`, send:

```http
POST /api/v1/media-files/movie/movie-id/delete
Content-Type: application/json
X-Confirm-Delete: true

{"paths":["sample.mp4","subs/movie.en.srt"],"deletionMode":"recycle"}
```

Use the listing's `deletionMode` without substituting a default. When
`recycle_bin.enabled` is true and `recycle_bin.path` is nonempty, files are copied
into the existing recycle bin layout, preserving their library-relative paths,
then removed from the title folder. A recycle failure never falls back to
permanent deletion. Otherwise the mode is `permanent`. If policy changes between
listing and confirmation, deletion returns `409` and requires a fresh confirmation.
Rooted source/destination operations prevent symlink escapes during removal.

The server resolves the title folder from library registration and tracked files,
or an unambiguous title/year folder if no file evidence remains. It retains the
resolved folder before deletion so extras remain manageable after removing the
last episode file (migration `0091`). Loose files at a shared library root,
ambiguous folders, symlink title folders, and folders shared with another tracked
title are rejected rather than widening the deletion scope. Clients cannot supply
an absolute root or folder. Paths must exactly match the current listing.

Between 1 and 1000 unique files may be deleted per call. Invalid selections fail
before any deletion (`400`); unknown titles/folders return `404`; unsafe or
ambiguous title folders return `409`. Authentication is required when enabled;
non-admins receive `403`. On runtime/storage failure, `500` includes
`{"deleted":[...],"error":{"message":"..."}}`: already removed files are not
rolled back. Refresh the list before retrying. Each successful disk removal is
audited as `file_deleted` with actor, path and mode (visible in **Events** under
the **Library** category); movie-file soft deletion,
episode-file deletion, library file inventory, movie status, episode `has_file`,
and request availability are reconciled. Fulfillment history is retained.

## Acquisition history routes

| Method | Path | Auth | Response | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/downloads/history` | apiKey/bearer/cookie | `200 application/json` (`HistoryEntry[]`) | Completion and durable acquisition history, paginated with `limit`/`offset`. |
| `DELETE` | `/api/v1/downloads/history` | admin | `204` | Clear completion history; retain acquisition admission/retry records. |
| `POST` | `/api/v1/downloads/history/actions` | admin identity required | `200 application/json` | Enable retry or blocklist up to 100 latest failed/interrupted history items, with per-item outcomes. |

Acquisition history extends existing history rows with `acquisition_key`,
`media_type`, `media_id`, `indexer_id`, `release_hash`, `stage`, `reason`,
`attempt`, `retry_count`, `retry_eligible` and `next_retry_at` (UTC RFC3339).
Failure reasons are sanitized stage summaries, never raw provider/client errors.
`retry_count` counts attempts in the current bounded budget; `attempt` is
monotonic even after explicit recovery.

For example, an administrator can send
`{"ids":["history-id","another-id"],"action":"retry"}` to the actions route.
The response is `{"results":[{"id":"history-id","ok":true},{"id":"another-id","ok":false,"error":"History item not found or superseded."}]}`.
An invalid top-level request returns `400`; missing/non-admin identity returns
`403`. `retry` re-arms the budget without replaying stale download URLs: search
again to acquire fresh release coordinates. It does not bypass the blocklist.
`blocklist` reuses the existing release blocklist.

### Blocklist overrides

`POST /api/v1/download-clients/{id}/items` rejects a release whose info hash
(explicit or parsed from the magnet) or normalized title is blocklisted with
`409 {"error":{"code":"blocklisted",...}}`. Set `"ignore_blocklist": true` to
grab it anyway; interactive search exposes this as **Ignore blocklist**.
`POST /api/v1/autosearch/evaluate` accepts the same flag. Automatic search never
ignores the blocklist. Releases are blocklisted automatically when a download
stalls (see `downloads.stall_action` in [configuration](configuration.md)).

`SystemStatus` is `{ version, commit, buildDate, engine }` — the database
engine name is included so readiness tooling can confirm what backend
this instance is running against.

### Release evaluation and policy previews

`POST /api/v1/autosearch/evaluate` accepts the existing search identity context
(`media_type`, `media_id`, `title`, `alternate_titles`, year/IDs, season/episode,
`daily_date`, `runtime` in minutes), a `quality_profile_id`, and indexer `results`.
Results use the native indexer shape (`size` in bytes, `pub_date`, optional
`seeders`); no search or grab is performed. Linked movie identity is loaded from
the library as for automatic acquisition.

Omit `policy_preview` to use the saved profile. Supply it to replace only that
profile during this evaluation; its `items` is a JSON-encoded quality array,
`format_items` contains `{format_id, score}`, and `min_format_score`, `cutoff`,
`upgrade_allowed` have their native meanings. Quality definitions and
custom-format rules are not replaced. Invalid preview quality arrays and empty
or duplicate quality IDs return HTTP 400; evaluation failures return HTTP 500.
The response includes `preview`, `total`, `passed`, and `results`, ordered by the
same comparator as automatic acquisition (accepted results before rejected).

This example is available after this change ships; substitute local IDs/titles:

```bash
curl -sS -X POST http://localhost:1925/api/v1/autosearch/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"media_type":"movie","title":"Example","quality_profile_id":"profile-id",
       "results":[{"title":"Example.2024.1080p.BluRay","size":2000000000}],
       "policy_preview":{"items":"[{\"id\":\"quality-definition-id\",\"allowed\":true}]",
                         "min_format_score":100,"upgrade_allowed":true,
                         "format_items":[{"format_id":"format-id","score":50}]}}'
```

Each result adds optional `explanation`: `category`, `summary`, `identity` and
`checks` evidence arrays (`field`, `expected`, `actual`, `unit`, `passed`);
`numbering` (season/episode/multi-episode/absolute/date/pack interpretation);
profile-specific `contributions`; and, for accepted results, `rank` and
`ranking_reason`. Missing checks mean evaluation stopped, not success. The
composite score remains display-only. New search-log evaluations use the same
explanation shape; historical rows may omit it. See
[release matching](release-matching.md) for examples and pack-mapping limitations.

### Try every route with curl

Boot the binary on the default port (`make build && ./dist/loom serve`)
or hit your container, then:

```bash
$ curl -s http://localhost:1925/healthz
{"status":"ok"}

$ curl -s http://localhost:1925/livez
{"status":"alive"}

$ curl -s http://localhost:1925/readyz
{"status":"ready"}        # or HTTP 503 with {"status":"not ready","reason":"..."}

$ curl -s http://localhost:1925/api/v1/system/status
{"buildDate":"2026-05-01T02:27:12Z","commit":"e561225","engine":"sqlite","version":"e561225"}

$ curl -s http://localhost:1925/metrics | head -3
# HELP go_build_info Build information about the main Go module.
# TYPE go_build_info gauge
go_build_info{checksum="",path="github.com/ebenderooock/loom",version="..."} 1

# pprof is gated by debug.pprof: true
$ curl -s http://localhost:1925/debug/pprof/goroutine?debug=1 | head -1
goroutine profile: total 9
```

## Headers, middleware, and conventions

### Calendar and discovery details

- `GET /api/v1/calendar?start=YYYY-MM-DD&end=YYYY-MM-DD&timezone=Pacific/Auckland`
  returns a unified array of movie, episode, and album events. Events include
  `monitored` independently of `status`; episode events include `seriesId` to
  resolve the parent show's detail drawer. Status is one of `missing`,
  `downloaded`, `unaired`, `unreleased`, `searching`, `grabbed`, `downloading`,
  or `importing`. `downloaded` is displayed as Available in the UI.
- `GET /api/v1/discover/movie/{id}` and
  `GET /api/v1/discover/series/{id}` return TMDB metadata for a positive numeric
  TMDB ID. Fields use snake_case and include title, overview, poster, genres,
  rating, and release date; movies include runtime, shows include season count.
  Invalid IDs return 400; upstream failures return 502.
- These routes require authentication. Discovery details do not add an item;
  `POST /api/v1/import-lists/discover/add` with `{"item_id":"..."}` adds it
  using the parent list's library, quality, monitoring, and search settings.

- Every response includes `X-Request-Id` (echoed from the request when
  present, otherwise generated by chi's `RequestID` middleware).
- Responses are gzip-compressed when the client signals `Accept-Encoding`.
- `/api/v1/system/status` is wrapped with an `ETag` middleware for
  cheap polling.
- CORS is **off** by default; populate `cors.allowed_origins` to enable.

## Authentication (incoming, Phase 1c)

### TV device pairing

The `/pair` browser page explicitly approves a TV request after sign-in.
Device requests use JSON POST bodies; device secrets and API keys never
appear in approval links. All pairing responses use `Cache-Control: no-store`.

| Method | Path | Auth | Response / purpose |
|---|---|---|---|
| `POST` | `/api/v1/auth/pairing/start` | none | `201`: device secret, human code, verification path, expiry and polling interval. |
| `POST` | `/api/v1/auth/pairing/poll` | device secret in JSON | `200`: pending, denied, or a one-time approved API key. |
| `POST` | `/api/v1/auth/pairing/cancel` | device secret in JSON | `204`: cancel an outstanding request. |
| `GET` | `/api/v1/auth/pairing/request?code=XXXX-XXXX` | browser session or trusted proxy user | Review device name, code, status and remaining lifetime; no mutation. |
| `POST` | `/api/v1/auth/pairing/decision` | browser session or trusted proxy user | Explicit approve/deny with JSON and `X-Loom-Pairing: 1`. |

API-key/default identities cannot authorize devices. Pairing is unavailable
with authentication disabled or setup incomplete. Requests expire after ten
minutes; poll at most every five seconds. Rate limits return `429` and
`Retry-After`; expired/consumed requests return `410`. See
[device pairing](device-pairing.md) for deployment, transport, revocation,
and single-process/sticky-routing constraints.

The OpenAPI spec defines three security schemes ahead of the handlers:

- `apiKey` — header `X-Api-Key`.
- `bearerAuth` — HTTP `Authorization: Bearer <token>`.
- `cookieAuth` — cookie `loom_session`.

The endpoints that consume them land with the Phase 1c auth PR. See
ADR-0004 and [auth.md](auth.md).

Indexer and download-client configuration mutations and connectivity tests
require an administrator role. Authenticated non-admin users retain the
indexer search and download-use surfaces. Config/list/detail and
write responses redact credential-like fields (including API keys, passwords,
cookies, tokens, and usernames). When editing, leave a credential blank or
omit it to preserve the stored value; send a non-empty value to replace it.
Proxy mutations/tests, Jackett imports, indexer-rule mutations, and sensitive
search diagnostics (health/reset and query logs) are administrator-only too.
The server's explicitly disabled-auth mode retains its existing behavior.
Edit-form connectivity tests send the saved `id` plus unsaved settings to
the test-config endpoint, which restores unchanged credentials without saving.

## Indexers (Phase 2a)

The indexer routes accept and emit JSON. Conceptual reference lives
in [indexers.md](indexers.md); routes are summarised below.

```bash
# Create
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"id":"demo","kind":"builtin/null","name":"Demo","enabled":true,"priority":25,"categories":[2000]}' \
  http://localhost:1925/api/v1/indexers/

# List
curl -sS -H "X-Api-Key: $LOOM_KEY" http://localhost:1925/api/v1/indexers/

# Manual health check
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" \
  http://localhost:1925/api/v1/indexers/demo/test

# Fan-out search
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"query":"ubuntu","categories":[4000]}' \
  http://localhost:1925/api/v1/indexers/search
```

### Newznab + Torznab (Phase 2c)

Loom registers two real source kinds, `newznab` and `torznab`, in
addition to `builtin/null`. They share a single config schema and
parser; only the extended-attribute namespace differs. Full reference
lives in [indexers-newznab.md](indexers-newznab.md).

```bash
# Create a Newznab indexer (e.g. NZBHydra2 or Prowlarr)
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{
    "id": "hydra-news",
    "kind": "newznab",
    "name": "NZBHydra2",
    "enabled": true,
    "priority": 50,
    "config": {
      "url": "https://nzbhydra.example/api",
      "api_key": "abcdef0123456789",
      "timeout": "20s"
    },
    "categories": [2000, 5000, 7000]
  }' \
  http://localhost:1925/api/v1/indexers/

# Probe caps (cached to indexer_health.last_caps_json)
curl -sS -H "X-Api-Key: $LOOM_KEY" \
  http://localhost:1925/api/v1/indexers/hydra-news/caps
```

### Proxies (Phase 2e)

Indexers may pin outbound traffic through a proxy row. Conceptual
reference lives in [indexers-proxies.md](indexers-proxies.md). All
routes share the indexer auth scope.

```bash
# Create an HTTP proxy
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{
    "kind":"http","name":"Squid","enabled":true,
    "config":{"url":"http://squid.lan:3128","username":"u","password":"p"}
  }' http://localhost:1925/api/v1/proxies/

# Pin an existing indexer to that proxy
curl -sS -X PATCH -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"proxy_id":"http-squid"}' http://localhost:1925/api/v1/indexers/hydra-news

# Test the proxy
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" \
  http://localhost:1925/api/v1/proxies/http-squid/test
```

### Error envelope

Indexer endpoints reply to 4xx/5xx responses with a structured
envelope:

```json
{ "error": { "code": "not_found", "message": "indexer not found" } }
```

This is **not** the same shape used by the auth package (which today
emits `{"error":"<msg>"}`). Clients should branch on response status
before parsing the body, or accept either shape via
`{ "error": <string|{"code":"...","message":"..."}> }`. The
discrepancy is tracked for unification in a later phase; the
indexer envelope is the long-term direction.

## Adding a route

1. Author the handler in `internal/server/` (or under the relevant
   module package).
2. Update [`api/openapi/loom.yaml`](../api/openapi/loom.yaml) — _every_ new
   route must be in the spec.
3. Re-run `redocly lint api/openapi/loom.yaml` (or
   `swagger-cli validate`).
4. Update this page if the change touches the route table or auth model.

The doc-update policy is enforced by the PR template — see
[contributing-style.md](contributing-style.md#documentation-requirements).
