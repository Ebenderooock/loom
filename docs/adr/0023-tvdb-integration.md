# ADR-0023: TheTVDB (TVDB) Integration

**Status:** Accepted  
**Date:** 2025-05-11  
**Authors:** Copilot  

## Context

Loom needs to support TV series and episode metadata retrieval. Phase 4a established the metadata provider abstraction (`MetadataProvider` interface). Phase 4b (TMDb) provides movie and TV metadata. Phase 4c adds **TheTVDB (TVDB)** as a dedicated TV-series-focused provider.

### Why TVDB?

- **TV-centric:** TVDB specializes in TV series metadata; better episode data than TMDb
- **Complementary:** TVDB and TMDb have different coverage; combined they reduce "no result" failures
- **Episode precision:** TVDB natively supports season/episode lookups with episode TVDB IDs
- **Industry standard:** Widely used in media centers (Kodi, Plex, Sonarr, Radarr)

## Decision

Implement a standalone TVDB client at `internal/metadata/tvdb` that:

1. **Uses TVDB v4 API** (https://api4.thetvdb.com/v4) with JWT authentication
2. **Implements `MetadataProvider` interface** for series and episode lookups
3. **Handles JWT tokens** (24-hour lifetime; refresh on 401)
4. **Implements exponential backoff** for rate limiting (HTTP 429)
5. **Provides non-blocking fallback** to other providers if TVDB unavailable

## Rationale

### JWT Authentication (vs. Static Bearer Tokens)

TVDB v4 uses session-based JWT tokens issued on `/login`, not static API keys. This design:
- Allows per-user rate limits (users can have their own PIN for higher limits)
- Provides session isolation (each login is a new session)
- Requires token refresh on 401 (transparent re-login)

**Tradeoff:** Adds complexity (login call, token refresh) but provides better rate limit isolation than static tokens.

### Separation from TMDb

- **TVDB** → TV series + episodes (primary use case)
- **TMDb** → Movies + TV series (both supported)

Separate clients allow:
- TVDB-specific auth (PIN for rate limits)
- TVDB-specific error handling (e.g., episode ID requirement)
- Independent configuration and testing
- Potential future: different cache TTLs, retry strategies

### Rate Limiting Strategy

TVDB enforces rate limits:
- **Free tier:** 30 requests per 10 seconds (180/min)
- **With PIN:** Higher limits

Implementation:
- Exponential backoff: 1s → 2s → 4s → 8s → 16s → 32s → 60s
- Respect `Retry-After` header
- Add jitter (±10%) to prevent thundering herd
- Log rate limits with context

### Pagination

TVDB search results paginate via `page` query parameter. Implementation:
- Fetch first page by default
- Return all results in first batch (common case: <20 results)
- Optional pagination for larger result sets (future enhancement)

### Error Handling

Typed errors distinguish:
- `ErrCodeNotFound` (404): Series/episode doesn't exist
- `ErrCodeUnauthorized` (401): Invalid credentials or token expired
- `ErrCodeRateLimit` (429): Rate limited; includes retry delay
- `ErrCodeServerError` (5xx): TVDB service unavailable
- `ErrCodeClientError` (4xx): Invalid request
- `ErrCodeNetworkError`: I/O or timeout
- `ErrCodeContextError`: Context cancelled/timeout

**Token Refresh:** On 401, client automatically calls `Login()` to refresh JWT. If re-login also fails (401), error is returned (do not retry infinitely).

### Concurrency

Client uses `sync.Mutex` to synchronize token refresh:
- Only one goroutine re-logins at a time
- Other goroutines wait for token to be refreshed
- Prevents race conditions and duplicate login requests

## Implementation

### Package Structure

```
internal/metadata/tvdb/
  ├── doc.go              # Package documentation
  ├── types.go            # TVDB response types (JSON)
  ├── errors.go           # Typed error definitions
  ├── client.go           # HTTP client with JWT auth
  ├── mapper.go           # JSON → MetadataProvider types
  ├── client_test.go      # Client tests (16+ cases)
  └── mapper_test.go      # Mapper tests
```

### Configuration

Env vars (from config package):
- `LOOM_METADATA_TVDB_APIKEY` — TheTVDB API key (required)
- `LOOM_METADATA_TVDB_PIN` — Personal PIN (optional; for higher rate limits)

### Supported Endpoints

**GET /series/{id}** — Retrieve series by TVDB ID
```go
series, err := client.GetSeries(ctx, 81189) // Breaking Bad
```

**GET /search** — Search for series by title + year
```go
results, err := client.SearchSeries(ctx, "Breaking Bad", 2008)
```

**GET /episodes/{id}** — Retrieve episode by TVDB episode ID
*(Not directly exposed; requires two-step lookup: list episodes, then get specific episode)*

### Limitations

1. **No episode lookup by season/episode number** — TVDB v4 doesn't support this directly
   - Workaround: Requires episode TVDB ID (future: fetch episode list first)

2. **No movie support** — TVDB is TV-focused; `FindMovie()` returns error
   - Use TMDb for movies

3. **Partial data** — Some fields may be missing (IMDb ID, poster image)
   - Client returns partial metadata; callers handle gracefully

## Testing

16+ tests covering:
- **Authentication:** Login success, invalid credentials (401)
- **GetSeries:** Happy path, not found (404), server error (500)
- **SearchSeries:** Multiple results, empty results, pagination metadata
- **Error handling:** 401 triggers re-login, 429 triggers backoff
- **Token refresh:** 401 response refreshes JWT transparently
- **Rate limiting:** Exponential backoff with Retry-After header
- **Concurrency:** 10 concurrent requests (race-safe)
- **Mapping:** JSON → struct conversions, year extraction, type filtering

All tests pass `-race` flag (race condition detection).

## Migration

**Phase 4c:** TVDB client implementation (completed)
**Phase 5+:** 
- Integration with service.go (register TVDB as provider)
- Configuration loading (env vars)
- Optional: Redis caching for horizontal scale
- Optional: Bulk metadata import workflows

## Related ADRs

- **ADR-0021:** Metadata abstraction layer (MetadataProvider interface)
- **ADR-0024:** MusicBrainz integration (parallel metadata provider)

## Appendix: TVDB v4 API References

- Swagger/OpenAPI: https://api4.thetvdb.com/swagger
- Authentication: /login endpoint with API key + PIN
- Series: /series/{id}
- Search: /search?query=...&type=series&year=...
- Episodes: /episodes/{id}
- Rate limits: 30 req/10s (free), higher with PIN
