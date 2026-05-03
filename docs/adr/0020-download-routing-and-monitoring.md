# ADR 0020: Download Routing & Monitoring

**Status:** ACCEPTED

**Date:** 2026-05-04

**Context:**

Loom's intake pipeline (Phases 1–3a) detects new releases via indexer polling and filters by quality. Once a high-quality result is identified, it must be queued on a download client (qBittorrent, Transmission, SABnzbd, Usenet client, NZBGet) and tracked until completion. The system must support multiple concurrent clients, handle failures gracefully, and emit events that downstream phases (release acquisition, dashboard) can subscribe to.

**Decision:**

Implement two interconnected services that form the download orchestration layer (Phase 3g):

1. **Router Service**
   - Listens to indexer result events (TopicIndexerResult) from the event bus
   - Applies quality filtering (simple seeder-based heuristics for Phase 3; full semantic rules deferred to Phase 5)
   - Attempts to queue results on available download clients in priority order
   - Emits `TopicDownloadQueued` on success or `TopicDownloadFailed` on error
   - Non-blocking design: runs synchronously in the event publisher's goroutine for immediate feedback
   - Early return (no event) if result fails quality filter; DownloadFailureEvent if no clients configured

2. **Monitor Service**
   - Runs periodic status sweeps across all registered download clients
   - Tracks which items have been emitted as completed to avoid duplicate events
   - Emits `TopicDownloadCompleted` for newly-completed items
   - Integrates with scheduler for periodic invocation (deferred to kernel phase)
   - Standalone design: `Run()` is a scheduler-compatible handler, not a persistent goroutine

3. **Event Model**
   - Three new topics: `TopicDownloadQueued`, `TopicDownloadFailed`, `TopicDownloadCompleted`
   - `IndexerResultEvent` wrapper adapts `indexers.Result` to the eventbus.Event interface
   - Events carry origin result IDs, client IDs, download IDs, and timestamps for traceability

**Quality Filtering (Phase 3)**

Simplified seeder-based filtering:
- Reject torrents with 0 seeders (Seeders == 0)
- Accept Usenet results (Seeders == nil)
- Accept torrents with >0 seeders (Seeders > 0)

Full semantic filtering (resolution, codec, release groups, language) is deferred to Phase 5.

**Client Selection**

Clients are selected by:
1. Registry priority ordering (lower priority value first)
2. Fallback to any available client if first fails
3. Early termination on first successful queue

**Request Precedence (buildAddRequest)**

Converts `indexers.Result` to `downloads.AddRequest`:
1. Prefer magnet URI (if provided)
2. Fall back to infohash (construct `magnet:?xt=urn:btih:{hash}`)
3. Fall back to Link as TorrentURL

**Known Limitations**

- Item struct returned by Status() does not carry ClientID. Completion events currently have empty ClientID. This will be resolved in a future phase when Item is extended or when Status results are grouped by client at the Registry level.
- Monitor receives a flat list of completed items from all clients without per-client grouping. Duplicate prevention relies on item ID uniqueness across clients, which is assumed to hold for the current client implementations.
- HTTP handler for manual completion (POST /api/downloads/{id}/complete) is deferred to Phase 3h.

**Alternatives Considered**

1. **Blocking queue with separate workers**: Would add latency to the indexer pipeline; rejected for Phase 3 simplicity.
2. **Persistent monitor goroutine**: Would complicate lifecycle management; scheduler integration is cleaner.
3. **Per-client tracking in Monitor**: Item struct lacks ClientID; flat tracking is sufficient for current client implementations.

**Consequences**

- Decouples indexer intake from download client operations via event bus
- Enables downstream phases (release acquisition, dashboard) to subscribe to download events
- Simple quality filtering adequate for Phase 3 pilot; Phase 5 adds semantic rules
- Monitor completion tracking prevents duplicate event emission across sweeps
- Non-blocking router design keeps indexer pipeline responsive

**Future Work**

- Phase 3h: HTTP handler for manual completion mark (POST /api/downloads/{id}/complete)
- Phase 5: Semantic quality filtering (resolution, codec, release groups, language)
- Phase 5+: Per-client Item tracking to resolve ClientID limitation
- Kernel: Scheduler integration for Monitor periodic invocation
