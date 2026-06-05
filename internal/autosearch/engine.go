// Engine is the automated search decision engine. It searches
// indexers, evaluates results against a quality profile and custom
// formats, then grabs the best qualifying release via the download
// client registry.
package autosearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/searchdebug"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/workflows"
)

// existingQuality represents the quality state of an existing file on disk.
type existingQuality struct {
	HasFile      bool // true if a file exists for this media item
	HasKnownTier bool // true if we could determine the quality tier
	Tier         int  // position in profile items (lower = better quality)
}

// profileItem mirrors the shape of items stored in quality_profiles_v2.items JSON.
type profileItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Preferred bool   `json:"preferred"`
	Allowed   bool   `json:"allowed"`
}

// Engine orchestrates automated search and grab.
type Engine struct {
	indexerSvc   *indexers.Service
	profileStore *qualityprofiles.Store
	cfEngine     *customformats.Engine
	cfStore      *customformats.Store
	dlRegistry   *downloads.Registry
	movieSvc     movies.Service
	seriesSvc    series.Service
	orchestrator *workflows.Orchestrator
	logger       *slog.Logger
	audit        *auditlog.Logger
	debugStore   *searchdebug.Store
	debugHub     *searchdebug.Hub
	// searchLogEnabled, when set, gates whether new search debug log entries
	// are recorded. A nil func means "always enabled" (back-compat).
	searchLogEnabled func() bool
}

// NewEngine creates an autosearch Engine.
func NewEngine(
	indexerSvc *indexers.Service,
	profileStore *qualityprofiles.Store,
	cfEngine *customformats.Engine,
	cfStore *customformats.Store,
	dlRegistry *downloads.Registry,
	movieSvc movies.Service,
	seriesSvc series.Service,
	logger *slog.Logger,
	opts ...EngineOption,
) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	e := &Engine{
		indexerSvc:   indexerSvc,
		profileStore: profileStore,
		cfEngine:     cfEngine,
		cfStore:      cfStore,
		dlRegistry:   dlRegistry,
		movieSvc:     movieSvc,
		seriesSvc:    seriesSvc,
		logger:       logger.With("module", "autosearch"),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// EngineOption configures optional Engine dependencies.
type EngineOption func(*Engine)

// WithAuditLogger attaches an audit logger to the engine for search event tracking.
func WithAuditLogger(al *auditlog.Logger) EngineOption {
	return func(e *Engine) { e.audit = al }
}

// WithOrchestrator sets the workflow orchestrator for unified state management.
func WithOrchestrator(o *workflows.Orchestrator) EngineOption {
	return func(e *Engine) { e.orchestrator = o }
}

// WithDebugStore sets the search debug log store for detailed search tracing.
func WithDebugStore(s *searchdebug.Store) EngineOption {
	return func(e *Engine) { e.debugStore = s }
}

// WithDebugHub sets the SSE broadcast hub for real-time search queue updates.
func WithDebugHub(h *searchdebug.Hub) EngineOption {
	return func(e *Engine) { e.debugHub = h }
}

// WithSearchLogEnabled gates recording of new search debug log entries behind
// a feature flag. When the supplied func returns false, searches run normally
// but no new trace entries are created or updated. Historical entries are
// unaffected. A nil func (or this option unused) means always enabled.
func WithSearchLogEnabled(fn func() bool) EngineOption {
	return func(e *Engine) { e.searchLogEnabled = fn }
}

// Evaluate scores a set of indexer results against a quality profile
// without grabbing anything. This is the dry-run counterpart to
// SearchAndGrab, used by the manual search UI to display quality
// badges, scores, and rejection reasons.
func (e *Engine) Evaluate(ctx context.Context, req EvaluateRequest) (*EvaluateResponse, error) {
	if req.QualityProfileID == "" {
		return &EvaluateResponse{Results: make([]EvaluatedResult, 0), Total: len(req.Results)}, nil
	}

	profile, err := e.profileStore.Get(ctx, req.QualityProfileID)
	if err != nil {
		return nil, fmt.Errorf("load quality profile %s: %w", req.QualityProfileID, err)
	}

	var items []profileItem
	if err := json.Unmarshal([]byte(profile.Items), &items); err != nil {
		return nil, fmt.Errorf("parse quality items: %w", err)
	}

	qualDefs, err := e.movieSvc.ListQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("load quality definitions: %w", err)
	}

	allowedMap := make(map[string]int)
	allowedDefs := make(map[string]bool)
	for i, item := range items {
		if item.Allowed {
			allowedMap[item.ID] = i
			allowedDefs[item.ID] = true
		}
	}

	formatScores := make(map[string]int)
	for _, fi := range profile.FormatItems {
		formatScores[fi.FormatID] = fi.Score
	}

	existing := e.getExistingQuality(ctx, req.SearchRequest, qualDefs, allowedMap)

	cutoffTier := -1
	if profile.Cutoff != "" {
		if ct, ok := allowedMap[profile.Cutoff]; ok {
			cutoffTier = ct
		}
	}

	// Determine if this looks like an ID-based search.
	idBased := req.IMDBID != "" || req.TVDBID != "" || req.TMDBID != ""

	resp := &EvaluateResponse{
		Total:   len(req.Results),
		Results: make([]EvaluatedResult, 0, len(req.Results)),
	}

	for _, res := range req.Results {
		sr := e.evaluateResult(req.SearchRequest, res, qualDefs, allowedMap, allowedDefs, formatScores, profile, existing, cutoffTier, idBased)

		er := EvaluatedResult{
			IndexerID:   res.IndexerID,
			Title:       res.Title,
			Link:        res.Link,
			SizeBytes:   res.Size,
			PublishDate: res.PubDate.Format("2006-01-02T15:04:05Z"),
			MagnetURI:   res.MagnetURI,
			Infohash:    res.Infohash,
			InfoURL:     res.InfoURL,
			Freeleech:   res.Freeleech,

			Rejected:       sr.Rejected,
			RejectReason:   sr.RejectReason,
			QualityTier:    sr.QualityTier,
			FormatScore:    sr.FormatScore,
			FormatMatches:  sr.FormatMatches,
			CompositeScore: sr.CompositeScore(),
		}

		if res.Seeders != nil {
			er.Seeders = *res.Seeders
		}
		if res.Peers != nil && res.Seeders != nil {
			er.Leechers = *res.Peers - *res.Seeders
			if er.Leechers < 0 {
				er.Leechers = 0
			}
		}
		for _, c := range res.Category {
			er.Categories = append(er.Categories, int(c))
		}

		if sr.QualityDef != nil {
			er.QualityName = sr.QualityDef.Title
		}
		if sr.Parsed != nil {
			er.ParsedTitle = sr.Parsed.Title
			er.ParsedYear = sr.Parsed.Year
			er.ParsedSource = sr.Parsed.Source
			er.ParsedRes = sr.Parsed.Resolution
		}

		if !sr.Rejected {
			resp.Passed++
		}

		resp.Results = append(resp.Results, er)
	}

	// Sort by composite score descending (accepted first, then rejected).
	sort.SliceStable(resp.Results, func(i, j int) bool {
		if resp.Results[i].Rejected != resp.Results[j].Rejected {
			return !resp.Results[i].Rejected
		}
		return resp.Results[i].CompositeScore > resp.Results[j].CompositeScore
	})

	return resp, nil
}

