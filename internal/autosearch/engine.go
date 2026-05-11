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
	"github.com/ebenderooock/loom/internal/grabs"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/series"
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
	grabStore    *grabs.Store
	logger       *slog.Logger
	audit        *auditlog.Logger
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
	grabStore *grabs.Store,
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
		grabStore:    grabStore,
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

// SearchAndGrab executes the full automated search pipeline:
//  1. Search indexers (with request-chain fallback)
//  2. Parse each result
//  3. Match to a quality definition
//  4. Filter: allowed qualities, zero seeders, upgrade check
//  5. Score: quality tier + custom format score + tiebreakers
//  6. Reject results below MinFormatScore
//  7. Grab the highest-scoring result
func (e *Engine) SearchAndGrab(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	// Load the quality profile.
	profile, err := e.profileStore.Get(ctx, req.QualityProfileID)
	if err != nil {
		e.logSearchFailed(ctx, req, fmt.Sprintf("load quality profile: %v", err))
		return nil, fmt.Errorf("load quality profile %s: %w", req.QualityProfileID, err)
	}

	// Parse quality items from the profile's JSON Items field.
	var items []profileItem
	if err := json.Unmarshal([]byte(profile.Items), &items); err != nil {
		return nil, fmt.Errorf("parse quality items: %w", err)
	}

	// Load quality definitions for matching parsed releases.
	qualDefs, err := e.movieSvc.ListQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("load quality definitions: %w", err)
	}

	// Build lookup: quality def ID → position in profile (tier).
	// Lower tier = higher quality = better.
	allowedMap := make(map[string]int)    // quality def ID → tier index
	allowedDefs := make(map[string]bool)   // quality def ID → is allowed
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

	// Check if this media is already grabbed (avoid duplicate downloads).
	// Stale grabs (>4h without import) are auto-cleared to allow retries.
	if e.grabStore != nil && req.MediaID != "" {
		alreadyGrabbed := false
		switch req.MediaType {
		case "movie":
			if grabbed, err := e.grabStore.GrabbedMovieIDs(ctx, []string{req.MediaID}); err == nil {
				alreadyGrabbed = grabbed[req.MediaID]
			}
			// Self-heal: if grab is stale, clear it and allow re-search.
			if alreadyGrabbed {
				const staleGrabAge = 4 * time.Hour
				if age, exists, _ := e.grabStore.GrabAge(ctx, req.MediaID); exists && age > staleGrabAge {
					e.logger.Info("clearing stale grab to allow re-search",
						"media_id", req.MediaID, "age", age.Round(time.Minute))
					_ = e.grabStore.RemoveByMovie(ctx, req.MediaID)
					if e.movieSvc != nil {
						_ = e.movieSvc.SetMovieStatus(ctx, req.MediaID, movies.MovieStatusMissing)
					}
					alreadyGrabbed = false
				}
			}
		}
		if alreadyGrabbed {
			return &SearchResult{
				Reason: "already_grabbed",
			}, nil
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

	// Request-chain fallback: try ID-based search first, then text fallback.
	queries := e.buildQueryChain(req)
	var allResults []indexers.Result

	for _, q := range queries {
		agg := e.indexerSvc.Search(ctx, q, nil, 30*time.Second)
		allResults = append(allResults, agg.Results...)
		if len(allResults) > 0 {
			break // Got results, stop trying weaker queries.
		}
	}

	result := &SearchResult{
		Considered: len(allResults),
	}

	if len(allResults) == 0 {
		result.Reason = "no results from indexers"
		e.logSearchCompleted(ctx, req, result)
		return result, nil
	}

	// Score and filter each result.
	rejectCounts := make(map[string]int)
	var scored []ScoredRelease

	for _, res := range allResults {
		sr := e.evaluateResult(req, res, qualDefs, allowedMap, allowedDefs, formatScores, profile, existing, cutoffTier)
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
			continue
		}
		scored = append(scored, sr)
	}

	result.Rejected = result.Considered - len(scored)

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
		e.logSearchCompleted(ctx, req, result)
		return result, nil
	}

	// Sort by composite score (highest first).
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].CompositeScore() > scored[j].CompositeScore()
	})

	// Grab the best result.
	best := scored[0]
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
			e.logSearchFailed(ctx, req, result.Reason)
			return result, nil
		}
	}

	result.Grabbed = grabbed

	// Record the grab linkage for UI status tracking.
	e.recordGrab(ctx, req, grabbed)

	e.logSearchCompleted(ctx, req, result)
	return result, nil
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
) ScoredRelease {
	sr := ScoredRelease{Result: res}

	// Parse the release name.
	parsed := parser.Parse(res.Title)
	sr.Parsed = parsed

	// Identity verification: ensure the result matches the requested media.
	if reason := e.verifyIdentity(req, parsed); reason != "" {
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
// Returns empty string if the result is valid, or a rejection reason.
func (e *Engine) verifyIdentity(req SearchRequest, parsed *parser.Release) string {
	if parsed == nil || req.Title == "" {
		return ""
	}

	// Title matching: verify the parsed title is close to the requested title.
	parsedTitle := normalizeTitle(parsed.Title)
	wantTitle := normalizeTitle(req.Title)

	if parsedTitle != "" && wantTitle != "" {
		distance := levenshteinDistance(strings.ToLower(parsedTitle), strings.ToLower(wantTitle))
		maxLen := max(len(parsedTitle), len(wantTitle))
		if maxLen > 0 {
			similarity := 1.0 - float64(distance)/float64(maxLen)
			if similarity < 0.6 {
				return "title_mismatch"
			}
		}
	}

	// Series-specific checks.
	if req.MediaType == "series" || req.MediaType == "episode" {
		// Season pack rejection for single-episode searches.
		if req.Episode > 0 && parsed.IsSeasonPack {
			return "season_pack_for_episode_search"
		}

		// Season verification.
		if req.Season > 0 && parsed.Season > 0 && parsed.Season != req.Season {
			return "wrong_season"
		}

		// Episode verification (only for single-episode searches).
		if req.Episode > 0 && parsed.Episode > 0 && parsed.Episode != req.Episode {
			// Check multi-episode — accept if any episode matches.
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
		Title:          sr.Result.Title,
		IndexerID:      sr.Result.IndexerID,
		Size:           sr.Result.Size,
		QualityTier:    sr.QualityTier,
		FormatScore:    sr.FormatScore,
		CompositeScore: sr.CompositeScore(),
		FormatMatches:  sr.FormatMatches,
		ClientID:       target.ID(),
		DownloadID:     addResult.ItemID,
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
		req.TorrentURL = res.Link
	}
	req.Normalize()
	return req
}

// buildQueryChain builds a prioritized list of indexer queries for
// request-chain fallback. Tries strongest ID first, then weaker IDs,
// then text-only search. The caller stops at the first query that
// returns results.
func (e *Engine) buildQueryChain(req SearchRequest) []indexers.Query {
	base := indexers.Query{
		Season:  req.Season,
		Episode: req.Episode,
	}

	switch req.MediaType {
	case "movie":
		base.Categories = []indexers.Category{2000, 2010, 2020, 2030, 2040, 2045, 2050, 2060}
	case "series", "episode":
		base.Categories = []indexers.Category{5000, 5010, 5020, 5030, 5040, 5045, 5050, 5060, 5070, 5080}
	}

	var chain []indexers.Query

	// Priority 1: IMDB ID (most universal and precise).
	if req.IMDBID != "" {
		q := base
		q.IMDBID = req.IMDBID
		chain = append(chain, q)
	}

	// Priority 2: TVDB ID (strong for series).
	if req.TVDBID != "" {
		q := base
		q.TVDBID = req.TVDBID
		chain = append(chain, q)
	}

	// Priority 3: TMDB ID.
	if req.TMDBID != "" {
		q := base
		q.TMDBID = req.TMDBID
		chain = append(chain, q)
	}

	// Priority 4: Text search fallback.
	if req.Title != "" {
		q := base
		q.Term = req.Title
		chain = append(chain, q)
	}

	// If no IDs and no title, use an empty query (shouldn't happen).
	if len(chain) == 0 {
		chain = append(chain, base)
	}

	return chain
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

// recordGrab persists the grab linkage so the UI can show "grabbed" status.
func (e *Engine) recordGrab(ctx context.Context, req SearchRequest, grabbed *GrabbedRelease) {
	if e.grabStore == nil || grabbed == nil {
		return
	}

	switch req.MediaType {
	case "movie":
		if req.MediaID != "" {
			if err := e.grabStore.RecordMovieGrab(ctx, grabbed.ClientID, grabbed.DownloadID, grabbed.Title, req.MediaID); err != nil {
				e.logger.Warn("failed to record movie grab", "error", err)
			}
			if err := e.movieSvc.SetMovieStatus(ctx, req.MediaID, movies.MovieStatusDownloading); err != nil {
				e.logger.Warn("failed to update movie status to downloading", "error", err)
			}
		}

	case "series", "episode":
		if req.MediaID == "" || e.seriesSvc == nil {
			return
		}

		// If we have a season number, use it to filter the episode query
		var seasonFilter *int
		if req.Season > 0 {
			s := req.Season
			seasonFilter = &s
		}

		episodes, err := e.seriesSvc.ListEpisodes(ctx, req.MediaID, seasonFilter)
		if err != nil {
			e.logger.Warn("failed to list episodes for grab tracking", "error", err)
			return
		}

		var episodeIDs []string
		for _, ep := range episodes {
			if req.Episode > 0 {
				// Specific episode requested — match by episode number
				if ep.EpisodeNumber == req.Episode {
					episodeIDs = append(episodeIDs, ep.ID)
				}
			} else {
				// Season pack — all episodes in the season
				episodeIDs = append(episodeIDs, ep.ID)
			}
		}

		if len(episodeIDs) > 0 {
			if err := e.grabStore.RecordEpisodeGrab(ctx, grabbed.ClientID, grabbed.DownloadID, grabbed.Title, episodeIDs); err != nil {
				e.logger.Warn("failed to record episode grab", "error", err)
			}
		}
	}
}
