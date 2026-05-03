# ADR 0021: Metadata Abstraction Layer

**Status:** Accepted (Phase 4a)

**Date:** 2025-05-14

**Authors:** Loom Dev

## Context

Loom needs to fetch, cache, and serve movie/series metadata from external providers (TMDB, TVDB, MusicBrainz) to support the Movies and Series modules (Phases 5–6). The abstraction must:

- Support multiple external providers in a pluggable fashion
- Implement a tiered caching strategy (fast, reliable, offline-capable)
- Handle provider timeouts gracefully without blocking the UI
- Work equally well with SQLite and Postgres
- Be read-heavy (metadata fetched once, read 1000s of times)

## Decision

We implement a three-tier lookup strategy:

1. **In-process cache** (memory, TTL-based)
   - Search results: 30 minutes (volatile, may be incomplete)
   - Full details: 7 days (stable, keyed by external ID)
   - Cleaned every 5 minutes by background goroutine

2. **SQL repository** (persistent, engines: SQLite + Postgres)
   - `metadata_movies(id, tmdb_id, imdb_id, tvdb_id, title, cached_json, expires_at, ...)`
   - `metadata_series(...)` — same shape for TV series
   - `metadata_episodes(series_id, season, episode, tvdb_id, ...)` — composite key
   - Append-mostly; old rows expire and are cleaned asynchronously
   - JSON columns store full metadata for fast retrieval

3. **Provider layer** (external APIs, pluggable)
   - `MetadataProvider interface` with `FindMovie()`, `FindSeries()`, `FindEpisode()`
   - Queried sequentially left-to-right with per-provider timeout (3s each, 10s total)
   - Providers are registered at startup; new providers drop in without code changes

### Why This Design

- **Fast path:** Most requests hit in-process cache (cold start < 1ms)
- **Resilience:** DB serves stale data when providers are down/slow
- **Scale:** In-process cache is bounded; DB is bounded by storage; no external caching service (Phase 5+)
- **Flexibility:** Providers are swappable; new metadata sources added in parallel modules
- **Immutability:** Cached metadata is never invalidated automatically; updates require explicit refresh
- **Non-blocking:** Service returns immediately on first match or timeout; UI never waits for network

## Alternatives Considered

1. **Single provider, no cache**
   - ❌ Unacceptable; external APIs are slow and rate-limited

2. **Redis + in-process cache**
   - ❌ Deferred to Phase 5; adds operational complexity (new service to run)

3. **Cache invalidation on updates**
   - ❌ Rejected; immutability is safer; explicit refresh is clearer

4. **Provider fallback strategies**
   - ❌ Deferred to Phase 5; scoring/merging results is complex

## Cache TTL Table

| Lookup Type | TTL | Reason |
|---|---|---|
| Search by title+year | 30 min | May be incomplete; providers change results |
| Search by external ID | 30 min | Same; ID lookups may return partial data |
| Full details (cached from provider) | 7 days | Stable; external IDs are immutable |
| DB read (no provider) | Infinite | Already cached; DB is source of truth |

## Timeout Strategy

- **Per-provider:** 3 seconds (generous for network + parse)
- **Total:** 10 seconds (wall-clock, not sum of providers)
- **Behavior:** Return first successful result or nil; never block
- **Partial results:** If provider 1 succeeds but provider 2 times out, return provider 1's result

## Concurrency

- Repository: Backed by DB (engine-native concurrency)
- Cache: `sync.Mutex` (simple, safe for few thousand entries)
- Service: Stateless for concurrent use (repo is the bottleneck, not the service)

## Deferred to Phase 5+

- Redis caching for horizontal scale (HA)
- Provider fallback strategies (merge results from multiple providers)
- Cached metadata refresh webhooks or scheduled jobs
- Metadata scoring (confidence/completeness metrics)
- Bulk metadata import/export
- Cache invalidation strategies

## Implementation Notes

- Migration 0011 creates the three tables with proper indexes on external IDs
- Repository is engine-neutral; SQLite/Postgres adapters dispatch to raw SQL
- Service is stateless except for the shared cache; Thread-safe
- No sqlc generation needed; raw SQL is sufficient and simpler
- Metadata is stored as JSON in the database for flexibility and speed

## Testing

- Cache TTL enforcement (manual cleanup + background goroutine)
- Repository CRUD (SQLite only; Postgres mirrors the pattern)
- Service lookup chain (cache → DB → providers)
- Provider timeout and partial results
- Race conditions (10 concurrent lookups)

All tests pass with `-race` flag.
