# Loom — Platform Documentation

> **Single binary, modular-monolith replacement for Radarr + Sonarr + Prowlarr.**
> Pre-alpha — not production-ready.

---

## Table of Contents

1. [Platform Vision & Goals](#1-platform-vision--goals)
2. [Architecture Overview](#2-architecture-overview)
3. [Capabilities Reference](#3-capabilities-reference)
   - 3.1 [Libraries](#31-libraries)
   - 3.2 [Movies](#32-movies)
   - 3.3 [TV Series](#33-tv-series)
   - 3.4 [Indexers](#34-indexers)
   - 3.5 [Download Clients](#35-download-clients)
   - 3.6 [Automated Search & Grab (AutoSearch)](#36-automated-search--grab-autosearch)
   - 3.7 [Workflows (Search → Download → Import Pipeline)](#37-workflows-search--download--import-pipeline)
   - 3.8 [Download Monitor](#38-download-monitor)
   - 3.9 [Quality Profiles](#39-quality-profiles)
   - 3.10 [Custom Formats](#310-custom-formats)
   - 3.11 [Import Lists](#311-import-lists)
   - 3.12 [Connect (Plex / Emby / Jellyfin / Trakt)](#312-connect-plex--emby--jellyfin--trakt)
   - 3.13 [Notifications](#313-notifications)
   - 3.14 [Proxies](#314-proxies)
   - 3.15 [Sources (RSS)](#315-sources-rss)
   - 3.16 [Rolling Search](#316-rolling-search)
   - 3.17 [Calendar](#317-calendar)
   - 3.18 [Sync Profiles](#318-sync-profiles)
   - 3.19 [Download Safety / Manual Review](#319-download-safety--manual-review)
   - 3.20 [Blocklist](#320-blocklist)
   - 3.21 [Remote Path Mappings](#321-remote-path-mappings)
   - 3.22 [Language Profiles](#322-language-profiles)
   - 3.23 [Anime Handling](#323-anime-handling)
   - 3.24 [Alternate Titles](#324-alternate-titles)
   - 3.25 [Season Packs](#325-season-packs)
   - 3.26 [Media Info & Naming](#326-media-info--naming)
   - 3.27 [Events & Audit Log](#327-events--audit-log)
   - 3.28 [Dashboard](#328-dashboard)
   - 3.29 [System / Health / Diagnostics](#329-system--health--diagnostics)
   - 3.30 [Compatibility Shims (arr-compat)](#330-compatibility-shims-arr-compat)
   - 3.31 [System Logs](#331-system-logs)
4. [Process Flows](#4-process-flows)
   - 4.1 [Manual Search & Grab Flow](#41-manual-search--grab-flow)
   - 4.2 [Automated Search & Grab Flow](#42-automated-search--grab-flow)
   - 4.3 [Download Lifecycle Flow](#43-download-lifecycle-flow)
   - 4.4 [Import / Post-Download Flow](#44-import--post-download-flow)
   - 4.5 [Import List Sync Flow](#45-import-list-sync-flow)
   - 4.6 [Trakt OAuth & Sync Flow](#46-trakt-oauth--sync-flow)
   - 4.7 [Library Scan Flow](#47-library-scan-flow)
5. [User Journeys](#5-user-journeys)
   - 5.1 [First-Time Setup](#51-first-time-setup)
   - 5.2 [Adding a Movie and Getting It Downloaded](#52-adding-a-movie-and-getting-it-downloaded)
   - 5.3 [Adding a TV Series](#53-adding-a-tv-series)
   - 5.4 [Setting Up Import Lists for Hands-Free Library Building](#54-setting-up-import-lists-for-hands-free-library-building)
   - 5.5 [Connecting Plex / Trakt for Watched-Status Archiving](#55-connecting-plex--trakt-for-watched-status-archiving)
   - 5.6 [Monitoring and Troubleshooting Downloads](#56-monitoring-and-troubleshooting-downloads)
   - 5.7 [Quality Upgrades Over Time](#57-quality-upgrades-over-time)
6. [Known Limitations & Incomplete Features](#6-known-limitations--incomplete-features)

---

## 1. Platform Vision & Goals

Loom aims to be a **single Go binary** that replaces the trio of Radarr (movies), Sonarr (TV), and Prowlarr (indexer management). Key design goals:

- **Unified experience** — one UI, one config, one database for movies + TV + indexers.
- **Container-native** — distroless image, `/config` for state, `/media` for libraries.
- **Observable** — Prometheus metrics at `/metrics`, structured logging, OpenTelemetry support.
- **Ecosystem-compatible** — wire-compatible Radarr/Sonarr/Prowlarr API shims so existing tools (Overseerr, Ombi, Tautulli) can integrate.
- **Modular monolith** — internal packages are cleanly separated so the app could theoretically be split into microservices later (Phase 11 goal).

**Current state:** Phases 0–2 complete, Phases 3–7 substantially done. Phase 3 (download workflow end-to-end) is the current focus.

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│                    Loom Binary                       │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐ │
│  │ React UI │  │ REST API │  │ Compat Shims      │ │
│  │ (embedded)│  │ /api/v1  │  │ /compat/radarr/.. │ │
│  └────┬─────┘  └────┬─────┘  └───────┬───────────┘ │
│       │              │                │             │
│  ┌────▼──────────────▼────────────────▼───────────┐ │
│  │              HTTP Router (chi)                  │ │
│  └────────────────────┬───────────────────────────┘ │
│                       │                             │
│  ┌────────────────────▼───────────────────────────┐ │
│  │           Service Layer (per-domain)            │ │
│  │  indexers · downloads · autosearch · workflows  │ │
│  │  movies · series · libraries · importlists      │ │
│  │  connect · notifications · qualityprofiles      │ │
│  │  customformats · scheduler · scanner · organizer│ │
│  └────────────────────┬───────────────────────────┘ │
│                       │                             │
│  ┌────────────────────▼───────────────────────────┐ │
│  │         Storage (SQLite or Postgres)            │ │
│  │         sqlc-generated query packages           │ │
│  └────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
          │                           │
          ▼                           ▼
   Download Clients            External APIs
   (qBit, Transmission,       (TMDB, Trakt, Plex,
    Deluge, SABnzbd,           Newznab/Torznab
    NZBGet)                    indexers)
```

**Key infrastructure:**
- **Scheduler** — cron-based jobs stored in `scheduled_jobs` table.
- **Event system** — internal pub/sub for download events, notifications, workflow transitions.
- **Health monitor** — tracks indexer and download client health with circuit-breaker patterns.
- **Download monitor** — periodic sweep of all download clients to detect completions, stalls, and failures.

---

## 3. Capabilities Reference

### 3.1 Libraries

**What it does:** Organises media into root folders on disk. Each library has a name, path, media type (movie/series), and default settings for items added to it.

**API:** `GET/POST/PUT/DELETE /api/v1/libraries`, `POST /{id}/scan`, `GET /{id}/unmapped`

**Key fields:**
- `name` — human-readable label for the library
- `path` — root filesystem path (e.g. `/media/movies`), must be unique
- `media_type` — `movie` or `series`
- `monitor_on_add` — whether new items are auto-monitored (default: true)
- `quality_profile_id` — default quality profile for new items (default: `"default"`)
- `unmonitor_on_delete` — unmonitor media when library is deleted
- `auto_archive_watched` — archive items after they're marked watched (via Trakt)
- `auto_archive_days_after_watch` — delay in days before archiving

**Computed fields (returned in API responses):**
- `accessible` — whether the path is reachable on disk
- `disk_space` — `{ total_bytes, used_bytes, free_bytes }` for the library volume
- `file_count` — number of indexed media files in `library_files`
- `unmapped_count` — number of top-level folders not matched to any media record

**Related table — `library_files`:**
Each scanned media file is tracked with: `id`, `library_id`, `path` (unique), `size_bytes`, `media_id` (nullable — set when matched to a movie/series), `last_scanned`, `created_at`.

**Expected outcomes:**
- Library appears in dashboard storage stats with disk usage.
- Scanning populates `library_files` and identifies unmapped folders.
- Movies/series can be assigned to libraries.

**Possible failures:**
- Path doesn't exist or isn't readable → scan fails.
- Permissions issues on `/media` mount.
- Unmapped folders remain if media isn't matched in TMDB.

---

### 3.2 Movies

**What it does:** Core movie entity management — add, search TMDB, monitor, track quality status, organise files, view credits and history.

**API:** `/api/v1/movies`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | List movies (paginated: `limit`, `offset`; in-memory filters: `search`, `status`, `quality`, `monitored`; sort: `sort`, `order`) |
| POST | `/` | Add movie (TMDB lookup → create record → optionally trigger search via `search_on_add`) |
| GET | `/search?q=` | Search local movie records |
| GET | `/lookup?term=` | Search TMDB for movies (metadata provider) |
| POST | `/bulk` | Bulk update: set `monitoring_status`, `quality_profile_id`, or `delete` for multiple IDs |
| POST | `/bulk-archive` | Bulk set monitoring to `archived` |
| POST | `/bulk-unarchive` | Bulk set monitoring to `monitored` |
| GET | `/files/{movieID}` | List media files for a movie |
| GET | `/{id}` | Get single movie |
| PUT | `/{id}` | Update movie fields |
| DELETE | `/{id}` | Soft-delete movie |
| PUT | `/{id}/monitoring` | Change monitoring status |
| POST | `/{id}/refresh` | Re-fetch metadata from TMDB |
| POST | `/{id}/archive` | Set monitoring to `archived` |
| POST | `/{id}/unarchive` | Set monitoring to `monitored` |
| GET | `/{id}/credits` | Fetch cast/crew from TMDB |
| GET | `/{id}/history` | View history of grabs/imports for this movie |

**Movie model fields:**
| Field | Description |
|-------|-------------|
| `id` | UUID |
| `title` | Display title |
| `year` | Release year |
| `imdb_id` | IMDB identifier |
| `tmdb_id` | TMDB identifier (primary metadata key) |
| `tvdb_id` | TVDB identifier |
| `overview` | Plot summary |
| `genres` | Genre list |
| `runtime` | Runtime in minutes |
| `rating` | TMDB rating |
| `backdrop_path` / `poster_path` | Image URLs |
| `metadata_provider` | Source of metadata (currently TMDB only) |
| `quality_profile_id` | Assigned quality profile |
| `library_id` | Which library this movie belongs to |
| `status` | Current status (see below) |
| `release_date` | General release date |
| `theatrical_date` / `digital_date` | Specific release dates |
| `last_search_at` | When the last automated search ran |
| `monitoring_status` | Monitoring state (see below) |
| `created_at` / `updated_at` / `deleted_at` | Timestamps |

**Status values:**
| Status | Meaning |
|--------|---------|
| `missing` | No file on disk |
| `unreleased` | Movie not yet released |
| `downloading` | Active download in progress |
| `storing` | File being processed/imported |
| `available_wrong_quality` | File exists but below quality cutoff |
| `available_right_quality` | File meets quality cutoff |
| `available_higher_quality` | File exceeds quality cutoff |

**Monitoring status values:** `monitored`, `unmonitored`, `deleted`, `archived`

**Key behaviours:**
- Add movie performs TMDB lookup by title/year, creating a local record with metadata.
- `search_on_add` option triggers an immediate automated search after adding.
- Refresh re-fetches metadata from TMDB (title, overview, images, release dates).
- Quality profile determines what releases are acceptable and when upgrades are sought.
- Bulk operations allow managing multiple movies at once from the UI.

**Expected outcomes:**
- Movie appears in library with status: missing → downloading → available.
- Monitored movies are eligible for automated search.
- Quality upgrades happen when a better release is found (within profile rules).

**Possible failures:**
- TMDB lookup fails (network, rate limit).
- Movie added but no indexer has results → stays "missing."
- Duplicate detection if movie already exists (checked by TMDB ID and IMDB ID).

---

### 3.3 TV Series

**What it does:** Series entity management with season/episode granularity — seasons, episodes, episode files, credits, and episode stats.

**API:** `/api/v1/series`

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/series/` | List series with filters (`search`, `status`, `monitored`, `sort`, `order`). Returns `episodeStats` per series. |
| `POST` | `/api/v1/series/` | Add series from TMDB — creates seasons/episodes. Optional `search: true` triggers immediate indexer search. |
| `GET` | `/api/v1/series/search?q=` | Search TMDB for TV series. |
| `GET` | `/api/v1/series/lookup?tmdbId=` | Lookup specific TMDB series details. |
| `POST` | `/api/v1/series/bulk` | Bulk update (monitoring status, quality profile) or delete multiple series. |
| `POST` | `/api/v1/series/bulk-archive` | Archive multiple series by IDs. |
| `POST` | `/api/v1/series/bulk-unarchive` | Unarchive multiple series by IDs. |
| `GET` | `/api/v1/series/{id}` | Get series with seasons, episodes, and episodeStats. |
| `PUT` | `/api/v1/series/{id}` | Update series fields (title, year, overview, genres, monitoring, quality profile, etc.). |
| `DELETE` | `/api/v1/series/{id}` | Delete series. May auto-unmonitor if library has unmonitor-on-delete enabled. |
| `PUT` | `/api/v1/series/{id}/monitoring` | Set monitoring status (validated against enum). |
| `POST` | `/api/v1/series/{id}/refresh` | Re-fetch metadata from TMDB — recreates seasons/episodes/credits. |
| `POST` | `/api/v1/series/{id}/archive` | Set monitoring to `archived`. |
| `POST` | `/api/v1/series/{id}/unarchive` | Set monitoring to `monitored`. |
| `GET` | `/api/v1/series/{id}/credits` | Get cast and crew (split by role). |
| `GET` | `/api/v1/series/{id}/seasons` | List seasons with per-season episode stats. |
| `GET` | `/api/v1/series/{id}/seasons/{seasonNum}/episodes` | List episodes for a season. Includes grab status when workflow store is available. |
| `POST` | `/api/v1/series/scan/` | Start a library scan for series. |
| `GET` | `/api/v1/series/scan/unmatched` | List unmatched files from last scan. |
| `GET` | `/api/v1/series/scan/{scanId}` | Get scan status. |
| `POST` | `/api/v1/series/{id}/rescan` | Rescan a single series folder. |

**Series model fields:**

| Field | Description |
|-------|-------------|
| `id` | UUID |
| `title` | Display title |
| `year` | First air year |
| `imdb_id` | IMDB identifier |
| `tmdb_id` | TMDB identifier (primary metadata key) |
| `tvdb_id` | TVDB identifier |
| `overview` | Plot summary |
| `genres` | Genre list (JSON array) |
| `runtime` | Episode runtime in minutes |
| `rating` | TMDB rating |
| `backdrop_path` / `poster_path` | Image URLs |
| `network` | Primary network (e.g. "HBO", "Netflix") |
| `status` | Airing status (see below) |
| `series_type` | Type classification (see below) |
| `metadata_provider` | Source of metadata (currently TMDB only) |
| `quality_profile_id` | Assigned quality profile |
| `library_id` | Which library this series belongs to |
| `monitoring_status` | Monitoring state (see below) |
| `season_folder` | Whether to use season subfolders for organisation |
| `release_date` | First air date |
| `created_at` / `updated_at` | Timestamps |
| `seasons` | Populated on read — list of Season objects |
| `episodes` | Populated on read — list of Episode objects |

**Season model:** `id`, `series_id`, `season_number`, `title`, `overview`, `poster_path`, `monitored`, `episode_count`, `created_at`, `updated_at`

**Episode model:** `id`, `series_id`, `season_id`, `episode_number`, `title`, `overview`, `air_date`, `runtime`, `still_path`, `monitored`, `has_file`, `created_at`, `updated_at`

**EpisodeFile model:** `id`, `episode_id`, `series_id`, `file_path`, `file_size`, `quality`, `source`, `resolution`, `codec`, `media_info` (JSON), `created_at`, `updated_at`

**SeriesCredit model:** `id`, `series_id`, `person_name`, `character_name`, `role`, `profile_path`, `tmdb_person_id`, `display_order`

**EpisodeStats:** `totalEpisodes`, `downloadedEpisodes`, `monitoredEpisodes`, `missingEpisodes` (monitored but not downloaded), `airedEpisodes` (air_date ≤ today)

**Series status values:**

| Status | Description |
|--------|-------------|
| `continuing` | Currently airing |
| `ended` | Finished airing |
| `upcoming` | Not yet aired |
| `cancelled` | Cancelled |

**Series type values:** `standard`, `daily`, `anime`

**Monitoring status values:**

| Status | Description |
|--------|-------------|
| `all` | Monitor all episodes |
| `future` | Only future episodes |
| `missing` | Only missing episodes |
| `existing` | Only existing episodes |
| `pilot` | Only pilot episode |
| `firstSeason` | First season only |
| `lastSeason` | Latest season only |
| `none` | No monitoring |
| `monitored` | General monitored state |
| `unmonitored` | Explicitly unmonitored |
| `archived` | Archived — excluded from searches |

**Key behaviours:**
- Adding a series fetches full metadata from TMDB including all seasons and episodes.
- If `monitoringStatus` is omitted on add, defaults to `all`.
- `SetMonitoringStatus` validates against the enum — invalid values are rejected.
- Refresh re-fetches from TMDB and **recreates** seasons/episodes/credits — local edits are lost.
- List endpoint supports in-memory filtering by `search`, `status`, `monitored` and sorting by `title`, `year`, `added`, `network`, `rating`.
- Bulk operations (`bulk`, `bulk-archive`, `bulk-unarchive`) process items independently — individual failures do not abort the batch.
- Delete may auto-set to `unmonitored` first if the library has `unmonitor_on_delete` enabled.

**Expected outcomes:**
- Series shows season/episode grid with per-episode status and download state.
- Missing episodes (monitored + not downloaded + aired) are eligible for automated search.
- Season packs can satisfy multiple episode needs at once.
- Episode stats show progress at series level and per-season level.

**Possible failures:**
- Episode numbering mismatches (especially anime — see §3.23).
- Season pack handling edge cases.
- Partial season availability.
- Refresh overwrites any local metadata edits.

---

### 3.4 Indexers

**What it does:** Manages Newznab/Torznab indexer connections used to search for releases.

**API:** `/api/v1/indexers` — CRUD, search, test, caps, definitions, health, rules, query log.

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/indexers/` | List all indexers with health and rate-limit info. |
| `POST` | `/api/v1/indexers/` | Create indexer from definition or manual config. |
| `POST` | `/api/v1/indexers/search` | Fan-out search across selected/all healthy indexers. |
| `POST` | `/api/v1/indexers/search/stream` | SSE streaming fan-out search with per-indexer results. |
| `POST` | `/api/v1/indexers/test` | Test an ephemeral indexer config without saving. |
| `GET` | `/api/v1/indexers/definitions` | List Cardigann catalogue (bundled tracker definitions). |
| `GET` | `/api/v1/indexers/{id}` | Get single indexer with health info. |
| `PUT` | `/api/v1/indexers/{id}` | Replace indexer definition entirely. |
| `PATCH` | `/api/v1/indexers/{id}` | Partial update of indexer fields. |
| `DELETE` | `/api/v1/indexers/{id}` | Delete indexer. |
| `GET` | `/api/v1/indexers/{id}/caps` | Fetch live capabilities from indexer. |
| `POST` | `/api/v1/indexers/{id}/test` | Test a saved indexer's connectivity. |
| `GET` | `/api/v1/indexers/health/` | Search health summary for all indexers. |
| `GET` | `/api/v1/indexers/health/{id}` | Search health for one indexer. |
| `POST` | `/api/v1/indexers/health/reset` | Reset all search-health metrics. |
| `POST` | `/api/v1/indexers/health/{id}/reset` | Reset one indexer's search-health metrics. |
| `GET` | `/api/v1/search/log/` | List query log entries (paginated with `limit`, `offset`). |
| `GET` | `/api/v1/search/log/{id}` | Get single query with per-indexer breakdown. |
| `DELETE` | `/api/v1/search/log/` | Prune query log entries older than `days` (default 30). |
| `GET` | `/api/v1/indexers/rules/` | List indexer rules. |
| `POST` | `/api/v1/indexers/rules/` | Create indexer rule. |
| `DELETE` | `/api/v1/indexers/rules/{ruleID}` | Delete indexer rule. |
| `POST` | `/api/v1/indexers/import-jackett` | Import indexers from Jackett instance. |

**Indexer definition fields:**

| Field | Description |
|-------|-------------|
| `id` | Unique ID (auto-generated or `jackett-{name}` for imports) |
| `kind` | Protocol — `newznab` or `torznab` |
| `name` | Display name |
| `enabled` | Whether indexer participates in searches |
| `priority` | Search priority (lower = higher priority) |
| `config` | JSON — URL, API key, and indexer-specific settings |
| `categories` | JSON — supported categories |
| `tags` | JSON — tags for rule filtering |
| `proxy_id` | Optional proxy to route traffic through |
| `rate_limit_per_min` | Per-indexer rate limit (requests per minute) |
| `rate_limit_burst` | Burst allowance for rate limiter |
| `retry_max_attempts` | Max retry attempts on failure |
| `created_at` / `updated_at` | Timestamps |

**Health tracking (two layers):**

1. **Persisted DB health** (`indexer_health` table): `indexer_id`, `status` (unknown/ok/degraded/failed), `last_checked_at`, `last_success_at`, `latency_ms`, `last_error`, `last_caps_json`. Updated on `TestOne()` and search result processing.

2. **In-memory search health** (`SearchHealthTracker`): Rolling metrics — total/success/fail counts, last search/error timestamps, rolling response times (100 samples), API call timestamps (24h window). Status derived from success rate: >90% → healthy, >70% → degraded, else failing.

**Query logging:**
- Every search operation is logged to `search_query_log` with per-indexer breakdown in `search_query_indexer_log`.
- Fields: query text, type, media type/ID, timing, total results, status, and per-indexer latency/result count/errors.

**Rate limiting:**
- Per-indexer configurable `rate_limit_per_min`, `rate_limit_burst`, `retry_max_attempts`.
- Implemented as HTTP transport wrapper — throttles requests before they reach the indexer.
- `RequestDelay` on definition caps RPM.

**Circuit breaker / availability:**
- **IndexerAvailability** (in-memory): Failure-based cooldown with escalating backoff — 5min → 15min → 30min → 1h.
- Methods: `RecordSuccess`, `RecordFailure`, `IsAvailable`, `FilterAvailable`.
- Unavailable indexers are skipped during fan-out search.

**Rules system:**
- Rules filter which indexers are eligible for a given media type.
- Fields: `indexer_id`, `media_type`, `category_filter`, `tag_filter`, `priority`, `enabled`.
- Fail-open: if no rules exist for a media type, all indexers are used.

**Catalogue:**
- Bundled Cardigann definitions for 100+ trackers.
- `GET /definitions` returns summaries: ID, name, description, type, language, links, settings, categories.

**Jackett import:**
- `POST /import-jackett` with `jackettUrl` + `apiKey`.
- Fetches configured indexers from Jackett API, creates Loom definitions prefixed `jackett-`.
- Auto-detects `newznab` vs `torznab` from Jackett type.

**Key features:**
- **Proxy support:** route indexer traffic through configured proxies.
- **Fan-out search:** parallel search across all healthy/available indexers with per-indexer timeouts.
- **SSE streaming search:** results stream in real-time as each indexer responds.

**Expected outcomes:**
- Indexer shows "healthy" with recent success metrics.
- Searches return results from all healthy indexers in parallel.

**Possible failures:**
- Invalid API key → test fails, health marked unhealthy.
- Indexer down → circuit breaker opens after escalating backoff, indexer skipped in searches.
- Rate limit hit → temporary failure, auto-retry later.
- Caps fetch fails → search categories may be wrong.

**Known limitations:**
- `resolveIndexerID` maps by name (not ID) in search diagnostics — fragile if names are duplicated or renamed.

---

### 3.5 Download Clients

**What it does:** Manages connections to torrent/usenet download clients.

**API:** `/api/v1/download-clients` — CRUD, test, categories, free-space, items, pause/resume/remove/priority/speed-limit/force-start/recheck/reannounce.

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/download-clients/` | List all clients with health info. |
| `POST` | `/api/v1/download-clients/` | Create download client. |
| `POST` | `/api/v1/download-clients/test` | Test unsaved client config. |
| `GET` | `/api/v1/download-clients/{id}/` | Get single client with health. |
| `PUT` | `/api/v1/download-clients/{id}/` | Replace client definition. |
| `PATCH` | `/api/v1/download-clients/{id}/` | Partial update of client fields. |
| `DELETE` | `/api/v1/download-clients/{id}/` | Delete client (returns 404 if not found). |
| `POST` | `/api/v1/download-clients/{id}/test` | Test saved client connectivity. |
| `GET` | `/api/v1/download-clients/{id}/categories` | List categories from client. |
| `GET` | `/api/v1/download-clients/{id}/free-space` | Get free space from client. |
| `GET` | `/api/v1/download-clients/{id}/items` | Get status of items (filter by `ids` query param). |
| `POST` | `/api/v1/download-clients/{id}/items` | Add/grab a release to client. |
| `POST` | `/api/v1/download-clients/{id}/pause` | Pause item(s). |
| `POST` | `/api/v1/download-clients/{id}/resume` | Resume item(s). |
| `POST` | `/api/v1/download-clients/{id}/remove` | Remove item(s). |
| `POST` | `/api/v1/download-clients/{id}/set-priority` | Set item priority. |
| `POST` | `/api/v1/download-clients/{id}/set-speed-limit` | Set speed limit. |
| `POST` | `/api/v1/download-clients/{id}/force-start` | Force start item. |
| `POST` | `/api/v1/download-clients/{id}/recheck` | Recheck item. |
| `POST` | `/api/v1/download-clients/{id}/reannounce` | Reannounce to tracker. |
| `GET` | `/api/v1/activity` | Activity queue across all clients. |
| `POST` | `/api/v1/activity/pause` | Pause activity items. |
| `POST` | `/api/v1/activity/resume` | Resume activity items. |
| `POST` | `/api/v1/activity/remove` | Remove activity items. |
| `GET` | `/api/v1/activity/detail` | Get detailed item info (tracker/peer/file details). |
| `GET` | `/api/v1/downloads/history` | Download history. |

**Supported client types:**

| Kind | Protocol | Description |
|------|----------|-------------|
| `qbittorrent` | torrent | qBittorrent WebUI API |
| `transmission` | torrent | Transmission RPC |
| `deluge` | torrent | Deluge JSON-RPC |
| `sabnzbd` | usenet | SABnzbd API |
| `nzbget` | usenet | NZBGet JSON-RPC |
| `builtin/torrent` | torrent | Built-in torrent client |
| `builtin/null` | — | No-op null client for testing |

**Download client model fields:**

| Field | Description |
|-------|-------------|
| `id` | UUID |
| `name` | Display name |
| `kind` | Client type (see above) |
| `protocol` | `torrent` or `usenet` |
| `enabled` | Whether client is active |
| `priority` | Selection priority (default 25, lower = higher priority) |
| `host` / `port` / `tls` | Connection details |
| `username` / `password` | Auth credentials |
| `config` | JSON — client-specific settings |
| `category_default` | Default category for new downloads |
| `save_path_default` | Default save path |
| `remove_completed` | Auto-remove completed downloads |
| `remove_failed` | Auto-remove failed downloads |
| `created_at` / `updated_at` | Timestamps |

**Health tracking:**

| Field | Description |
|-------|-------------|
| `client_id` | Client UUID |
| `status` | `unknown`, `ok`, `degraded`, `failed` |
| `last_checked_at` | Last health check time |
| `last_success_at` / `last_failure_at` | Last success/failure time |
| `last_error` | Last error message |
| `consecutive_failures` | Failure counter |
| `last_free_space_bytes` | Last known free space |
| `last_categories` | Last known categories (JSON) |

**Test connectivity:**
- Saved client: calls `client.Test(ctx)` with timeout, persists health, records categories and free space on success.
- Unsaved config: builds ephemeral client from request body, runs `Test()`, returns `{ok, error}`.

**Key behaviours:**
- Clients are hydrated into a live registry at startup.
- Free space and categories are captured during health checks.
- Per-item control endpoints support pause, resume, remove, priority, speed limit, force start, recheck, reannounce.
- Activity endpoints aggregate items across all registered clients.
- `DetailProvider` interface provides rich item info (tracker details, peer lists, file trees) for supported clients.

**Expected outcomes:**
- Client shows healthy with free space stats.
- Items can be sent to client and tracked.
- Activity page shows queue with real-time progress.

**Possible failures:**
- Connection refused → health marked unhealthy.
- Authentication failure.
- Insufficient disk space → download may stall.
- Client-specific API incompatibilities.

**Known limitations:**
- Priority-based client selection (`sortClientsByPriority`) is stubbed — clients are used in registry insertion order. TODO: Add `Priority()` to `DownloadClient` interface.

---

### 3.6 Automated Search & Grab (AutoSearch)

**What it does:** The core engine that searches indexers, evaluates results against quality profiles, and grabs the best match.

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/autosearch` | Trigger automated search + grab for a media item. |
| `POST` | `/api/v1/autosearch/evaluate` | Dry-run evaluate indexer results against quality profile (no grab). Used by manual search UI for quality badges. |

**Process (Engine.SearchAndGrab):**
1. Load the media item's quality profile.
2. Load quality definitions (resolution/codec tiers).
3. Build allowed quality tiers + custom format scores from profile.
4. Reject if media already has an active workflow (duplicate prevention).
5. Inspect existing file quality (for upgrade comparison).
6. Resolve cutoff tier from profile.
7. Build tiered query chain:
   - **Tier 0:** ID-based query (IMDb/TMDB/TVDB) if IDs present.
   - **Tier 1:** Title/alternate title text queries.
8. For each tier, search all queries via indexer fan-out.
9. Parse + evaluate each result; stop at first tier with accepted results.
10. If no results: return reason `"no results from indexers"`.
11. If all rejected: return reason `"all results rejected"` with top reject counts.
12. Sort accepted results by composite score descending.
13. Grab best candidate; fall back to next-best if add fails.
14. Record workflow linkage (if orchestrator present).

**Evaluate filters (in order):**
1. Parse release name → extract quality, codec, source, group, etc.
2. Identity check (title/year/IMDB match).
3. Quality mapping against definitions.
4. Allowed-quality check from profile.
5. Upgrade logic: `quality_cutoff_met`, `upgrade_not_allowed`, `not_an_upgrade`, `existing_quality_unknown`.
6. Zero-seeder reject.
7. Min/max size from quality definition (scaled by runtime).
8. Custom format scoring.
9. Min format score reject.
10. Tiebreaker computation.

**Scoring formula (ScoredRelease.CompositeScore):**
- `qualityWeight = (20 - qualityTier) × 1000` — quality tier dominates
- `formatWeight = formatScore` — custom format score
- `tiebreakerScore = seeders + age + size + freeleech`
- `compositeScore = qualityWeight + formatWeight + tiebreakerScore`

**Integration points:**
- **Indexers:** `indexerSvc.Search()` for fan-out search across healthy indexers.
- **Download clients:** `downloads.Registry` for grabbing best result.
- **Workflows:** `orchestrator.StartSearch()` + `Send(CmdGrabbed)` for pipeline tracking.
- **Quality profiles:** Profile items, cutoff, format items, min format score, upgrade allowed.
- **Custom formats:** `customformats.Engine.ScoreRelease()` for bonus/penalty scoring.
- **Movie/series services:** Existing file quality checks for upgrade decisions.
- **Parser:** `internal/parser/` — release name parsing (title, year, resolution, source, codec, season/episode, etc.).

**Expected outcomes:**
- Best available release is grabbed and sent to download client.
- Workflow created to track progress through pipeline.
- If no results pass quality filters → no grab, media stays "missing."

**Possible failures:**
- No indexers healthy → search returns empty.
- All results rejected by quality profile → no grab.
- Download client unreachable → grab fails.
- Timeout during indexer search.
- Duplicate workflow prevention blocks search if one is already active.

---

### 3.7 Workflows (Search → Download → Import Pipeline)

**What it does:** State machine tracking the full lifecycle of a search-to-import operation.

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/workflows` | List recent workflows (last 50). |
| `GET` | `/api/v1/workflows/{id}` | Get single workflow with items and history. |
| `GET` | `/api/v1/workflows/{id}/events` | List audit events for a workflow. |
| `POST` | `/api/v1/workflows/{id}/cancel` | Cancel an active workflow. Routes through orchestrator if available. |
| `POST` | `/api/v1/workflows/{id}/retry` | Retry a failed workflow. Routes through orchestrator if available. |
| `DELETE` | `/api/v1/workflows/{id}` | Delete workflow and all associated items/history. |

**Workflow model fields:**

| Field | Description |
|-------|-------------|
| `id` | UUID |
| `type` | Workflow type |
| `state` | Current state (see state machine below) |
| `media_type` | `movie` or `episode` |
| `grab_title` | Release title that was grabbed |
| `download_client_id` | Download client handling this workflow |
| `download_id` | ID within the download client |
| `quality_profile_id` | Quality profile used for search |
| `retry_count` | Number of retry attempts |
| `max_retries` | Maximum allowed retries |
| `last_error` | Last error message |
| `metadata` | JSON metadata (indexer, quality, size, etc.) |
| `created_at` / `updated_at` | Timestamps |
| `completed_at` | When workflow completed (nullable) |
| `items` | Populated on read — associated workflow items |
| `history` | Populated on read — state transition history |

**WorkflowEvent fields:** `id`, `workflow_id`, `from_state`, `to_state`, `message`, `created_at`

**State machine:**
```
searching → grabbed → downloading → post_download → importing → completed
    │           │          │              │              │
    └───────────┴──────────┴──────────────┴──────────────┘
                           │
                           ▼
                     failed / cancelled
```

**States explained:**

| State | Meaning |
|-------|---------|
| `searching` | Indexers are being queried for this media item |
| `grabbed` | A release was selected and sent to the download client |
| `downloading` | Download client is actively downloading the item |
| `post_download` | Download complete, awaiting processing (seeding ratio/time checks) |
| `importing` | File is being moved/renamed into the library |
| `completed` | Successfully imported into library |
| `failed` | An error occurred at any stage |
| `cancelled` | User cancelled the workflow |

**Valid transitions:**

| From | To |
|------|----|
| `searching` | `grabbed`, `failed`, `cancelled` |
| `grabbed` | `downloading`, `failed`, `cancelled` |
| `downloading` | `post_download`, `failed`, `cancelled` |
| `post_download` | `importing`, `failed`, `cancelled` |
| `importing` | `completed`, `failed`, `cancelled` |
| `failed` | `searching`, `downloading`, `post_download`, `importing`, `completed` (via retry/recovery) |

**Orchestrator commands:**
- `CmdSearchStarted` — new search initiated
- `CmdGrabbed` — release grabbed, sent to download client
- `CmdDownloadProgress` — progress update from download monitor
- `CmdDownloadComplete` — download finished
- `CmdImportResult` — import succeeded or failed
- `CmdCancel` — user cancellation
- `CmdRetry` — user retry (with smart retry logic)
- `CmdDownloadRemoved` — download removed externally
- `CmdTick` — periodic maintenance (stale detection, pruning, post-download checks)

**Key behaviours:**
- **Duplicate prevention:** only one active workflow per media item at a time.
- **Retry:** failed workflows can be retried with smart retry (targets appropriate state based on failure point).
- **Recovery:** `RecoverToImporting`, `RecoverToPostDownload`, `RecoverToDownloading` for manual recovery.
- **Cancel:** cancels the workflow and resets media status.
- **Media status:** workflow transitions update the media item's status (missing → downloading → available).
- **Post-download policy:** seeding ratio/time gating before import.
- **Boot reconciliation:** on startup, reconciles workflow state with actual download client state.
- **Progress buffering:** download progress updates are coalesced to reduce DB writes.
- **Stale detection:** workflows stuck beyond state timeouts are detected and handled.
- **Pruning:** completed workflows are pruned periodically.

**Integration points:**
- **AutoSearch:** `orchestrator.StartSearch()` + `Send(CmdGrabbed)` to create and advance workflows.
- **Download monitor:** `NotifyDownloadComplete`, `NotifyDownloadProgress`, `NotifyDownloadRemoved`.
- **Import pipeline:** `importFn` callback triggered on transition to `importing`.
- **Media services:** status updates on workflow transitions.

**Expected outcomes:**
- User can see real-time pipeline progress on the Workflows page.
- Completed workflows result in media files in the library.
- Failed workflows show error details and can be retried.
- **Workflow Logs tab** shows all application-level log entries correlated to this workflow (via `workflow_id` context propagation), enabling deep diagnosis of failures.

**Possible failures:**
- Search yields no results → workflow fails at `searching`.
- Grab fails (client down) → workflow fails at `grabbed`.
- Download stalls → detected by monitor, workflow fails at `downloading`.
- Import fails (permissions, disk full, file parsing error) → fails at `importing`.
- **Current known issue:** the full end-to-end pipeline has reliability issues; the import/post-download stages may not complete successfully.

---

### 3.8 Download Monitor

**What it does:** Periodic background sweep of all download clients to detect state changes.

**Scheduling:** Polls every 30 seconds (configurable `CheckInterval`). Runs an immediate sweep on startup, then continues on a ticker loop. Started as a goroutine in `cmd/loom/wire_downloads.go`.

**Process (Monitor.Run — single sweep):**
1. Fan out `Status()` across all registered download clients.
2. Log per-client errors but continue (partial failure tolerance).
3. `emitCompletions()` — detect newly completed/seeding items.
4. Forward progress updates to workflow orchestrator for downloading/paused/seeding/completed items.
5. `detectStalled()` — detect stalled and failed downloads (if enabled).

**Completion detection (`emitCompletions`):**
1. Check each item for `StatusItemCompleted` or `StatusItemSeeding`.
2. Deduplicate against `lastCompleted` in-memory map (key: `clientID:itemID`).
3. Cross-restart idempotency via `HistoryStore.WasCompleted()` (persisted DB check).
4. Publish `DownloadCompletedEvent` on event bus.
5. Notify workflow orchestrator via `NotifyDownloadComplete()`.
6. Persist completion in `HistoryStore.RecordCompletion()`.
7. Update `lastCompleted` set for next sweep.

**Stall detection (`detectStalled`):**
1. Track `StatusItemDownloading` items with progress bytes/rate.
2. On first sighting, store initial progress and timestamp.
3. If progress changes between sweeps, reset the stall timer.
4. If progress is unchanged for `StallTimeout` (default 30 minutes), mark as stalled and invoke `StallHandler`.
5. `StatusItemFailed` items are immediately handled via `StallHandler` (once per item).
6. Prune tracking state for items no longer active.

**Events published:**

| Event | Description |
|-------|-------------|
| `downloads.completed` | Download finished — triggers import workflow |
| `downloads.stalled` | Download stalled (no progress for timeout period) |
| `downloads.retry` | Stalled download being retried |
| `downloads.failed` | Download failed |
| `downloads.queued` | New download queued (published elsewhere) |

**Workflow orchestrator integration:**
- `NotifyDownloadComplete()` → sends `CmdDownloadComplete` to orchestrator
- `NotifyDownloadProgress()` → sends `CmdDownloadProgress` with rate/ratio/status
- `NotifyDownloadRemoved()` → sends `CmdDownloadRemoved`
- Progress is forwarded for `Downloading`, `Paused`, `Seeding`, `Completed` states

**State tracking (in-memory):**
- `lastCompleted` — set of `clientID:itemID` seen as completed in previous sweep
- `lastProgress` — per-item progress bytes, download rate, and timestamp for stall detection
- `stalledEmitted` — per-item flag to avoid duplicate stall notifications

**Expected outcomes:**
- Completed downloads are detected and trigger import workflows.
- Stalled downloads are flagged for retry or manual intervention.
- Activity page reflects real-time download state.

**Possible failures:**
- Client unreachable → monitor skips client, logs error.
- Active grab record missing in DB → completion not matched.
- Race condition between monitor sweep and manual user actions.

**Known limitations:**
- `HistoryStore.WasCompleted()` uses only `(client_id, download_id)` — if a client reuses IDs, deduplication may misfire.
- `lastCompleted` is in-memory only; cross-restart idempotency relies entirely on the history table.
- Event bus publish errors are silently ignored.

---

### 3.9 Quality Profiles

**What it does:** Defines quality preferences that control what releases are acceptable and when upgrades should happen.

**API:** `/api/v1/quality-profiles` — CRUD + format score endpoints.

**Key concepts:**
- **Quality tiers:** ordered list of acceptable qualities (e.g., HDTV-720p < Bluray-1080p < Remux-2160p).
- **Cutoff:** the quality tier at which Loom stops searching for upgrades.
- **Allowed items:** which quality tiers are acceptable at all.
- **Custom format scores:** bonus/penalty scores applied per custom format match.

**Expected outcomes:**
- Only releases matching allowed qualities are grabbed.
- Upgrades happen automatically when a better-scoring release is found.
- Upgrades stop once the cutoff quality is reached.

**Possible failures:**
- Profile too restrictive → nothing ever matches.
- Custom format scores misconfigured → wrong release preferred.

---

### 3.10 Custom Formats

**What it does:** Rule-based release name matching that assigns scores to releases.

**API:** `/api/v1/custom-formats` — CRUD, test, import presets.

**How it works:**
- Each custom format has one or more conditions (regex patterns, size ranges, etc.).
- When a release name matches all conditions, the format's score (from the quality profile) is added.
- Positive scores = preferred, negative scores = penalised.

**Examples:**
- "DV" format → +100 score for Dolby Vision releases.
- "x265" format → +50 for HEVC encoding.
- "CAM" format → -1000 to reject cam rips.

**Expected outcomes:**
- Releases are ranked considering both quality tier and format scores.
- Users can fine-tune preferences without changing quality profiles.

---

### 3.11 Import Lists

**What it does:** Automatically adds movies/series from external list sources.

**API:** `/api/v1/import-lists` — CRUD, sync, exclusions, Trakt user lists.

**Supported providers:**
| Provider | List types |
|----------|-----------|
| Trakt | User list, watchlist, popular, trending, anticipated |
| IMDb | User list, watchlist |
| TMDb | User list, popular |
| Plex | Watchlist |
| Radarr | External Radarr instance |
| Sonarr | External Sonarr instance |
| RSS | Custom RSS feed |

**Sync process (SyncManager.SyncList):**
1. Fetch items from provider API.
2. Fill credentials (Trakt OAuth tokens from Connect, TMDb API key from config).
3. Check exclusion list → skip excluded items.
4. Upsert items into import list items table.
5. Update `last_sync` timestamp.
6. Process pending items → create movie/series records in library.
7. Background sync loop runs every minute.

**Expected outcomes:**
- New items appear in library automatically.
- Exclusions prevent unwanted re-adds.
- Sync status visible on Import Lists page.

**Possible failures:**
- Provider API down or rate-limited.
- Trakt OAuth token expired → needs refresh.
- TMDb API key missing → TMDb lists fail.
- Items not found in TMDB metadata → skipped.

---

### 3.12 Connect (Plex / Emby / Jellyfin / Trakt)

**What it does:** Integrates with media servers and tracking services.

**API:** `/api/v1/connect` — CRUD, test, Trakt OAuth, Trakt sync.

**Providers:**

| Provider | Features |
|----------|----------|
| Plex | Library refresh on import, host + API key auth |
| Emby | Library refresh on import, host + API key auth |
| Jellyfin | Library refresh on import, host + API key auth |
| Trakt | OAuth2 auth, sync watched/collection/watchlist, auto-archive watched items |

**Trakt-specific:**
- OAuth2 flow: authorize URL → user approves → callback with code → token exchange.
- Sync endpoints: watched, collection, watchlist.
- `mediaArchiver` bridges Trakt watched status to auto-archive in libraries.
- Token refresh for long-lived connections.

**Expected outcomes:**
- Plex/Emby/Jellyfin: library auto-refreshes when new media is imported.
- Trakt: watched status syncs, watched items auto-archive, watchlist syncs to import list.

**Possible failures:**
- Media server unreachable → refresh silently fails.
- Trakt OAuth expired → needs manual re-auth or refresh.
- Trakt API rate limits.

---

### 3.13 Notifications

**What it does:** Sends notifications on system events to various channels.

**API:** `/api/v1/notifications` — CRUD, test, history.

**Supported services:** Discord, Slack, Telegram, Email, Webhook, and others.

**Event triggers:**
| Event | When it fires |
|-------|--------------|
| Grab | Release sent to download client |
| Download | Download completed |
| Upgrade | Quality upgrade imported |
| Rename | Media file renamed/moved |
| Delete | Media deleted |
| Health Issue | Indexer/client health problem |
| Application Update | New Loom version available |

**Expected outcomes:**
- Notifications sent in parallel to all configured channels.
- History log shows past notifications.
- Test send verifies configuration.

**Possible failures:**
- Service unreachable → notification silently fails, logged in history.
- Invalid webhook URL or API token.
- Rate limiting by notification service.

---

### 3.14 Proxies

**What it does:** HTTP/SOCKS proxy servers that can be attached to indexers.

**API:** `/api/v1/proxies` — CRUD, test.

**Expected outcomes:**
- Indexer traffic routed through proxy.
- Useful for accessing geo-restricted indexers.

**Possible failures:**
- Proxy unreachable → indexer searches fail.
- Authentication failure.

---

### 3.15 Sources (RSS)

**What it does:** User-defined RSS sources for discovering new releases.

**API:** `/api/v1/sources` (implied from frontend)

**Expected outcomes:**
- RSS feeds are periodically fetched.
- New items stored in `rss_items` table.
- Can trigger automated search for matching media.

---

### 3.16 Rolling Search

**What it does:** Scheduled background search that cycles through monitored media looking for missing or upgradable items.

**API:** `/api/v1/rolling-search`

**Process:**
- Maintains `search_state` table tracking position in the media library.
- Periodically picks the next batch of monitored items.
- Runs autosearch for each.
- Avoids hammering indexers by spreading searches over time.

**Expected outcomes:**
- Missing media eventually gets found and downloaded without manual intervention.
- Upgrades happen as better releases become available.

**Possible failures:**
- State gets stuck if errors accumulate.
- Indexer rate limits slow progress.

---

### 3.17 Calendar

**What it does:** Shows upcoming and recent media releases on a calendar view.

**API:** `/api/v1/calendar`

**Expected outcomes:**
- Month view showing release dates for monitored series/movies.
- Highlights missing items that need searching.

---

### 3.18 Sync Profiles

**What it does:** Configures which indexers and categories to use for specific sync/search operations.

**API:** `/api/v1/sync-profiles`

**Database:** `sync_profiles`, `sync_profile_indexers`, `sync_profile_categories`

---

### 3.19 Download Safety / Manual Review

**What it does:** Allows manual review of releases before they're grabbed.

**API:** `/api/v1/reviews`

**Database:** `manual_review`

**Expected outcomes:**
- Releases flagged for review are held until approved.
- Prevents accidental downloads of unwanted content.

---

### 3.20 Blocklist

**What it does:** Prevents specific releases from being grabbed again.

**API:** `/api/v1/blocklist`

**Database:** `blocklist`

**Expected outcomes:**
- Blocklisted releases are skipped during search evaluation.
- Useful for releases that failed import or had quality issues.

---

### 3.21 Remote Path Mappings

**What it does:** Maps paths between the download client's filesystem and Loom's filesystem.

**API:** `/api/v1/download-clients/remote-path-mappings`

**Database:** `remote_path_mappings`

**Use case:** When the download client runs on a different machine or in a different container, the completed download path may differ from what Loom sees.

**Expected outcomes:**
- Loom correctly locates completed downloads for import.

**Possible failures:**
- Incorrect mapping → import can't find files → workflow fails at import stage.
- Missing mapping when client and Loom have different mount points.

---

### 3.22 Language Profiles

**What it does:** Configures preferred languages for media search and selection.

**API:** `/api/v1/languages` (implied)

**Database:** `language_profiles`

---

### 3.23 Anime Handling

**What it does:** Special handling for anime episode numbering (absolute vs season-based).

**API:** `/api/v1/anime`

**Database:** `anime_preferences`, `anime_mappings`

**Key challenge:** Anime often uses absolute episode numbers (1-900+) rather than Season×Episode format. Mapping between the two requires external databases.

---

### 3.24 Alternate Titles

**What it does:** Associates alternate/foreign titles with media for better search matching.

**API:** `/api/v1/alt-titles`

**Database:** `alternate_titles`

---

### 3.25 Season Packs

**What it does:** Handles season pack downloads that contain multiple episodes.

**API:** `/api/v1/packs`

**Database:** `season_pack_history`

---

### 3.26 Media Info & Naming

**What it does:** Parsing release names and configuring naming conventions.

**API:** `/api/v1/media-info` — `getMediaPreferences`, `updateMediaPreferences`, `parseReleaseName`

**Key features:**
- Release name parser extracts: quality, codec, source, group, resolution, etc.
- Naming settings control how imported files are renamed.
- Import mode configuration (copy, move, hardlink).

---

### 3.27 Events & Audit Log

**What it does:** System event trail for debugging and compliance.

**API:** `GET /api/v1/system/audit-log`

**Database:** `audit_log`

**Events logged:**
- Configuration changes
- Media additions/deletions
- Download events (grab, complete, fail)
- System health changes

---

### 3.28 Dashboard

**What it does:** Home page with system overview and quick actions.

**Displays:**
- Media counts (movies, TV shows)
- Indexer count and health summary
- Download client count
- Active downloads with progress
- Storage usage per library
- System health issues
- Onboarding cards for fresh installs

**Quick actions:**
- Add Movie
- Add Series
- Search All (trigger search for all missing)
- RSS Sync

---

### 3.29 System / Health / Diagnostics

**API endpoints:**
- `GET /healthz` — basic health check
- `GET /livez` — liveness probe
- `GET /readyz` — readiness probe
- `GET /metrics` — Prometheus metrics
- `GET /api/v1/system/status` — version, commit, uptime
- `GET /api/v1/system/health/*` — component health details
- `GET /debug/pprof/*` — Go profiling (when enabled)

---

### 3.30 Compatibility Shims (arr-compat)

**What it does:** Wire-compatible API endpoints that mimic Radarr v3, Sonarr v3, and Prowlarr v1 APIs.

**Routes:**
- `/compat/radarr/api/v3/*`
- `/compat/sonarr/api/v3/*`
- `/compat/prowlarr/api/v1/*`

**Purpose:** Allows tools like Overseerr, Ombi, and Tautulli to interact with Loom as if it were the original *arr apps.

---

### 3.31 System Logs

**What it does:** Captures application-level `slog` output (info, warn, error messages from every subsystem) into an in-memory ring buffer and a persistent database table. Provides a real-time streaming endpoint (SSE) and a paginated history API, surfaced in the UI under **Settings → System**.

**Why it exists:** The existing Events / Audit Log (§3.27) records domain-level actions (e.g. "download grabbed", "library scanned"). System Logs capture the underlying application log stream — startup messages, HTTP request errors, internal state transitions, background-job output — giving users without OTLP infrastructure visibility into what Loom is doing.

**Architecture:**
```
slog.Logger
  └─ CaptureHandler (wraps redactingHandler)
       ├─ Console output (unchanged — stdout, JSON/text)
       ├─ RingBuffer (in-memory, 5,000 entries, SSE fan-out)
       └─ BatchWriter → system_logs DB table (async, non-blocking)
```

**Capture level:** Independent of the console log level. Defaults to `warn` but can be changed at runtime via the API or UI (debug / info / warn / error). Lowering it to `debug` captures everything; raising it to `error` captures only errors.

**Workflow correlation:** Every log entry can carry an optional `workflow_id`. The `CaptureHandler` extracts this from `context.Context` (set by the workflow engine at each entry point) or falls back to scanning slog attributes for `"workflow_id"` / `"id"` keys. This enables "show me all logs for workflow X" queries.

**API:** `/api/v1/system/logs`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/system/logs` | Paginated list with filters: `level`, `search`, `workflow_id`, `since`, `until`, `limit`, `offset` |
| GET | `/api/v1/system/logs/stream` | SSE real-time stream. Optional `workflow_id` query param to filter. |
| GET | `/api/v1/system/logs/config` | Get current capture level |
| PUT | `/api/v1/system/logs/config` | Update capture level at runtime (`{"capture_level":"info"}`) |
| DELETE | `/api/v1/system/logs` | Clear all persisted log entries |

**Storage:**
- **Ring buffer:** Fixed-size circular buffer (default 5,000 entries). SSE clients subscribe here for low-latency streaming. Drop-on-full semantics for slow consumers.
- **Database:** `system_logs` table with indexes on `timestamp`, `level`, `workflow_id`. Batch-inserted asynchronously (flush every 500ms or 100 entries). Pruned daily (default retention: 7 days).

**UI locations:**
- **Settings → System** tab: full log viewer with live/history modes, level filter, text search, capture level config, and clear button.
- **Workflow Detail → Logs** tab: shows only logs for that specific workflow, with real-time streaming for active workflows.

**Expected outcomes:**
- Users can view real-time application activity without SSH or OTLP.
- Workflow failures can be diagnosed by switching to the Logs tab on the workflow detail page.
- Capture level can be temporarily lowered to `debug` for troubleshooting, then raised back.

**Possible failures:**
- Ring buffer overflow: oldest entries silently dropped (by design).
- DB write failure: logged to console but does not block the application.
- SSE disconnection: browser EventSource auto-reconnects.

---

## 4. Process Flows

### 4.1 Manual Search & Grab Flow

```
User clicks "Search" on a movie/series
        │
        ▼
POST /api/v1/indexers/search
        │
        ▼
┌───────────────────────────────┐
│  For each healthy indexer:    │
│  - Apply rate limits          │
│  - Query via Newznab/Torznab  │
│  - Timeout after configured   │
│    duration                   │
│  - Log query in search_query_log
└───────────┬───────────────────┘
            │
            ▼
   Aggregate results
   Parse release names
   Score each result
            │
            ▼
   Display results to user
   (sorted by score)
            │
            ▼
   User selects a release
            │
            ▼
POST /api/v1/download-clients/{id}/items
   (grab release → send to client)
            │
            ▼
   Workflow created: state = "grabbed"
   Active grab record created
   Notification: "Grab" event fired
```

### 4.2 Automated Search & Grab Flow

```
Trigger: Rolling search scheduler
         OR "Search All" button
         OR import list adds new media
         OR RSS sync finds match
                │
                ▼
POST /api/v1/autosearch
                │
                ▼
┌─────────────────────────────────────────┐
│  AutoSearch Engine.SearchAndGrab:       │
│  1. Check no active workflow exists     │
│  2. Load quality profile               │
│  3. Load quality definitions           │
│  4. Build allowed tiers + format scores│
│  5. Parse existing file quality        │
│  6. Search all healthy indexers        │
│  7. Evaluate each result:              │
│     - Parse release name              │
│     - Check quality tier allowed      │
│     - Check if upgrade over existing  │
│     - Calculate composite score       │
│     - Apply custom format scores      │
│  8. Sort by score, pick best          │
│  9. Grab via download client          │
│  10. Create workflow record           │
└─────────────────────────────────────────┘
                │
        ┌───────┴────────┐
        ▼                ▼
   Results found    No results
   Best grabbed     Media stays
   Workflow created "missing"
```

### 4.3 Download Lifecycle Flow

```
Release grabbed → sent to download client
        │
        ▼
   Download client starts download
   Workflow state: "downloading"
        │
        ▼
┌──────────────────────────────────┐
│  Download Monitor (periodic):    │
│  - Sweep all clients            │
│  - Compare items vs active_grabs│
│  - Detect state changes         │
└──────────┬───────────────────────┘
           │
     ┌─────┴──────────┬──────────────┐
     ▼                ▼              ▼
  Completed        Stalled        Failed
     │                │              │
     ▼                ▼              ▼
  Emit            Emit           Emit
  "completed"     "stalled"      "failed"
  event           event          event
     │                │              │
     ▼                ▼              ▼
  Trigger         Flag for       Retry or
  import          retry/manual   blocklist
  workflow        intervention
```

### 4.4 Import / Post-Download Flow

```
Download completed event received
        │
        ▼
   Workflow transitions to "post_download"
        │
        ▼
┌──────────────────────────────────────┐
│  Post-download processing:           │
│  1. Apply remote path mappings      │
│  2. Locate completed files          │
│  3. Parse file info (mediainfo)     │
│  4. Match to media item             │
│  5. Apply naming conventions        │
│  6. Move/copy/hardlink to library   │
│     (based on import mode setting)  │
│  7. Update media status → available │
│  8. Record in import_history        │
│  9. Clean up grab records           │
│  10. Notify Connect (Plex refresh)  │
│  11. Fire "Download" notification   │
└──────────────────────────────────────┘
        │
   ┌────┴────┐
   ▼         ▼
Success    Failure
   │         │
   ▼         ▼
Workflow   Workflow
→ completed → failed
             │
             ▼
          Can retry
```

**⚠️ Known issue:** This flow is the area with the most reported problems. The post-download → import transition may not complete reliably. See §6.

### 4.5 Import List Sync Flow

```
Background sync (every 60 seconds)
   OR manual "Sync Now"
        │
        ▼
┌───────────────────────────────────┐
│  SyncManager.SyncList:            │
│  1. Fetch items from provider     │
│     (Trakt/IMDb/TMDb/Plex/RSS)   │
│  2. Resolve Trakt creds from      │
│     Connect service               │
│  3. Auto-fill TMDb API key        │
│  4. For each item:                │
│     a. Check exclusion list       │
│     b. Skip if excluded           │
│     c. Upsert into import items   │
│  5. Update last_sync timestamp    │
│  6. Process pending items:        │
│     a. Lookup in TMDB metadata    │
│     b. Create movie/series record │
│     c. Assign to library          │
│     d. Optionally trigger search  │
└───────────────────────────────────┘
```

### 4.6 Trakt OAuth & Sync Flow

```
User configures Trakt connection
        │
        ▼
Enter Client ID + Client Secret
Click "Save & Authorize"
        │
        ▼
POST /api/v1/connect/trakt/oauth/authorize
   → returns authorize_url
        │
        ▼
Browser opens Trakt authorization page
User approves application
        │
        ▼
Trakt redirects to /settings/trakt/callback?code=...
        │
        ▼
Callback page extracts code
Redirects to /settings?trakt_code=...
        │
        ▼
Settings page auto-populates code field
User clicks "Complete Authorization"
        │
        ▼
POST /api/v1/connect/trakt/oauth/callback
   → exchanges code for access_token + refresh_token
   → stores tokens in connection settings
        │
        ▼
Connection status: "connected"
Sync endpoints now available:
   POST /trakt/sync/watched/{id}
   POST /trakt/sync/collection/{id}
   POST /trakt/sync/watchlist/{id}
```

### 4.7 Library Scan Flow

```
User clicks "Scan Library"
        │
        ▼
POST /api/v1/libraries/{id}/scan
  → returns 202 Accepted immediately
  → scan runs in background goroutine
        │
        ▼
┌────────────────────────────────────┐
│  libraries.Scanner.ScanLibrary:    │
│  1. Walk filesystem under library  │
│     path recursively               │
│  2. Skip hidden directories        │
│  3. Index video files (.mkv, .mp4, │
│     .avi, etc.) into library_files │
│     via UpsertFile (ON CONFLICT    │
│     updates size + last_scanned)   │
│  4. Delete stale files not seen    │
│     since scan start               │
│  5. Compute disk space stats       │
└────────────────────────────────────┘
        │
        ▼
Library shows updated file counts
Unmapped folders available for review
Dashboard shows updated storage stats

Note: The libraries scanner only indexes
files. Matching files to movie/series
records and parsing media info is handled
separately by the media scanner
(internal/scanner/).
```

---

## 5. User Journeys

### 5.1 First-Time Setup

1. **Deploy Loom** — run Docker container or binary.
2. **Setup wizard** — browser redirects to `/setup` on first visit.
3. **Create account** — set username and password.
4. **Add a library** — Settings → Libraries → Add → browse filesystem → set path and media type.
5. **Add a download client** — Settings → Download Clients → Add → configure qBittorrent/Transmission/etc → test connection.
6. **Add indexers** — Indexers page → Add from catalogue → enter API key → test.
7. **Create a quality profile** — Quality Profiles page → create profile → set allowed qualities and cutoff.
8. **Add media** — Movies or Series page → search TMDB → add to library.
9. **Verify** — Dashboard should show counts; trigger a manual search to verify the pipeline works.

### 5.2 Adding a Movie and Getting It Downloaded

1. Navigate to **Movies** page.
2. Click **Add Movie**.
3. Search by title → select from TMDB results.
4. Choose library, quality profile, and whether to monitor.
5. Optionally check "Search on add" to immediately trigger search.
6. Movie appears in list as "Missing" (yellow) or "Searching" if auto-search triggered.
7. **If auto-search:** workflow created → searches indexers → grabs best result → sends to download client → monitor detects completion → import moves file to library → status becomes "Available" (green).
8. **If manual search:** click movie → click Search → view results → click Grab on preferred release → same flow from step 7.
9. Check **Workflows** page to monitor pipeline progress.
10. Check **Activity** page to see download progress in client.

### 5.3 Adding a TV Series

1. Navigate to **Series** page.
2. Click **Add Series**.
3. Search by title → select from TMDB results.
4. Choose library, quality profile, monitoring level (all, future, missing, none).
5. Series created with all seasons/episodes.
6. Monitored episodes eligible for automated search.
7. Same download pipeline as movies, but per-episode.
8. Season packs may satisfy multiple episodes at once.

### 5.4 Setting Up Import Lists for Hands-Free Library Building

1. Navigate to **Import Lists** page.
2. Click **Add Import List**.
3. Choose provider (e.g., Trakt Watchlist).
4. Configure credentials:
   - Trakt: uses OAuth from Connect integration (must be set up first).
   - IMDb: list URL.
   - TMDb: API key auto-filled from config.
5. Set target library and quality profile.
6. Enable/disable auto-search on add.
7. Save → list syncs automatically every 60 seconds.
8. New items appear in library.
9. Use **Exclusions** tab to prevent specific items from being re-added.

### 5.5 Connecting Plex / Trakt for Watched-Status Archiving

**Plex:**
1. Settings → Connect → Add → Plex.
2. Enter Plex host URL and API token.
3. Test connection.
4. When media is imported, Loom sends a library refresh to Plex.

**Trakt:**
1. Settings → Connect → Add → Trakt.
2. Enter Trakt API application Client ID and Client Secret.
3. Click "Save & Authorize" → new tab opens to Trakt.
4. Approve on Trakt → redirected back to Loom.
5. Code auto-populates → click "Complete Authorization."
6. Use sync buttons: Sync Watched, Sync Collection, Sync Watchlist.
7. Enable "Auto-archive watched" on libraries → watched movies/shows are automatically unmonitored/archived after configurable delay.

### 5.6 Monitoring and Troubleshooting Downloads

1. **Activity** page shows all active downloads across all clients.
   - Progress bars, speeds, ETA.
   - Actions: pause, resume, remove, set priority, speed limit.
2. **Workflows** page shows pipeline state for each search operation.
   - States: searching → grabbed → downloading → importing → completed.
   - Actions: cancel, retry, delete.
3. **Workflow detail** page shows timeline of events for a single workflow.
   - **Logs tab** shows all application-level log entries correlated to this workflow.
   - For active workflows, logs stream in real-time via SSE.
4. **Settings → System** page shows the full application log stream.
   - Live mode: real-time SSE streaming with pause/resume.
   - History mode: paginated search with level and text filters.
   - Capture level can be temporarily lowered to `debug` for deep troubleshooting.
5. **Indexer Health** page shows per-indexer success rates and latency.
6. **Search Query Log** shows every search sent to indexers with timing.
7. **Download History** shows past completed/failed downloads.
8. **Blocklist** shows releases that have been blocked from future grabs.

**Common troubleshooting steps:**
- Workflow stuck at "grabbed" → download client may not be receiving the item. Check client connectivity. Check workflow Logs tab for error details.
- Workflow stuck at "downloading" → check Activity page for download progress. May be stalled.
- Workflow fails at "importing" → check remote path mappings, file permissions, disk space. Workflow Logs tab will show the import error.
- No search results → check indexer health, verify indexer has the content category.
- Unexpected behaviour → open **Settings → System**, lower capture level to `debug`, reproduce the issue, and review the log stream.

### 5.7 Quality Upgrades Over Time

1. Set up a quality profile with cutoff at e.g., Bluray-1080p.
2. Add media — initial grab may be HDTV-720p (best available).
3. Rolling search periodically re-checks monitored media.
4. When a Bluray-1080p release appears, autosearch evaluates it as an upgrade.
5. New release grabbed → downloaded → imported (replacing old file).
6. "Upgrade" notification sent.
7. Once cutoff quality reached, no more searches for that item.
8. Custom format scores can further refine: prefer DV/HDR, avoid certain groups, etc.

---

## 6. Known Limitations & Incomplete Features

| Area | Status | Notes |
|------|--------|-------|
| **End-to-end download→import pipeline** | ⚠️ Unreliable | The post-download and import stages have known issues. Workflows may not complete successfully. This is the current development focus (Phase 3). |
| **Grab cleanup on import** | 🔴 Incomplete | Active grab records may not be properly cleaned up after import. |
| **Remote path mappings** | ⚠️ Partial | Feature exists but may not be fully integrated into the import pipeline. |
| **Blocklist on failure** | ⚠️ Partial | Failed releases should be auto-blocklisted for redownload, but this may not work end-to-end. |
| **Season pack import** | ⚠️ Partial | Pack detection exists but unpacking/splitting into episodes may have gaps. |
| **Anime episode mapping** | ⚠️ Partial | Absolute-to-season mapping exists but coverage depends on external data. |
| **Wire-compat parity** | 🔴 Incomplete | Compatibility shims exist but don't cover all Radarr/Sonarr/Prowlarr endpoints. |
| **Migration tooling** | 🔴 Not started | No tool to import existing Radarr/Sonarr databases. |
| **Backup/restore** | 🔴 Not started | No CLI tool for backup/restore yet. |
| **Kubernetes/split deployment** | 🔴 Not started | Phase 11 goal. |
| **Secrets encryption at rest** | 🔴 Not started | API keys and tokens stored as plaintext in DB. |
| **Advanced RSS sync** | ⚠️ Partial | Basic RSS intake exists but full automated grab-on-match may have gaps. |

---

*This document reflects the codebase as of May 2025. Loom is under active development.*