// SearchAndGrab executes the full automated search pipeline:
//  1. Search indexers (with request-chain fallback)
//  2. Parse each result
//  3. Match to a quality definition
//  4. Filter: allowed qualities, zero seeders, upgrade check
//  5. Score: quality tier + custom format score + tiebreakers
//  6. Reject results below MinFormatScore
//  7. Grab the highest-scoring result
//
// For TV series, follows Sonarr's search strategy:
//   - Season search (Season > 0, Episode == 0): try season pack first, then individual episodes
//   - Series search (Season == 0, Episode == 0): iterate seasons, trying pack then episodes for each
func (e *Engine) SearchAndGrab(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	// Assign a search run ID for grouping sub-searches.
	if req.SearchRunID == "" {
		req.SearchRunID = searchdebug.NewID()
	}

	// Sonarr-style fallback for season/series searches.
	if (req.MediaType == "series" || req.MediaType == "episode") && e.seriesSvc != nil && req.Episode == 0 {
		return e.searchSeriesFallback(ctx, req)
	}

	return e.searchAndGrabSingle(ctx, req)
}

// searchSeriesFallback implements Sonarr-style search: try season pack first,
// then fall back to individual episode searches.
func (e *Engine) searchSeriesFallback(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	if req.Season > 0 {
		// Single season: try pack first, then episodes.
		return e.searchSeasonWithFallback(ctx, req)
	}

	// Full series: iterate through each season.
	seasons, err := e.seriesSvc.ListSeasons(ctx, req.MediaID)
	if err != nil {
		e.logger.Warn("failed to list seasons for series search, falling back to single search",
			"series_id", req.MediaID, "error", err)
		return e.searchAndGrabSingle(ctx, req)
	}

	combined := &SearchResult{}
	for _, season := range seasons {
		if ctx.Err() != nil {
			break
		}
		if season.SeasonNumber == 0 {
			continue // skip specials
		}
		seasonReq := req
		seasonReq.Season = season.SeasonNumber
		result, err := e.searchSeasonWithFallback(ctx, seasonReq)
		if err != nil {
			e.logger.Warn("season search failed", "season", season.SeasonNumber, "error", err)
			continue
		}
		combined.Considered += result.Considered
		combined.Rejected += result.Rejected
		if result.Grabbed != nil && combined.Grabbed == nil {
			combined.Grabbed = result.Grabbed
		}
	}
	if combined.Grabbed == nil && combined.Considered == 0 {
		combined.Reason = "no results from any season"
	}
	return combined, nil
}

// searchSeasonWithFallback tries a season pack search first, then falls back
// to searching for each individual episode in the season.
func (e *Engine) searchSeasonWithFallback(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	// Try season pack first (Season set, Episode == 0).
	result, err := e.searchAndGrabSingle(ctx, req)
	if err != nil {
		return nil, err
	}
	if result.Grabbed != nil {
		e.logger.Info("season pack found", "season", req.Season, "title", result.Grabbed.Title)
		return result, nil
	}

	// Season pack not found or all rejected — try individual episodes.
	e.logger.Info("no season pack found, falling back to individual episodes",
		"season", req.Season, "title", req.Title)

	seasonFilter := req.Season
	episodes, err := e.seriesSvc.ListEpisodes(ctx, req.MediaID, &seasonFilter)
	if err != nil {
		e.logger.Warn("failed to list episodes for fallback", "error", err)
		return result, nil // return the season pack result as-is
	}

	for _, ep := range episodes {
		if ctx.Err() != nil {
			break
		}
		epReq := req
		epReq.Episode = ep.EpisodeNumber
		epReq.MediaType = "episode"

		epResult, err := e.searchAndGrabSingle(ctx, epReq)
		if err != nil {
			e.logger.Warn("episode search failed", "season", req.Season,
				"episode", ep.EpisodeNumber, "error", err)
			continue
		}
		result.Considered += epResult.Considered
		result.Rejected += epResult.Rejected
		if epResult.Grabbed != nil && result.Grabbed == nil {
			result.Grabbed = epResult.Grabbed
		}
	}

	return result, nil
}

