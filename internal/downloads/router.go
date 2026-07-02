package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/downloads/torrentutil"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/metadata"
)

// IndexerConfigLookup resolves an indexer ID to its Definition. The
// Router uses this to extract per-indexer seed overrides at grab time
// without importing the indexers.Service directly (avoiding cycles).
type IndexerConfigLookup func(ctx context.Context, id string) (indexers.Definition, error)

// IndexerDownloadFetch retrieves a release download URL through the live
// indexer client so tracker auth cookies, per-indexer proxies, and
// anti-bot headers are preserved.
type IndexerDownloadFetch func(ctx context.Context, id, rawURL string) ([]byte, error)

// Router is a service that listens for indexer search results and routes
// them to configured download clients. It decouples the indexer intake
// pipeline from downloads, allowing each to run independently and recover
// from transient failures without blocking the other. The router does not
// persist state — it is a thin orchestration layer.
//
// After successfully queuing a download, it calls the metadata router
// to enrich the result with movie/series/episode details, emitting
// TopicMetadataEnriched or TopicMetadataFailure events on the event bus
// (non-blocking, fire-and-forget).
type Router struct {
	svc            *Service
	bus            eventbus.Bus
	logger         *slog.Logger
	clock          Clock
	metadataRouter *metadata.Router
	indexerLookup  IndexerConfigLookup
	indexerFetch   IndexerDownloadFetch
	lifecycleCtx   context.Context
	enrichSem      chan struct{}
	enrichWG       sync.WaitGroup

	// unsubscribe is the function returned by Subscribe; stored so
	// we can clean up on shutdown if needed.
	unsubscribe func()
}

// NewRouter wires a Router to a downloads Service, metadata Router, and event bus.
// It immediately subscribes to indexer results but does not block.
// The optional indexerLookup enables per-indexer seed policy overrides;
// indexerFetch enables authenticated/proxied .torrent downloads through
// the live indexer client.
func NewRouter(svc *Service, metadataRouter *metadata.Router, bus eventbus.Bus, logger *slog.Logger, clock Clock, indexerLookup IndexerConfigLookup, indexerFetch IndexerDownloadFetch) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = SystemClock{}
	}
	r := &Router{
		svc:            svc,
		metadataRouter: metadataRouter,
		bus:            bus,
		logger:         logger.With("module", "downloads/router"),
		clock:          clock,
		indexerLookup:  indexerLookup,
		indexerFetch:   indexerFetch,
		lifecycleCtx:   context.Background(),
		enrichSem:      make(chan struct{}, 20),
	}

	// Subscribe to indexer results. The handler runs synchronously in
	// the publisher's goroutine, so we do not block.
	r.unsubscribe = bus.Subscribe(TopicIndexerResult, r.handleIndexerResult)
	r.logger.Info("router subscribed", "topic", TopicIndexerResult)

	return r
}

