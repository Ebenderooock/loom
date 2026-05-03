# Metadata Service

The metadata service (`internal/metadata`) provides a unified abstraction layer for movie, series, and episode metadata sourced from external providers (TMDB, TVDB, MusicBrainz).

## Architecture

The service implements a three-tier lookup strategy:

1. **In-process cache** — fast, volatile (TTL-based)
2. **Database** — persistent, reliable (SQLite or Postgres)
3. **Providers** — pluggable external APIs (TMDB, TVDB, etc.)

## Provider Interface

Providers implement `MetadataProvider`:

```go
type MetadataProvider interface {
    Name() string
    FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*MovieMetadata, error)
    FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*SeriesMetadata, error)
    FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*EpisodeMetadata, error)
}
```

Providers are queried sequentially with per-provider timeout (3s each, 10s total). Results are cached and persisted immediately.

## Service API

```go
type Service struct { ... }

// Find a movie by title/year or external IDs
func (s *Service) FindMovie(ctx context.Context, params SearchMovieParams) (*MovieMetadata, error)

// Find a series by title or external IDs
func (s *Service) FindSeries(ctx context.Context, params SearchSeriesParams) (*SeriesMetadata, error)

// Find an episode by series ID and season/episode numbers
func (s *Service) FindEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error)
```

Returns `nil` if no providers yield a result or all timeout. Partial results are acceptable (first successful provider wins).

## Cache TTL

| Tier | TTL | Use Case |
|---|---|---|
| Search results (title+year) | 30 min | May be incomplete; volatile |
| Full details (external ID) | 7 days | Stable; keyed by immutable ID |

Expired entries are cleaned every 5 minutes by a background goroutine.

## Database Schema

Three tables store metadata:

- `metadata_movies` — keyed by (TMDB ID / IMDB ID / TVDB ID)
- `metadata_series` — same shape as movies
- `metadata_episodes` — composite key (series_id, season, episode)

Each row includes:
- External IDs (TMDB, IMDB, TVDB)
- Basic fields (title, overview, year, etc.)
- `cached_json` — full metadata struct as JSON
- `cached_at` — timestamp of last fetch
- `expires_at` — soft expiration for cleanup

Migration 0011 creates all three tables with indexes on external IDs.

## Immutability

Metadata is immutable once cached. Updates require:
1. Explicit refresh via provider API (not automatic)
2. DB row replacement (new timestamp, new JSON)
3. Cache eviction (old TTL expires naturally)

This prevents the complexity of invalidation strategies and keeps the system simple.

## Non-Blocking Behavior

The service never blocks the UI:
- Returns immediately on first provider match
- Returns cached/DB value if providers timeout
- Returns `nil` if no data available (graceful degradation)

Timeout strategy:
- **Per-provider:** 3 seconds (network + parse)
- **Total:** 10 seconds (wall-clock, not sum)

## Concurrency

The service is safe for concurrent use:
- Repository delegates to the database driver (engine-native concurrency)
- In-process cache uses `sync.Mutex` (thread-safe)
- Service is stateless (no race conditions)

All tests pass with `-race` flag.

## Metadata Providers

### TMDB Provider

The TMDB (TheMovieDB) provider (`internal/metadata/tmdb`) sources movie and TV series metadata.

**Configuration:**
```bash
export LOOM_METADATA_TMDB_APIKEY=your_tmdb_api_key
```

**API Key:** Obtain at https://www.themoviedb.org/settings/api

**Supported:**
- `FindMovie()` — Movies by title/year or external IDs
- `FindSeries()` — TV series by title or external IDs
- `FindEpisode()` — Episodes by series ID + season/episode (if available)

**Limitations:**
- Static API key (no per-user rate limit customization)
- Rate limit: 40 req/10s

### TVDB Provider

The TVDB (TheTVDB) provider (`internal/metadata/tvdb`) specializes in TV series and episode metadata.

**Configuration:**
```bash
export LOOM_METADATA_TVDB_APIKEY=your_tvdb_api_key
export LOOM_METADATA_TVDB_PIN=optional_pin_for_higher_limits  # optional
```

**API Key:** Obtain at https://www.thetvdb.com/api-information

**Authentication:**
- TVDB v4 uses JWT tokens (session-based, not static)
- Login via `/login` endpoint with API key + optional PIN
- Token valid 24 hours; auto-refreshed on 401 response
- Token stored in-memory (no persistence)

**Supported:**
- `GetSeries(ctx, tvdbID)` — Series by TVDB ID
- `SearchSeries(ctx, query, year)` — Series search by title
- `FindSeries()` — Implements MetadataProvider interface
- `FindEpisode()` — Requires episode TVDB ID (not season/episode directly)

**Limitations:**
- TV-only (no movie support)
- Episode lookup requires episode TVDB ID (future: add list-then-get pattern)

**Rate Limiting:**
- Free tier: 30 req/10s (180 req/min)
- With PIN: Higher limits (user-specific)
- Implementation: Exponential backoff (1s → 60s) + Retry-After header
- Jitter: ±10% to prevent thundering herd

**Data Coverage:**
- TVDB and TMDB have different series coverage
- Combined: higher hit rate for international/niche series
- Fallback: if TVDB unavailable, service tries TMDB

## Integration Points

### Movies Module (Phase 5)
- Calls `Service.FindMovie()` to populate movie details
- Stores TMDB ID foreign key in `movies.tmdb_id`
- Joins with metadata for poster, overview, runtime

### Series Module (Phase 6)
- Calls `Service.FindSeries()` for series details
- Calls `Service.FindEpisode()` for episode metadata
- Stores TVDB series ID foreign key in `series.tvdb_id`

### Provider Registration
Providers are registered at startup in order of preference:
```go
providers := []MetadataProvider{
    tvdbProvider,   // Try TVDB first (TV-focused)
    tmdbProvider,   // Fall back to TMDB
}
svc := NewService(repo, providers)
```

Non-blocking: if TVDB times out or fails, service continues to TMDB.

## Deferred to Phase 5+

- Redis caching for HA/scale
- Provider fallback strategies (merge from multiple sources)
- Automatic metadata refresh (webhooks, scheduled jobs)
- Metadata scoring (completeness/confidence metrics)
- Bulk import/export