// searchAndGrabSingle executes a single search+grab (no fallback logic).
func (e *Engine) searchAndGrabSingle(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	startTime := time.Now()

	// Debug/queue entry — created immediately, updated progressively.
	// Skipped entirely when the Search Log feature is disabled, so no new
	// rows are written (all downstream writes are guarded by dbg != nil).
	var dbg *searchdebug.Entry
	if e.debugStore != nil && (e.searchLogEnabled == nil || e.searchLogEnabled()) {
		dbg = &searchdebug.Entry{
			ID:               searchdebug.NewID(),
			CreatedAt:        startTime,
			UpdatedAt:        startTime,
			Status:           searchdebug.StatusSearching,
			SearchRunID:      req.SearchRunID,
			MediaType:        req.MediaType,
			MediaID:          req.MediaID,
			Title:            req.Title,
			Year:             req.Year,
			Season:           req.Season,
			Episode:          req.Episode,
			IMDBID:           req.IMDBID,
			TVDBID:           req.TVDBID,
			TMDBID:           req.TMDBID,
			QualityProfileID: req.QualityProfileID,
			Request:          req,
			Outcome:          "",
		}
		// Persist immediately so the UI can show it as "searching".
		if err := e.debugStore.Create(context.Background(), dbg); err != nil {
			e.logger.Warn("failed to create search queue entry", "error", err)
		} else {
			e.publishUpdate(dbg)
		}
		defer func() {
			dbg.DurationMS = time.Since(startTime).Milliseconds()
			if dbg.Status != searchdebug.StatusCompleted && dbg.Status != searchdebug.StatusFailed {
				if ctx.Err() != nil {
					dbg.Status = searchdebug.StatusCancelled
					dbg.Outcome = "cancelled"
				} else {
					dbg.Status = searchdebug.StatusCompleted
				}
			}
			if err := e.debugStore.Update(context.Background(), dbg); err != nil {
				e.logger.Warn("failed to update search queue entry", "error", err)
			}
			e.publishUpdate(dbg)
		}()
	}

	// Load the quality profile.
	profile, err := e.profileStore.Get(ctx, req.QualityProfileID)
	if err != nil {
		e.logSearchFailed(ctx, req, fmt.Sprintf("load quality profile: %v", err))
		if dbg != nil {
			dbg.Outcome = "profile_load_failed"
			dbg.ErrorMessage = err.Error()
			dbg.Status = searchdebug.StatusFailed
		}
		return nil, fmt.Errorf("load quality profile %s: %w", req.QualityProfileID, err)
	}

	// Parse quality items from the profile's JSON Items field.
	var items []profileItem
	if err := json.Unmarshal([]byte(profile.Items), &items); err != nil {
		if dbg != nil {
			dbg.Outcome = "profile_parse_failed"
			dbg.ErrorMessage = err.Error()
			dbg.Status = searchdebug.StatusFailed
		}
		return nil, fmt.Errorf("parse quality items: %w", err)
	}

	// Load quality definitions for matching parsed releases.
	qualDefs, err := e.movieSvc.ListQualityDefinitions(ctx)
	if err != nil {
		if dbg != nil {
			dbg.Outcome = "quality_defs_failed"
			dbg.ErrorMessage = err.Error()
			dbg.Status = searchdebug.StatusFailed
		}
		return nil, fmt.Errorf("load quality definitions: %w", err)
	}

	// Build lookup: quality def ID → position in profile (tier).
	// Lower tier = higher quality = better.
	allowedMap := make(map[string]int)   // quality def ID → tier index
	allowedDefs := make(map[string]bool) // quality def ID → is allowed
	for i, item := range items {
		if item.Allowed {
			allowedMap[item.ID] = i
			allowedDefs[item.ID] = true
		}
	}

	// Build format score lookup from profile's FormatItems.
	formatScores := make(map[string]int) // custom format ID → score
	for _, fi := range profile.FormatItems {
		formatScores[fi.FormatID] = fi.Score
	}

	// Check if this media already has an active workflow (avoid duplicate downloads).
	if req.MediaID != "" {
		mediaType := workflows.MediaTypeMovie
		if req.MediaType == "series" || req.MediaType == "episode" {
			mediaType = workflows.MediaTypeEpisode
		}
		var store *workflows.Store
		if e.orchestrator != nil {
			store = e.orchestrator.Store()
		}
		if store != nil {
			existing, err := store.FindActiveForMedia(ctx, mediaType, req.MediaID)
			if err == nil && existing != nil {
				if dbg != nil {
					dbg.Outcome = "already_grabbed"
					dbg.Status = searchdebug.StatusCompleted
				}
				return &SearchResult{
					Reason: "already_grabbed",
				}, nil
			}
		}
	}

	// Compute existing quality once (not per-result) for upgrade logic.
	existing := e.getExistingQuality(ctx, req, qualDefs, allowedMap)

	// Resolve cutoff tier from profile.
	cutoffTier := -1 // -1 means no cutoff configured
	if profile.Cutoff != "" {
		if ct, ok := allowedMap[profile.Cutoff]; ok {
			cutoffTier = ct
		}
	}

	// Request-chain fallback: try tiers in order; within each tier,
	// aggregate results from all queries, evaluate, and stop at the
	// first tier that produces accepted results.
	tiers := e.buildQueryChain(req)
	var scored []ScoredRelease
	rejectCounts := make(map[string]int)
	totalConsidered := 0

	for tierIdx, tier := range tiers {
		var tierResults []indexers.Result
		var tierIDsBased bool

		// Capture debug tier detail.
		tierDetail := searchdebug.TierDetail{TierIndex: tierIdx}
		for _, q := range tier {
			if dbg != nil {
				cats := make([]int, len(q.Categories))
				for i, c := range q.Categories {
					cats[i] = int(c)
				}
				tierDetail.Queries = append(tierDetail.Queries, searchdebug.QueryDetail{
					Term:       q.Term,
					Mode:       string(q.Mode),
					IMDBID:     q.IMDBID,
					TVDBID:     q.TVDBID,
					TMDBID:     q.TMDBID,
					Season:     q.Season,
					Episode:    q.Episode,
					Year:       q.Year,
					Categories: cats,
				})
			}

			// Pass 0 so the indexer service applies its configured
			// fail-fast per-indexer timeout (15s direct, 65s for
			// FlareSolverr-proxied indexers) instead of a flat 120s.
			agg := e.indexerSvc.Search(ctx, q, nil, 0)
			tierResults = append(tierResults, agg.Results...)
			if q.IMDBID != "" || q.TVDBID != "" || q.TMDBID != "" {
				tierIDsBased = true
			}
			// Capture per-indexer results for debug.
			if dbg != nil && agg.Diagnostics != nil {
				for _, d := range agg.Diagnostics.Indexers {
					ir := searchdebug.IndexerResult{
						IndexerID:   d.ID,
						IndexerName: d.Name,
						Status:      d.Status,
						ResultCount: d.ResultCount,
						LatencyMS:   d.ResponseTimeMS,
						Error:       d.ErrorMessage,
					}
					// Attach sanitized result entries (cap at 50 per indexer).
					var count int
					for _, r := range agg.Results {
						if count >= 50 {
							break
						}
						if r.IndexerID != "" && r.IndexerID != d.ID {
							continue
						}
						ir.Results = append(ir.Results, sanitizeResult(r))
						count++
					}
					dbg.IndexerResults = append(dbg.IndexerResults, ir)
				}
			}
		}

		if len(tierResults) == 0 {
			e.logger.Debug("autosearch: tier returned no results, trying next",
				"tier", tierIdx,
				"queries", len(tier),
			)
			if dbg != nil {
				dbg.Tiers = append(dbg.Tiers, tierDetail)
				dbg.TotalResults = totalConsidered
				e.updateDebugEntry(dbg)
			}
			continue
		}

		totalConsidered += len(tierResults)
		tierDetail.ResultCount = len(tierResults)

		// Transition to evaluating status.
		if dbg != nil {
			dbg.Status = searchdebug.StatusEvaluating
			dbg.TotalResults = totalConsidered
			e.updateDebugEntry(dbg)
		}

		var tierAccepted int
		for _, res := range tierResults {
			sr := e.evaluateResult(req, res, qualDefs, allowedMap, allowedDefs, formatScores, profile, existing, cutoffTier, tierIDsBased)
			if sr.Rejected {
				rejectCounts[sr.RejectReason]++
				e.logger.Debug("autosearch: result rejected",
					"title", res.Title,
					"reason", sr.RejectReason,
					"parsed_source", func() string {
						if sr.Parsed != nil {
							return sr.Parsed.Source
						}
						return ""
					}(),
					"parsed_resolution", func() int {
						if sr.Parsed != nil {
							return sr.Parsed.Resolution
						}
						return 0
					}(),
				)
			} else {
				scored = append(scored, sr)
				tierAccepted++
			}

			// Capture evaluation details for debug.
			if dbg != nil {
				dbg.Evaluation = append(dbg.Evaluation, scoredToEval(sr))
			}
		}

		tierDetail.AcceptedCount = tierAccepted
		tierDetail.RejectedCount = len(tierResults) - tierAccepted

		// If this tier produced at least one accepted result, stop.
		if len(scored) > 0 {
			tierDetail.StoppedHere = true
			if dbg != nil {
				dbg.Tiers = append(dbg.Tiers, tierDetail)
			}
			break
		}
		// Otherwise continue to the next tier.
		e.logger.Debug("autosearch: all results rejected for tier, trying next",
			"tier", tierIdx,
			"results", len(tierResults),
		)
		if dbg != nil {
			dbg.Tiers = append(dbg.Tiers, tierDetail)
		}
	}

	result := &SearchResult{
		Considered: totalConsidered,
	}

	if totalConsidered == 0 {
		result.Reason = "no results from indexers"
		if dbg != nil {
			dbg.Outcome = "no_results"
			dbg.TotalResults = 0
			dbg.Status = searchdebug.StatusCompleted
		}
		e.logSearchCompleted(ctx, req, result)
		return result, nil
	}

	result.Rejected = totalConsidered - len(scored)
	if dbg != nil {
		dbg.TotalResults = totalConsidered
		dbg.TotalRejected = result.Rejected
	}

	// Build top reject stats.
	for reason, count := range rejectCounts {
		result.TopRejects = append(result.TopRejects, RejectStat{Reason: reason, Count: count})
	}
	sort.Slice(result.TopRejects, func(i, j int) bool {
		return result.TopRejects[i].Count > result.TopRejects[j].Count
	})

	if len(scored) == 0 {
		result.Reason = "all results rejected"
		e.logger.Warn("autosearch: all results rejected",
			"title", req.Title,
			"considered", result.Considered,
			"top_rejects", result.TopRejects,
		)
		if dbg != nil {
			dbg.Outcome = "all_rejected"
			dbg.Status = searchdebug.StatusCompleted
		}
		e.logSearchCompleted(ctx, req, result)
		return result, nil
	}

	// Sort by composite score (highest first).
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].CompositeScore() > scored[j].CompositeScore()
	})

	// Grab the best result.
	best := scored[0]
	if dbg != nil {
		dbg.Status = searchdebug.StatusGrabbing
		e.updateDebugEntry(dbg)
	}
	grabbed, err := e.grabRelease(ctx, &best)
	if err != nil {
		// If grab fails, try the next best.
		e.logger.Warn("grab failed for best result, trying next",
			"title", best.Result.Title,
			"error", err)
		for i := 1; i < len(scored); i++ {
			grabbed, err = e.grabRelease(ctx, &scored[i])
			if err == nil {
				best = scored[i]
				break
			}
			e.logger.Warn("grab failed",
				"title", scored[i].Result.Title,
				"error", err)
		}
		if err != nil {
			result.Reason = fmt.Sprintf("grab failed for all %d candidates: %v", len(scored), err)
			if dbg != nil {
				dbg.Outcome = "grab_failed"
				dbg.ErrorMessage = err.Error()
				dbg.Status = searchdebug.StatusFailed
			}
			e.logSearchFailed(ctx, req, result.Reason)
			return result, nil
		}
	}

	result.Grabbed = grabbed
	if dbg != nil {
		dbg.Outcome = "grabbed"
		dbg.GrabbedTitle = grabbed.Title
		dbg.Status = searchdebug.StatusCompleted
	}

	// Record the grab linkage for UI status tracking.
	e.recordGrab(ctx, req, grabbed)

	e.logSearchCompleted(ctx, req, result)
	return result, nil
}