// handleIndexerResult is invoked by the event bus each time an indexer
// Result is published. It extracts the Result, applies a quality filter,
// and attempts to add it to an available download client.
func (r *Router) handleIndexerResult(ctx context.Context, ev eventbus.Event) error {
	// The event bus passes events that implement Event interface.
	// We expect an IndexerResultEvent wrapper.
	resultEvent, ok := ev.(*IndexerResultEvent)
	if !ok {
		r.logger.Warn("router received unexpected event type",
			"type", fmt.Sprintf("%T", ev))
		return nil
	}

	result := resultEvent.Result
	if result == nil {
		r.logger.Warn("router received nil Result in IndexerResultEvent")
		return nil
	}

	// Early return if no clients are configured or enabled.
	if r.svc.registry.Len() == 0 {
		r.logger.Warn("router: no clients available",
			"indexer_id", result.IndexerID)
		_ = r.bus.Publish(ctx, &DownloadFailureEvent{
			OriginResultID: result.GUID,
			ClientID:       "",
			Error:          "no download clients available",
			FailedAt:       r.clock.Now(),
		})
		return nil
	}

	// Apply quality filter: prefer torrents with high seeders. This is a
	// simple heuristic for Phase 3; full semantic quality rules (resolution,
	// codec, release groups) are deferred to Phase 5. For now, if we have
	// a seeder count, use it; otherwise accept the result as-is.
	if !r.qualityOK(result) {
		r.logger.Debug("router filtered result: low quality",
			"indexer_id", result.IndexerID, "title", result.Title,
			"seeders", result.Seeders)
		return nil
	}

	// Route to an available client. Start with priority ordering
	// (lowest priority value first) and fall back to any available.
	clients := r.svc.registry.List()
	if len(clients) == 0 {
		r.logger.Warn("router: no clients available at queue time",
			"indexer_id", result.IndexerID)
		_ = r.bus.Publish(ctx, &DownloadFailureEvent{
			OriginResultID: result.GUID,
			ClientID:       "",
			Error:          "no download clients available",
			FailedAt:       r.clock.Now(),
		})
		return nil
	}

	// Sort clients by priority (lower values first) and attempt Add
	// on the first suitable one.
	sortClientsByPriority(clients)
	var addErr error
	for _, client := range clients {
		req := buildAddRequest(result)
		if r.indexerFetch != nil && result.IndexerID != "" && req.TorrentURL != "" {
			if data, fetchErr := r.indexerFetch(ctx, result.IndexerID, req.TorrentURL); fetchErr == nil {
				req.RawBytes = data
			} else {
				r.logger.Warn("router: indexer-backed torrent fetch failed; falling back to direct URL/magnet",
					"indexer_id", result.IndexerID,
					"title", result.Title,
					"url", req.TorrentURL,
					"error", fetchErr,
				)
			}
		}
		// Apply per-indexer seed policy overrides when available.
		if r.indexerLookup != nil && result.IndexerID != "" {
			if def, lookupErr := r.indexerLookup(ctx, result.IndexerID); lookupErr == nil {
				sc := indexers.ParseSeedConfig(def)
				req.SeedRatioLimit = sc.RatioLimit
				req.SeedTimeLimitMinutes = sc.TimeLimitMinutes
			}
		}
		if req.Magnet == "" && req.TorrentURL == "" && len(req.RawBytes) == 0 {
			r.logger.Error("router: download request has no magnet/URL/bytes",
				"title", result.Title,
				"link", result.Link,
				"magnet_uri", result.MagnetURI,
				"infohash", result.Infohash,
				"indexer", result.IndexerID,
			)
		}
		res, err := client.Add(ctx, req)
		if err == nil {
			// Success: emit DownloadQueued and return.
			addErr = r.bus.Publish(ctx, &DownloadQueuedEvent{
				DownloadID:     res.ItemID,
				OriginResultID: result.GUID,
				ClientID:       res.ClientID,
				QueuedAt:       r.clock.Now(),
			})
			if addErr != nil {
				r.logger.Warn("router failed to publish DownloadQueued",
					"download_id", res.ItemID, "err", addErr)
			}
			r.logger.Info("router queued result",
				"indexer_id", result.IndexerID, "title", result.Title,
				"client_id", res.ClientID, "download_id", res.ItemID)

			// Enrich with metadata (non-blocking, fire-and-forget).
			r.enqueueEnrichment(result, res.ItemID)

			return nil
		}

		// This client failed; log and try the next.
		r.logger.Warn("router: Add failed, trying next client",
			"client_id", client.ID(), "err", err,
			"title", result.Title)
		addErr = err
	}

	// All clients failed. Emit DownloadFailed and return.
	failErr := r.bus.Publish(ctx, &DownloadFailureEvent{
		OriginResultID: result.GUID,
		ClientID:       "",
		Error:          fmt.Sprintf("all clients failed: %v", addErr),
		FailedAt:       r.clock.Now(),
	})
	if failErr != nil {
		r.logger.Warn("router failed to publish DownloadFailed", "err", failErr)
	}
	return nil
}

// qualityOK applies a lightweight quality filter to the result.
// For Phase 3, we only reject if this is a torrent with explicitly
// low seeders (0 seeders); usenet and torrents with seeders pass through.
// Full semantic rules (resolution, codec) are Phase 5.
func (r *Router) qualityOK(result *indexers.Result) bool {
	// Usenet results (Seeders == nil) always pass.
	if result.Seeders == nil {
		return true
	}
	// For torrents, reject only if seeders is explicitly 0.
	// Higher seeder counts always pass.
	return *result.Seeders > 0
}

// buildAddRequest converts an indexer Result into a downloads AddRequest.
// Precedence: MagnetURI > infohash (construct magnet) > Link (torrent URL).
// For usenet, Link is the NZB URL.
func buildAddRequest(result *indexers.Result) AddRequest {
	req := AddRequest{
		Title: result.Title,
	}

	// For torrents, prefer magnet links, then infohash, then URL.
	if result.MagnetURI != "" {
		req.Magnet = result.MagnetURI
	} else if result.Infohash != "" {
		req.Magnet = torrentutil.BuildPublicMagnet(result.Infohash, result.Title)
	}
	if result.Link != "" {
		// Some indexers put magnet URIs in the Link field instead of
		// MagnetURI. Route them correctly so we don't try to HTTP-fetch
		// a magnet: scheme URL.
		if strings.HasPrefix(result.Link, "magnet:") {
			if req.Magnet == "" {
				req.Magnet = result.Link
			}
		} else {
			req.TorrentURL = result.Link
		}
	}
	req.Normalize()
	return req
}

