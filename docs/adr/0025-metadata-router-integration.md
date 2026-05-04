# ADR 0025: Metadata Router Integration

**Status:** Accepted  
**Decision Date:** 2026-05-04  
**Author:** Copilot  
**Affects:** Phase 4e  

## Problem

Phases 4a–4d delivered metadata providers (TMDB, TVDB, MusicBrainz), but they operated in isolation:
- **No orchestration layer** to fan-out queries to multiple providers in parallel
- **No unified resolution strategy** (by-ID vs. search fallback)
- **No integration with downloads pipeline** to enrich download events with metadata
- **No timeout enforcement** at the orchestration level (only per-provider)

Downloads router had no way to:
1. Enrich a queued download with movie/series metadata
2. Signal success (TopicMetadataEnriched) or failure (TopicMetadataFailure) to downstream consumers

## Decision

Build a **metadata router** (Phase 4e) that:

### 1. Fan-out Pattern (Concurrent Providers)
- Call all providers **concurrently** using `errgroup.Group`
- Why: Reduces latency when one provider is slow; fast provider's result is returned immediately
- Avoids sequential bottleneck (old Service model was OK for single queries, not orchestration)
- Example: TMDB responds in 100ms, TVDB in 2s → return after 100ms, not 2.1s

### 2. Total Timeout per Resolve (not per-provider)
- **Single 10-second timeout** for entire fan-out
- Each provider gets 3-second individual timeout (context.WithTimeout)
- If provider 1 finishes in 100ms with a result, we return immediately
- If all providers timeout/fail, return error after remaining time expires
- Why: Predictable latency (clients always know 10s max); prevents slow providers from blocking

### 3. Graceful Degradation
- If provider N fails → try provider N+1 (don't stop)
- If all providers fail → return last error
- If first provider succeeds → return that result (don't wait for others)
- Partial results acceptable (e.g., TMDB returns movie, TVDB missing details)

### 4. Integration with Downloads Router
After successfully queuing a download:
1. Call `MetadataRouter.ResolveMovie()` or `ResolveSeries()` in **background goroutine**
2. On success → emit `TopicMetadataEnriched` event (includes source Result + matched metadata)
3. On failure → emit `TopicMetadataFailure` event (includes source Result + reason)
4. **Non-blocking**: enrichment doesn't hold up download routing (fire-and-forget)
5. Event bus subscribers can update search indexes, release metadata, etc.

### 5. Event Propagation Strategy
```
IndexerResult → Downloads Router routes to client → triggers background metadata enrichment
                                                   → (success) TopicMetadataEnriched → consumers
                                                   → (failure) TopicMetadataFailure → consumers
```

Consumers (e.g., releases service, search indexer):
- **TopicMetadataEnriched**: "This download acquired metadata; update release record"
- **TopicMetadataFailure**: "This download lacks metadata; mark as needs-review or quarantine"

## Rationale

### Why Fan-out vs. Sequential?
- **Fan-out (chosen)**: 3 providers at 1s each = ~1s total (all start at t=0)
- **Sequential**: 3 providers at 1s each = 3s total (start at t=0, t=1, t=2)
- Higher throughput; lower user latency

### Why Timeout per Resolve, not per Provider?
- **Per-resolve (chosen)**: Clients know max latency (10s guaranteed)
- **Per-provider**: Total latency = (# providers × timeout); unpredictable
- Prevents cascading slowness across the system

### Why Background Enrichment?
- Download routing should complete immediately (queue to client)
- Metadata lookup can be slow (3–10s total) and may fail
- Non-blocking design decouples download success from metadata availability
- Allows clients to start downloading while enrichment happens

### Why Two Event Topics?
- **TopicMetadataEnriched**: "We have metadata; use it"
- **TopicMetadataFailure**: "We don't have metadata; handle gracefully"
- Subscribers can distinguish success vs. failure at the event level (no need to poll status)

## Implementation

### Router Service
```go
type Router struct {
    service *Service  // Accesses providers + cache + DB
}

func (r *Router) ResolveMovie(ctx, title, year, externalIDs) (*MovieMetadata, error) {
    // 1. Try external ID lookups (TMDB, IMDB, TVDB)
    // 2. Fall back to title+year search if no ID match
    // Uses Service's existing cache → DB → provider hierarchy
}

func (r *Router) ResolveSeries(ctx, title, externalIDs) (*SeriesMetadata, error) {
    // Similar to ResolveMovie
}

func (r *Router) ResolveEpisode(ctx, seriesID, season, episode) (*EpisodeMetadata, error) {
    // Concurrent provider queries with 10s total timeout
    // Returns first success or nil on timeout/all-fail
}
```

### Events
- `TopicMetadataEnriched` + `MetadataEnrichedEvent`
- `TopicMetadataFailure` + `MetadataFailureEvent`
- Both carry source Result GUID for traceability

### Downloads Router Enhancement
```go
func (r *Router) handleIndexerResult(ctx eventbus.Event) {
    // ... route to download client ...
    // On success:
    go r.enrichMetadata(result, downloadID)  // Non-blocking
}

func (r *Router) enrichMetadata(result, downloadID) {
    // 15s overall timeout (router timeout + overhead)
    movie, _ := r.metadataRouter.ResolveMovie(ctx, ...)
    if movie != nil {
        r.bus.Publish(ctx, &MetadataEnrichedEvent{...})
        return
    }
    series, _ := r.metadataRouter.ResolveSeries(ctx, ...)
    if series != nil {
        r.bus.Publish(ctx, &MetadataEnrichedEvent{...})
        return
    }
    // No match
    r.bus.Publish(ctx, &MetadataFailureEvent{Reason: "no match"})
}
```

### Configuration
```go
type Config struct {
    Providers    []string      // ["tmdb", "tvdb", "musicbrainz"]
    Timeout      time.Duration // 10s
    CacheEnabled bool
}
```

Env vars:
- `LOOM_METADATA_PROVIDERS`: comma-separated provider list
- `LOOM_METADATA_TIMEOUT`: duration string (e.g., "10s")

## Constraints

1. **Non-blocking orchestration**: Router doesn't hold up event bus (metadata enrichment deferred)
2. **10s timeout**: Hardcoded at resolve time; config allows override per deployment
3. **No retry strategy**: Phase 5+ can add exponential backoff if needed
4. **No scoring**: All providers treated equally; Phase 5+ can implement "prefer TMDB" logic

## Alternatives Considered

### 1. Sequential Provider Queries
- ❌ Slower (sum of provider timeouts)
- ❌ Unpredictable latency

### 2. Blocking Enrichment in Router
- ❌ Slows down download routing
- ❌ Ties metadata availability to download success

### 3. Single Event Topic with Status Field
- ❌ Subscribers must poll/inspect event payload to determine success
- ✅ Two topics (Enriched vs. Failure) is explicit

## Testing

- 10+ tests covering resolve by ID, search fallback, no match, timeout, partial results
- Concurrent resolve calls (race-safe with `errgroup.Group` + `sync.Mutex`)
- All tests pass with `-race` flag
- Download router integration: verify enrichment triggered after successful queue

## Deployment Notes

- Default providers: tmdb, tvdb, musicbrainz (configurable)
- Default timeout: 10 seconds (configurable)
- No breaking changes to Service or Provider interfaces
- Event bus must support async handlers (already does)

## References

- **Phase 4a–4d**: Metadata providers (TMDB, TVDB, MusicBrainz)
- **Phase 3**: Downloads router (queue to clients)
- **Phase 5+**: Provider scoring, retry strategies, Redis caching

---

**Approved by:** Copilot  
**Implementation Date:** 2026-05-04