// updateDebugEntry persists the current state of a debug entry and broadcasts
// a lightweight SSE update to connected clients.
func (e *Engine) updateDebugEntry(dbg *searchdebug.Entry) {
	if e.debugStore == nil {
		return
	}
	if err := e.debugStore.Update(context.Background(), dbg); err != nil {
		e.logger.Warn("failed to update search queue entry", "error", err)
	}
	e.publishUpdate(dbg)
}

// publishUpdate broadcasts a lightweight status update via the SSE hub.
func (e *Engine) publishUpdate(dbg *searchdebug.Entry) {
	if e.debugHub == nil {
		return
	}
	e.debugHub.Publish(searchdebug.MakeStatusUpdate(dbg))
}

// logSearchCompleted writes a search.completed audit entry.
func (e *Engine) logSearchCompleted(ctx context.Context, req SearchRequest, res *SearchResult) {
	if e.audit == nil {
		return
	}
	grabbedTitle := ""
	if res.Grabbed != nil {
		grabbedTitle = res.Grabbed.Title
	}
	e.audit.Log(ctx, auditlog.Entry{
		Category:   "search",
		EventType:  "search.completed",
		Message:    fmt.Sprintf("Search completed: %s (%d considered, %d rejected)", req.Title, res.Considered, res.Rejected),
		Detail:     auditlog.DetailJSON(map[string]any{"media_type": req.MediaType, "media_id": req.MediaID, "title": req.Title, "considered": res.Considered, "rejected": res.Rejected, "grabbed": grabbedTitle, "reason": res.Reason}),
		EntityType: auditlog.StrPtr(req.MediaType),
		EntityID:   auditlog.StrPtr(req.MediaID),
		EntityName: auditlog.StrPtr(req.Title),
		Level:      "info",
		Source:     auditlog.StrPtr("system"),
	})
}

// logSearchFailed writes a search.failed audit entry.
func (e *Engine) logSearchFailed(ctx context.Context, req SearchRequest, errMsg string) {
	if e.audit == nil {
		return
	}
	e.audit.Log(ctx, auditlog.Entry{
		Category:   "search",
		EventType:  "search.failed",
		Message:    fmt.Sprintf("Search failed: %s — %s", req.Title, errMsg),
		Detail:     auditlog.DetailJSON(map[string]any{"media_type": req.MediaType, "media_id": req.MediaID, "title": req.Title, "error": errMsg}),
		EntityType: auditlog.StrPtr(req.MediaType),
		EntityID:   auditlog.StrPtr(req.MediaID),
		EntityName: auditlog.StrPtr(req.Title),
		Level:      "error",
		Source:     auditlog.StrPtr("system"),
	})
}