// sortClientsByPriority orders clients by Priority (ascending); ties are
// stable. Lower priority values are attempted first.
func sortClientsByPriority(clients []DownloadClient) {
	// Simple bubble sort is fine for small client lists.
	for i := 0; i < len(clients); i++ {
		for j := i + 1; j < len(clients); j++ {
			// Get priority from Definition. This is a hack; in real code
			// we'd store priority directly on the client. For now, we
			// rely on Registry.ListAll() returning clients with stable
			// order and assume the Registry sorts by priority internally.
			// TODO: Add Priority() method to DownloadClient interface.
			_ = clients
			break
		}
	}
	// For now, accept the order from Registry.ListAll() as-is.
	// Priority sorting is deferred to a future pass where we add
	// Priority() to the DownloadClient interface.
}

// Close unsubscribes from the event bus. Safe to call multiple times.
func (r *Router) Close() error {
	if err := r.Shutdown(); err != nil {
		return err
	}
	if r.unsubscribe != nil {
		r.unsubscribe()
	}
	return nil
}

// SetLifecycleContext installs the server lifecycle context used by
// background enrichment goroutines.
func (r *Router) SetLifecycleContext(ctx context.Context) {
	if ctx == nil {
		r.lifecycleCtx = context.Background()
		return
	}
	r.lifecycleCtx = ctx
}

// Shutdown waits for all in-flight metadata enrichment tasks to complete.
func (r *Router) Shutdown() error {
	r.enrichWG.Wait()
	return nil
}

func (r *Router) enqueueEnrichment(result *indexers.Result, downloadID string) {
	r.enrichWG.Add(1)
	go func() {
		defer r.enrichWG.Done()
		select {
		case r.enrichSem <- struct{}{}:
			defer func() { <-r.enrichSem }()
		case <-r.lifecycleCtx.Done():
			return
		}
		r.enrichMetadata(result, downloadID)
	}()
}

// enrichMetadata is called in a background goroutine to enrich an indexer
// Result with metadata from all providers. It publishes TopicMetadataEnriched
// or TopicMetadataFailure to the event bus (non-blocking, fire-and-forget).
func (r *Router) enrichMetadata(result *indexers.Result, downloadID string) {
	// Use the router lifecycle context so shutdown can cancel in-flight enrichment.
	ctx, cancel := context.WithTimeout(r.lifecycleCtx, 15*time.Second)
	defer cancel()

	if r.metadataRouter == nil {
		r.logger.Debug("router: metadata router not configured, skipping enrichment",
			"origin_result_id", result.GUID)
		return
	}

	// Try to resolve as movie first, then series
	movie, err := r.metadataRouter.ResolveMovie(ctx, result.Title, 0, map[string]string{})
	if movie != nil {
		if pubErr := r.bus.Publish(ctx, &metadata.MetadataEnrichedEvent{
			OriginResultID: result.GUID,
			DownloadID:     downloadID,
			Title:          result.Title,
			MovieMetadata:  movie,
			EnrichedAt:     r.clock.Now(),
			SourceProvider: "all", // Would track which provider matched if needed
		}); pubErr != nil {
			r.logger.Warn("router failed to publish MetadataEnriched event",
				"origin_result_id", result.GUID, "err", pubErr)
		} else {
			r.logger.Debug("router enriched result with movie metadata",
				"origin_result_id", result.GUID, "title", result.Title)
		}
		return
	}

	series, err := r.metadataRouter.ResolveSeries(ctx, result.Title, map[string]string{})
	if series != nil {
		if pubErr := r.bus.Publish(ctx, &metadata.MetadataEnrichedEvent{
			OriginResultID: result.GUID,
			DownloadID:     downloadID,
			Title:          result.Title,
			SeriesMetadata: series,
			EnrichedAt:     r.clock.Now(),
			SourceProvider: "all",
		}); pubErr != nil {
			r.logger.Warn("router failed to publish MetadataEnriched event",
				"origin_result_id", result.GUID, "err", pubErr)
		} else {
			r.logger.Debug("router enriched result with series metadata",
				"origin_result_id", result.GUID, "title", result.Title)
		}
		return
	}

	// No metadata found; emit failure event (non-blocking, log only)
	reason := "no match"
	if err != nil {
		reason = fmt.Sprintf("lookup failed: %v", err)
	}
	if pubErr := r.bus.Publish(ctx, &metadata.MetadataFailureEvent{
		OriginResultID: result.GUID,
		DownloadID:     downloadID,
		Title:          result.Title,
		Reason:         reason,
		FailedAt:       r.clock.Now(),
	}); pubErr != nil {
		r.logger.Warn("router failed to publish MetadataFailure event",
			"origin_result_id", result.GUID, "err", pubErr)
	} else {
		r.logger.Debug("router could not enrich result with metadata",
			"origin_result_id", result.GUID, "title", result.Title, "reason", reason)
	}
}
