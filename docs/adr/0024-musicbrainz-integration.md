# ADR 0024: MusicBrainz Integration for Music Metadata

## Status
Accepted

## Context

Loom's metadata service (Phase 4) needed a provider for music metadata (artists, albums, recordings).
MusicBrainz is the leading open music encyclopedia with comprehensive, community-maintained data.

### Why MusicBrainz?

1. **Comprehensive Coverage**: 45M+ artists, 15M+ releases, 110M+ recordings
2. **Open & Free**: No authentication required, free API access
3. **Community Maintained**: Best-in-class data quality from collaborative submissions
4. **APIs & Standards**: RESTful JSON API, Lucene query syntax, standard music taxonomy
5. **Non-Blocking**: If MusicBrainz unavailable, other providers can fulfill requests

### Design Constraints

- **Separate from Movie/TV**: Music metadata is structurally different (artists, releases, recordings vs. movies, episodes)
- **Non-Blocking**: Service continues if MusicBrainz fails; caching and fallbacks handle outages
- **Rate Limiting**: MusicBrainz enforces 1 req/sec; we respect this with a throttler
- **User-Agent Required**: MusicBrainz requires descriptive User-Agent (polite clients policy)

## Decision

Implement `internal/metadata/musicbrainz` package with:

1. **HTTP Client** (`client.go`)
   - Methods: `GetArtist()`, `GetRelease()`, `GetRecording()`, `SearchArtist()`, `SearchRelease()`
   - Rate limit: 1s throttler between consecutive requests (sync.Mutex-based)
   - Exponential backoff for 429: 1s → 60s (2x multiplier, max 5 retries)
   - User-Agent: "Loom/1.0 (metadata_service)"

2. **Request Throttler** (`client.go` → `Throttler`)
   - Enforces minimum 1s delay between all requests
   - Concurrent-safe with sync.Mutex
   - Allows first request immediately (last_req initialized to time.Now() - interval)

3. **Error Handling**
   - Typed errors: `ClientError` with `ErrorCode` enum
   - Codes: `ErrCodeNotFound`, `ErrCodeRateLimit`, `ErrCodeServerError`, `ErrCodeClientError`, `ErrCodeNetworkError`, `ErrCodeContextError`
   - Error messages: "musicbrainz: {code} (HTTP {status}): {message}"

4. **JSON Mappers** (`mapper.go`)
   - `MapArtist()`, `MapRelease()`, `MapRecording()`
   - Extract: MBID, name, disambiguation, area, tracks, artists, year, duration

5. **Response Types** (`types.go`)
   - `ArtistResponse`, `ReleaseResponse`, `RecordingResponse`, `SearchResponse`
   - Supports MusicBrainz JSON API v2 format

## Rate Limiting Strategy

### 1s Throttler
- **Why**: MusicBrainz default rate limit is 1 req/sec
- **Implementation**: `Throttler` struct with sync.Mutex and `time.Sleep()`
- **Concurrent Safety**: Serializes all requests; no concurrent API calls

### Exponential Backoff (429 Responses)
- **Initial**: 1 second
- **Multiplier**: 2x per retry
- **Max**: 60 seconds
- **Max Retries**: 5
- **Sequence**: 1s → 2s → 4s → 8s → 16s → 32s (→ capped at 60s)
- **Retry-After Header**: Parsed and respected if present

### Other Retryable Status Codes
- 503 Service Unavailable: Retried with exponential backoff
- 504 Gateway Timeout: Retried with exponential backoff
- Network errors: Retried with exponential backoff

### Non-Retryable Status Codes
- 404 Not Found: Return immediately
- 401/403 Unauthorized: Return immediately (no auth in MusicBrainz)
- Other 4xx: Return immediately

## User-Agent Header

**Requirement**: MusicBrainz API requires a descriptive User-Agent to identify clients.

**Implementation**:
- Set on every request: `User-Agent: Loom/1.0 (metadata_service)`
- Documented in package `doc.go` and error messages

**Rationale**: Polite clients help MusicBrainz ops team monitor and understand traffic patterns.

## Concurrent Safety

- Client is safe for concurrent use
- Throttler uses sync.Mutex to serialize requests
- HTTP client (via net/http) is concurrency-safe
- All internal state (config, httpClient, throttler) is immutable after construction

## Testing

- 28 tests covering:
  - Happy path: `GetArtist()`, `GetRelease()`, `GetRecording()`, `SearchArtist()`, `SearchRelease()`
  - Errors: 404, 5xx, context timeout, network errors
  - Rate limiting: 429 with backoff
  - Throttling: 1s minimum delay verification
  - Concurrency: 5 concurrent requests (race-safe)
  - User-Agent verification: Validates header on every request
  - Mappers: Nil handling, multi-artist, pagination
- All tests pass with `-race` flag

## Alternatives Considered

1. **Genius API**: Music-focused, but smaller catalog and authentication required
2. **Spotify API**: Excellent, but authentication and rate limits stricter
3. **Last.fm**: Smaller catalog, less open

**Choice**: MusicBrainz best fits Loom's open-source, non-blocking model.

## Consequences

### Positive
- Users get music metadata without external API keys
- Loom respects MusicBrainz rate limits gracefully
- Service remains available if MusicBrainz times out
- Clear error codes help debugging

### Negative
- 1s throttle means music metadata requests are serialized (not concurrent)
- Rate limits prevent high-throughput scenarios
- MusicBrainz API is sometimes slow (5-10s responses on complex queries)

**Mitigation**: Caching (7d TTL) masks rate limit impact for repeated queries.

## References

- MusicBrainz API v2: https://musicbrainz.org/development/mmd
- MusicBrainz Rate Limits: https://musicbrainz.org/doc/XML_Web_Service
- User-Agent Policy: https://musicbrainz.org/doc/HTTP_User-Agent