// evaluateResult parses, quality-matches, filters, and scores a single result.
// The req parameter provides identity context, existing provides upgrade
// comparison state, and cutoffTier is the profile's cutoff position (-1 = none).
func (e *Engine) evaluateResult(
	req SearchRequest,
	res indexers.Result,
	qualDefs []*movies.QualityDefinition,
	allowedMap map[string]int,
	allowedDefs map[string]bool,
	formatScores map[string]int,
	profile *qualityprofiles.QualityProfile,
	existing existingQuality,
	cutoffTier int,
	idBasedQuery bool,
) ScoredRelease {
	sr := ScoredRelease{Result: res}

	// Parse the release name.
	parsed := parser.Parse(res.Title)
	sr.Parsed = parsed

	// Identity verification: ensure the result matches the requested media.
	if reason := e.verifyIdentity(req, parsed, idBasedQuery); reason != "" {
		sr.Rejected = true
		sr.RejectReason = reason
		return sr
	}

	// Match to a quality definition.
	qd := matchQualityDef(parsed, qualDefs)
	if qd == nil {
		sr.Rejected = true
		sr.RejectReason = "unknown_quality"
		return sr
	}
	sr.QualityDef = qd

	// Check if this quality is allowed in the profile.
	tier, ok := allowedMap[qd.ID]
	if !ok || !allowedDefs[qd.ID] {
		sr.Rejected = true
		sr.RejectReason = "quality_not_allowed"
		return sr
	}
	sr.QualityTier = tier

	// Upgrade logic: compare candidate against existing file quality.
	if existing.HasFile {
		if existing.HasKnownTier {
			// Check cutoff first: if existing quality is at or above cutoff,
			// stop upgrading (lower tier = better quality).
			if cutoffTier >= 0 && existing.Tier <= cutoffTier {
				sr.Rejected = true
				sr.RejectReason = "quality_cutoff_met"
				return sr
			}

			if !profile.UpgradeAllowed {
				sr.Rejected = true
				sr.RejectReason = "upgrade_not_allowed"
				return sr
			}

			// Candidate must be a strict upgrade (better tier), unless it's
			// a Proper/Repack at the same tier.
			if tier > existing.Tier {
				sr.Rejected = true
				sr.RejectReason = "not_an_upgrade"
				return sr
			}
			if tier == existing.Tier && !(parsed.IsProper || parsed.IsRepack) {
				sr.Rejected = true
				sr.RejectReason = "not_an_upgrade"
				return sr
			}
		} else {
			// Existing file with unknown quality — reject conservatively
			// unless upgrades are allowed (user explicitly wants to replace).
			if !profile.UpgradeAllowed {
				sr.Rejected = true
				sr.RejectReason = "existing_quality_unknown"
				return sr
			}
		}
	}

	// Reject zero-seeder torrents (usenet results have nil seeders).
	if res.Seeders != nil && *res.Seeders == 0 {
		sr.Rejected = true
		sr.RejectReason = "zero_seeders"
		return sr
	}

	// Size check against quality definition limits.
	minBytes, maxBytes := qd.EffectiveSizeLimits(req.Runtime)
	if minBytes > 0 && res.Size > 0 && res.Size < minBytes {
		sr.Rejected = true
		sr.RejectReason = "below_min_size"
		return sr
	}
	if maxBytes > 0 && res.Size > 0 && res.Size > maxBytes {
		sr.Rejected = true
		sr.RejectReason = "above_max_size"
		return sr
	}

	// Score custom formats.
	ri := buildReleaseInfo(parsed, &res)
	matches := e.cfEngine.ScoreRelease(ri)
	sr.FormatMatches = matches

	totalFormatScore := 0
	for _, m := range matches {
		if score, ok := formatScores[m.CustomFormatID]; ok {
			totalFormatScore += score
		}
	}
	sr.FormatScore = totalFormatScore

	// Reject if below minimum format score.
	if totalFormatScore < profile.MinFormatScore {
		sr.Rejected = true
		sr.RejectReason = "below_min_format_score"
		return sr
	}

	// Tiebreaker score from seeders, age, size, freeleech.
	sr.TiebreakerScore = computeTiebreaker(res)

	return sr
}

// verifyIdentity checks that a parsed result matches the requested media.
// For ID-based queries (IMDB/TVDB/TMDB), the indexer already filtered by
// the external ID so title matching is relaxed (0.3 threshold instead of
// 0.5). For text-only queries the threshold is stricter (0.5). Returns
// empty string if the result is valid, or a rejection reason.
func (e *Engine) verifyIdentity(req SearchRequest, parsed *parser.Release, idBasedQuery bool) string {
	if parsed == nil {
		return ""
	}

	// Year verification for movies: reject if parsed year differs by >1
	// from the requested year. This catches wrong movies with similar
	// titles (e.g. remakes, reboots).
	if req.MediaType == "movie" && req.Year > 0 && parsed.Year > 0 {
		diff := req.Year - parsed.Year
		if diff < 0 {
			diff = -diff
		}
		if diff > 1 {
			return "wrong_year"
		}
	}

	// Title matching: verify the parsed title matches the requested title.
	//
	// For ID-based queries (TVDB/IMDB/TMDB), the indexer pre-filtered by
	// external ID — trust the indexer and skip title comparison entirely.
	// This mirrors Sonarr's SeriesSpecification which compares DB IDs,
	// not titles.
	//
	// For title-based queries, use exact equality after CleanSeriesTitle
	// normalization (strips articles, punctuation, diacritics). Sonarr
	// does not use fuzzy/Levenshtein matching.
	if !idBasedQuery && req.Title != "" {
		parsedTitle := parser.CleanSeriesTitle(parsed.Title)
		wantTitle := parser.CleanSeriesTitle(req.Title)

		if parsedTitle != "" && wantTitle != "" && parsedTitle != wantTitle {
			e.logger.Debug("autosearch: title mismatch",
				"want", wantTitle,
				"got", parsedTitle,
			)
			return "title_mismatch"
		}
	}

	// Series-specific checks.
	if req.MediaType == "series" || req.MediaType == "episode" {
		// Season pack search: reject single-episode releases but allow multi-episode
		// files (e.g. S04E01-E08) which may cover the whole season.
		// Mirrors Sonarr: SeasonSearchCriteria only checks season number; individual
		// episodes are accepted if they are in the requested episode list. We are
		// stricter here (season-pack-first strategy) but we must not reject files
		// that bundle several episodes together.
		if req.Season > 0 && req.Episode == 0 {
			if parsed.Episode > 0 && !parsed.IsSeasonPack && len(parsed.Episodes) <= 1 {
				return "not_a_season_pack"
			}
		}

		// Season pack rejection for single-episode searches.
		if req.Episode > 0 && parsed.IsSeasonPack {
			return "season_pack_for_episode_search"
		}

		// Season verification.
		if req.Season > 0 && parsed.Season > 0 && parsed.Season != req.Season {
			return "wrong_season"
		}

		// Episode verification (only for single-episode searches).
		// Use != -1 instead of > 0 to also catch episode 0 (specials).
		if req.Episode > 0 && parsed.Episode != -1 && parsed.Episode != req.Episode {
			// Check multi-episode — accept if any episode in the file matches.
			found := false
			for _, ep := range parsed.Episodes {
				if ep == req.Episode {
					found = true
					break
				}
			}
			if !found {
				return "wrong_episode"
			}
		}

		// Daily-show date verification. When the request targets a dated
		// episode (daily/talk shows), require the parsed air date to match.
		// Indexers like EZTV's IMDb API return the whole-series feed, so
		// without this a wrong-dated episode could slip through because
		// date-named releases have parsed.Episode == -1 (the episode check
		// above is skipped).
		if req.DailyDate != "" {
			if parsed.DailyDate == "" {
				return "missing_air_date"
			}
			if parsed.DailyDate != req.DailyDate {
				return "wrong_air_date"
			}
		}

		// Unverifiable single-episode guard. A single-episode search must
		// positively identify the episode. Reject releases that carry
		// neither a parseable episode number, a multi-episode list, a daily
		// date, nor a season-pack marker — otherwise an ID-filtered
		// whole-series feed (e.g. EZTV's API) could yield a wrong-episode
		// grab. Standard releases (Clarkson's Farm S05E04 → Episode 4) are
		// unaffected because parsed.Episode is set.
		if req.Episode > 0 && req.DailyDate == "" &&
			parsed.Episode == -1 && len(parsed.Episodes) == 0 &&
			parsed.DailyDate == "" && !parsed.IsSeasonPack {
			return "unverifiable_episode"
		}
	}

	return ""
}

// matchQualityDef maps a parsed release to a quality definition by
// matching source and resolution.
func matchQualityDef(rel *parser.Release, defs []*movies.QualityDefinition) *movies.QualityDefinition {
	if rel == nil {
		return nil
	}

	parsedRes := fmt.Sprintf("%dp", rel.Resolution)
	parsedSource := normalizeSource(rel.Source)

	// Build the canonical slug the parser would produce (e.g., "webdl-1080p",
	// "bluray-2160p-remux"). Match against the quality definition's Name field
	// which uses the same convention.
	slug := parsedSource + "-" + strings.ToLower(parsedRes)
	if rel.IsRemux {
		slug += "-remux"
	}

	// Priority 1: exact slug match against definition Name.
	for _, d := range defs {
		if strings.ToLower(d.Name) == slug {
			return d
		}
	}

	// Priority 2: normalised source + resolution + modifier match.
	// Quality definitions may use display names for Source (e.g. "Web"
	// for webdl, "TV" for hdtv), so normalise both sides.
	for _, d := range defs {
		defRes := strings.ToLower(d.Resolution)
		defSrc := normalizeDefSource(d.Source)
		defRemux := strings.EqualFold(d.Modifier, "REMUX")

		if defRes == strings.ToLower(parsedRes) && defSrc == parsedSource && defRemux == rel.IsRemux {
			return d
		}
	}

	// Priority 3: resolution + modifier only (unknown source).
	for _, d := range defs {
		defRes := strings.ToLower(d.Resolution)
		defRemux := strings.EqualFold(d.Modifier, "REMUX")
		if defRes == strings.ToLower(parsedRes) && defRemux == rel.IsRemux {
			return d
		}
	}

	// Priority 4: resolution only (unknown source and modifier).
	for _, d := range defs {
		defRes := strings.ToLower(d.Resolution)
		if defRes == strings.ToLower(parsedRes) && d.Modifier == "" {
			return d
		}
	}

	return nil
}

// normalizeDefSource maps quality definition Source display names to the
// same canonical form that normalizeSource produces from parser output.
func normalizeDefSource(source string) string {
	switch strings.ToLower(source) {
	case "web":
		return "webdl"
	case "tv":
		return "hdtv"
	default:
		return normalizeSource(source)
	}
}

// normalizeSource maps parser source strings to quality definition source names.
func normalizeSource(source string) string {
	switch strings.ToLower(source) {
	case "bluray", "blu-ray", "bdrip", "brrip":
		return "bluray"
	case "webdl", "web-dl", "web dl":
		return "webdl"
	case "webrip", "web-rip", "web rip":
		return "webrip"
	case "hdtv":
		return "hdtv"
	case "dvd", "dvdrip", "dvd-rip":
		return "dvd"
	case "remux":
		return "remux"
	case "cam", "ts", "telesync", "telecine", "tc":
		return "cam"
	case "screener", "dvdscr":
		return "screener"
	default:
		return strings.ToLower(source)
	}
}

// buildReleaseInfo converts a parsed release + indexer result into the
// shape the custom format engine expects.
func buildReleaseInfo(rel *parser.Release, res *indexers.Result) customformats.ReleaseInfo {
	ri := customformats.ReleaseInfo{
		Title:      res.Title,
		Source:     rel.Source,
		Resolution: fmt.Sprintf("%dp", rel.Resolution),
		Codec:      rel.Codec,
		Audio:      rel.Audio,
		Size:       res.Size,
		Indexer:    res.IndexerID,
		Group:      rel.Group,
		Languages:  rel.Languages,
	}
	if res.Freeleech {
		ri.IndexerFlags = append(ri.IndexerFlags, "freeleech")
	}
	if res.Internal {
		ri.IndexerFlags = append(ri.IndexerFlags, "internal")
	}
	if res.Scene {
		ri.IndexerFlags = append(ri.IndexerFlags, "scene")
	}
	return ri
}

// computeTiebreaker produces a 0–100 score from ancillary signals.
func computeTiebreaker(res indexers.Result) float64 {
	score := 0.0

	// Seeders: 0–30 range (diminishing returns above 20).
	if res.Seeders != nil {
		s := float64(*res.Seeders)
		score += math.Min(30, s/20*30)
	} else {
		// Usenet: assume decent availability.
		score += 20
	}

	// Age: prefer newer (0–25). Full score for <1 day, linearly
	// decreasing to 0 at 30 days.
	if !res.PubDate.IsZero() {
		ageHours := time.Since(res.PubDate).Hours()
		ageDays := ageHours / 24
		if ageDays < 30 {
			score += 25 * (1 - ageDays/30)
		}
	}

	// Size: prefer medium-sized releases (0–15). Penalise very small
	// (<500MB) and very large (>50GB).
	if res.Size > 0 {
		gb := float64(res.Size) / (1024 * 1024 * 1024)
		switch {
		case gb < 0.5:
			score += 5
		case gb < 2:
			score += 10
		case gb < 15:
			score += 15
		case gb < 50:
			score += 10
		default:
			score += 5
		}
	}

	// Freeleech bonus.
	if res.Freeleech {
		score += 15
	}

	return score
}

// grabRelease sends the best-scoring release to a download client.
func (e *Engine) grabRelease(ctx context.Context, sr *ScoredRelease) (*GrabbedRelease, error) {
	// Determine protocol from the result.
	protocol := inferProtocol(&sr.Result)

	// Find a matching download client.
	clients := e.dlRegistry.List()
	var target downloads.DownloadClient
	for _, c := range clients {
		if c.Protocol() == protocol {
			target = c
			break
		}
	}
	if target == nil {
		// Fallback: try any client.
		if len(clients) > 0 {
			target = clients[0]
		} else {
			return nil, fmt.Errorf("no download clients configured")
		}
	}

	// Build the download request.
	req := buildDownloadRequest(&sr.Result)

	// Apply per-indexer seed policy overrides if available.
	if sr.Result.IndexerID != "" {
		if def, err := e.indexerSvc.Get(ctx, sr.Result.IndexerID); err == nil {
			sc := indexers.ParseSeedConfig(def)
			req.SeedRatioLimit = sc.RatioLimit
			req.SeedTimeLimitMinutes = sc.TimeLimitMinutes
		}
	}

	if req.Magnet == "" && req.TorrentURL == "" && len(req.RawBytes) == 0 {
		e.logger.Error("autosearch: download request has no magnet/URL/bytes",
			"title", sr.Result.Title,
			"link", sr.Result.Link,
			"magnet_uri", sr.Result.MagnetURI,
			"infohash", sr.Result.Infohash,
			"indexer", sr.Result.IndexerID,
		)
	}

	addResult, err := target.Add(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("add to %s: %w", target.Name(), err)
	}

	e.logger.Info("release grabbed",
		"title", sr.Result.Title,
		"client", target.Name(),
		"quality", sr.QualityDef.Name,
		"format_score", sr.FormatScore,
		"composite_score", sr.CompositeScore(),
	)

	return &GrabbedRelease{
		Title:                sr.Result.Title,
		IndexerID:            sr.Result.IndexerID,
		Size:                 sr.Result.Size,
		QualityTier:          sr.QualityTier,
		FormatScore:          sr.FormatScore,
		CompositeScore:       sr.CompositeScore(),
		FormatMatches:        sr.FormatMatches,
		ClientID:             target.ID(),
		DownloadID:           addResult.ItemID,
		ContentPath:          addResult.ContentPath,
		SavePath:             addResult.SavePath,
		SeedRatioLimit:       req.SeedRatioLimit,
		SeedTimeLimitMinutes: req.SeedTimeLimitMinutes,
	}, nil
}

// inferProtocol guesses whether a result is torrent or usenet.
func inferProtocol(res *indexers.Result) downloads.Protocol {
	if res.MagnetURI != "" || res.Infohash != "" || res.Seeders != nil {
		return downloads.ProtocolTorrent
	}
	// If the link ends in .nzb or contains "nzb", assume usenet.
	if strings.HasSuffix(strings.ToLower(res.Link), ".nzb") ||
		strings.Contains(strings.ToLower(res.Link), "nzb") {
		return downloads.ProtocolUsenet
	}
	return downloads.ProtocolTorrent
}

// buildDownloadRequest converts an indexer result to a download add request.
func buildDownloadRequest(res *indexers.Result) downloads.AddRequest {
	req := downloads.AddRequest{
		Title:    res.Title,
		Infohash: res.Infohash,
	}
	if res.MagnetURI != "" {
		req.Magnet = res.MagnetURI
	} else if res.Infohash != "" {
		req.Magnet = fmt.Sprintf("magnet:?xt=urn:btih:%s", res.Infohash)
	}
	if res.Link != "" {
		// Some indexers (e.g. EZTV) store the magnet URI in the Link field
		// rather than MagnetURI. Detect this and route it correctly so we
		// don't try to HTTP-fetch a magnet: URI.
		if strings.HasPrefix(strings.ToLower(res.Link), "magnet:") {
			if req.Magnet == "" {
				req.Magnet = res.Link
			}
		} else {
			req.TorrentURL = res.Link
		}
	}
	req.Normalize()
	return req
}

// buildQueryChain builds a tiered list of indexer queries for
// request-chain fallback, faithfully porting the Arr stack's search
// strategy.
//
// Return type is [][]Query: each outer slice is a "tier". All queries
// within a tier are executed and their results aggregated; only if the
// entire tier produces no usable results does the caller fall back to
// the next tier.
//
// Tier 0: ID-based queries (tvsearch/movie with external IDs)
// Tier 1: Title-based queries (tvsearch with q= for TV, search with q= for movies)
//
// Each alternate title generates its own query within the same tier.
func (e *Engine) buildQueryChain(req SearchRequest) [][]indexers.Query {
	base := indexers.Query{
		Season:    req.Season,
		Episode:   req.Episode,
		DailyDate: req.DailyDate,
	}

	switch req.MediaType {
	case "movie":
		base.Categories = []indexers.Category{2000, 2010, 2020, 2030, 2040, 2045, 2050, 2060}
	case "series", "episode":
		base.Categories = []indexers.Category{5000, 5010, 5020, 5030, 5040, 5045, 5050, 5060, 5070, 5080}
	}

	var tiers [][]indexers.Query

	// --- Tier 0: ID-based search ---
	// Aggregate all available IDs into one query (like Arr's aggregated ID search).
	if req.IMDBID != "" || req.TVDBID != "" || req.TMDBID != "" {
		var idQueries []indexers.Query

		if req.MediaType == "movie" {
			// Radarr: movie mode with tmdbid/imdbid.
			q := base
			q.Mode = indexers.ModeMovie
			if req.TMDBID != "" {
				q.TMDBID = req.TMDBID
			}
			if req.IMDBID != "" {
				q.IMDBID = req.IMDBID
			}
			idQueries = append(idQueries, q)
		} else {
			// Sonarr: tvsearch mode with tvdbid/imdbid.
			q := base
			q.Mode = indexers.ModeTVSearch
			if req.TVDBID != "" {
				q.TVDBID = req.TVDBID
			}
			if req.IMDBID != "" {
				q.IMDBID = req.IMDBID
			}
			if req.TMDBID != "" {
				q.TMDBID = req.TMDBID
			}
			idQueries = append(idQueries, q)
		}

		if len(idQueries) > 0 {
			tiers = append(tiers, idQueries)
		}
	}

	// --- Tier 1: Title-based search ---
	// Each title variant (primary + alternates) generates its own query.
	titles := []string{req.Title}
	for _, alt := range req.AlternateTitles {
		if alt != "" && alt != req.Title {
			titles = append(titles, alt)
		}
	}

	if len(titles) > 0 && titles[0] != "" {
		var titleQueries []indexers.Query
		for _, title := range titles {
			q := base
			q.Term = title

			switch req.MediaType {
			case "movie":
				// Radarr: generic search mode with "Title Year".
				q.Mode = indexers.ModeSearch
				q.Year = req.Year
			case "series", "episode":
				// Sonarr: tvsearch mode with q=Title + season/ep params.
				q.Mode = indexers.ModeTVSearch
			}

			titleQueries = append(titleQueries, q)
		}
		tiers = append(tiers, titleQueries)
	}

	// If no IDs and no title, a single empty query as fallback.
	if len(tiers) == 0 {
		tiers = append(tiers, []indexers.Query{base})
	}

	return tiers
}

// getExistingQuality determines the quality state of existing files for
// the requested media item. Called once before the scoring loop.
func (e *Engine) getExistingQuality(
	ctx context.Context,
	req SearchRequest,
	qualDefs []*movies.QualityDefinition,
	allowedMap map[string]int,
) existingQuality {
	switch req.MediaType {
	case "movie":
		return e.getExistingMovieQuality(ctx, req.MediaID, qualDefs, allowedMap)
	case "series", "episode":
		return e.getExistingEpisodeQuality(ctx, req)
	}
	return existingQuality{}
}

// getExistingMovieQuality checks for existing movie files and determines
// the best quality tier among them.
func (e *Engine) getExistingMovieQuality(
	ctx context.Context,
	movieID string,
	qualDefs []*movies.QualityDefinition,
	allowedMap map[string]int,
) existingQuality {
	if movieID == "" || e.movieSvc == nil {
		return existingQuality{}
	}

	files, err := e.movieSvc.ListMovieFiles(ctx, movieID)
	if err != nil || len(files) == 0 {
		return existingQuality{}
	}

	// Find the best quality tier among existing files.
	bestTier := -1
	foundKnown := false
	for _, f := range files {
		if f.Quality == "" {
			continue
		}
		// Try to match the stored quality string to a quality def.
		tier, ok := matchStoredQualityToTier(f.Quality, qualDefs, allowedMap)
		if ok {
			foundKnown = true
			if bestTier < 0 || tier < bestTier {
				bestTier = tier
			}
		}
	}

	return existingQuality{
		HasFile:      true,
		HasKnownTier: foundKnown,
		Tier:         bestTier,
	}
}

// getExistingEpisodeQuality checks if the target episode already has a file.
// Series.EpisodeFile has Source/Resolution/Codec but there's no
// ListEpisodeFiles method yet, so we use the Episode.HasFile flag
// as a conservative indicator.
func (e *Engine) getExistingEpisodeQuality(ctx context.Context, req SearchRequest) existingQuality {
	if req.MediaID == "" || e.seriesSvc == nil || req.Episode <= 0 {
		return existingQuality{}
	}

	seasonFilter := &req.Season
	if req.Season <= 0 {
		seasonFilter = nil
	}
	episodes, err := e.seriesSvc.ListEpisodes(ctx, req.MediaID, seasonFilter)
	if err != nil {
		return existingQuality{}
	}

	for _, ep := range episodes {
		if ep.EpisodeNumber == req.Episode && ep.HasFile {
			// HasFile is true but we don't have structured quality info
			// to determine tier (no ListEpisodeFiles method yet).
			return existingQuality{HasFile: true, HasKnownTier: false}
		}
	}

	return existingQuality{}
}

// matchStoredQualityToTier parses a stored quality string (e.g. "Bluray-1080p",
// "WEBDL-720p") and finds its tier in the profile.
func matchStoredQualityToTier(
	quality string,
	qualDefs []*movies.QualityDefinition,
	allowedMap map[string]int,
) (int, bool) {
	lower := strings.ToLower(quality)

	// Try matching against quality definition names first (exact match).
	for _, qd := range qualDefs {
		if strings.ToLower(qd.Name) == lower || strings.ToLower(qd.Title) == lower {
			if tier, ok := allowedMap[qd.ID]; ok {
				return tier, true
			}
		}
	}

	// Try matching source-resolution pattern (e.g., "bluray-1080p").
	parts := strings.SplitN(lower, "-", 2)
	if len(parts) == 2 {
		src := normalizeSource(parts[0])
		res := parts[1]
		for _, qd := range qualDefs {
			if strings.ToLower(qd.Source) == src && strings.ToLower(qd.Resolution) == res {
				if tier, ok := allowedMap[qd.ID]; ok {
					return tier, true
				}
			}
		}
	}

	return 0, false
}

// recordGrab creates/updates the workflow with grab info so the pipeline can track it.
func (e *Engine) recordGrab(ctx context.Context, req SearchRequest, grabbed *GrabbedRelease) {
	if grabbed == nil || e.orchestrator == nil {
		return
	}

	e.recordGrabOrchestrator(ctx, req, grabbed)
}

// recordGrabOrchestrator uses the unified orchestrator for workflow creation and state transitions.
func (e *Engine) recordGrabOrchestrator(ctx context.Context, req SearchRequest, grabbed *GrabbedRelease) {
	switch req.MediaType {
	case "movie":
		if req.MediaID == "" {
			return
		}
		wf, err := e.orchestrator.StartSearch(ctx, workflows.TypeMovieSearch, workflows.MediaTypeMovie, req.QualityProfileID, []string{req.MediaID})
		if err != nil {
			e.logger.Warn("failed to create movie workflow via orchestrator", "error", err)
			return
		}
		ctx = workflows.WithWorkflowID(ctx, wf.ID)
		e.orchestrator.Send(workflows.CmdGrabbed{
			WorkflowID:           wf.ID,
			ClientID:             grabbed.ClientID,
			DownloadID:           grabbed.DownloadID,
			Title:                grabbed.Title,
			ContentPath:          grabbed.ContentPath,
			SavePath:             grabbed.SavePath,
			SeedRatioLimit:       grabbed.SeedRatioLimit,
			SeedTimeLimitMinutes: grabbed.SeedTimeLimitMinutes,
		})

	case "series", "episode":
		if req.MediaID == "" || e.seriesSvc == nil {
			return
		}

		var seasonFilter *int
		if req.Season > 0 {
			s := req.Season
			seasonFilter = &s
		}

		episodes, err := e.seriesSvc.ListEpisodes(ctx, req.MediaID, seasonFilter)
		if err != nil {
			e.logger.Warn("failed to list episodes for workflow tracking", "error", err)
			return
		}

		var episodeIDs []string
		for _, ep := range episodes {
			if req.Episode > 0 {
				if ep.EpisodeNumber == req.Episode {
					episodeIDs = append(episodeIDs, ep.ID)
				}
			} else {
				episodeIDs = append(episodeIDs, ep.ID)
			}
		}

		if len(episodeIDs) > 0 {
			wf, err := e.orchestrator.StartSearch(ctx, workflows.TypeEpisodeSearch, workflows.MediaTypeEpisode, req.QualityProfileID, episodeIDs)
			if err != nil {
				e.logger.Warn("failed to create episode workflow via orchestrator", "error", err)
				return
			}
			ctx = workflows.WithWorkflowID(ctx, wf.ID)
			e.orchestrator.Send(workflows.CmdGrabbed{
				WorkflowID:           wf.ID,
				ClientID:             grabbed.ClientID,
				DownloadID:           grabbed.DownloadID,
				Title:                grabbed.Title,
				ContentPath:          grabbed.ContentPath,
				SavePath:             grabbed.SavePath,
				SeedRatioLimit:       grabbed.SeedRatioLimit,
				SeedTimeLimitMinutes: grabbed.SeedTimeLimitMinutes,
			})
		}
	}
}

// sanitizeResult converts an indexer Result to a debug-safe ResultEntry
// (strips download URLs, passkeys, magnets).
func sanitizeResult(r indexers.Result) searchdebug.ResultEntry {
	pubDate := ""
	if !r.PubDate.IsZero() {
		pubDate = r.PubDate.Format(time.RFC3339)
	}
	return searchdebug.ResultEntry{
		Title:     r.Title,
		Size:      r.Size,
		Seeders:   r.Seeders,
		Peers:     r.Peers,
		Quality:   r.Quality,
		PubDate:   pubDate,
		Freeleech: r.Freeleech,
		Internal:  r.Internal,
		Scene:     r.Scene,
		IndexerID: r.IndexerID,
	}
}

// scoredToEval converts a ScoredRelease to a debug EvalResult.
func scoredToEval(sr ScoredRelease) searchdebug.EvalResult {
	ev := searchdebug.EvalResult{
		Title:          sr.Result.Title,
		IndexerID:      sr.Result.IndexerID,
		Rejected:       sr.Rejected,
		RejectReason:   sr.RejectReason,
		QualityTier:    sr.QualityTier,
		FormatScore:    sr.FormatScore,
		CompositeScore: sr.CompositeScore(),
		Size:           sr.Result.Size,
		Seeders:        sr.Result.Seeders,
	}
	if sr.Parsed != nil {
		ev.ParsedTitle = sr.Parsed.Title
		ev.ParsedSource = sr.Parsed.Source
		ev.ParsedRes = sr.Parsed.Resolution
	}
	if sr.QualityDef != nil {
		ev.QualityName = sr.QualityDef.Name
	}
	return ev
}
