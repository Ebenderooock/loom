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

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/grabs"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/series"
)

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
) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
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
}

// SearchAndGrab executes the full automated search pipeline:
//  1. Search indexers
//  2. Parse each result
//  3. Match to a quality definition
//  4. Filter: only allowed qualities, reject zero-seeder torrents
//  5. Score: quality tier + custom format score + tiebreakers
//  6. Reject results below MinFormatScore
//  7. Grab the highest-scoring result
func (e *Engine) SearchAndGrab(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	// Load the quality profile.
	profile, err := e.profileStore.Get(ctx, req.QualityProfileID)
	if err != nil {
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

	// Build the indexer search query.
	q := e.buildQuery(req)

	// Search all indexers.
	agg := e.indexerSvc.Search(ctx, q, nil, 30*time.Second)

	result := &SearchResult{
		Considered: len(agg.Results),
	}

	if len(agg.Results) == 0 {
		result.Reason = "no results from indexers"
		return result, nil
	}

	// Score and filter each result.
	rejectCounts := make(map[string]int)
	var scored []ScoredRelease

	for _, res := range agg.Results {
		sr := e.evaluateResult(res, qualDefs, allowedMap, allowedDefs, formatScores, profile)
		if sr.Rejected {
			rejectCounts[sr.RejectReason]++
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
			return result, nil
		}
	}

	result.Grabbed = grabbed

	// Record the grab linkage for UI status tracking.
	e.recordGrab(ctx, req, grabbed)

	return result, nil
}

// evaluateResult parses, quality-matches, filters, and scores a single result.
func (e *Engine) evaluateResult(
	res indexers.Result,
	qualDefs []*movies.QualityDefinition,
	allowedMap map[string]int,
	allowedDefs map[string]bool,
	formatScores map[string]int,
	profile *qualityprofiles.QualityProfile,
) ScoredRelease {
	sr := ScoredRelease{Result: res}

	// Parse the release name.
	parsed := parser.Parse(res.Title)
	sr.Parsed = parsed

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

	// Reject zero-seeder torrents (usenet results have nil seeders).
	if res.Seeders != nil && *res.Seeders == 0 {
		sr.Rejected = true
		sr.RejectReason = "zero_seeders"
		return sr
	}

	// Size check against quality definition limits.
	if qd.MinFileSize > 0 && res.Size > 0 && res.Size < qd.MinFileSize {
		sr.Rejected = true
		sr.RejectReason = "below_min_size"
		return sr
	}
	if qd.MaxFileSize > 0 && res.Size > 0 && res.Size > qd.MaxFileSize {
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

// matchQualityDef maps a parsed release to a quality definition by
// matching source and resolution.
func matchQualityDef(rel *parser.Release, defs []*movies.QualityDefinition) *movies.QualityDefinition {
	if rel == nil {
		return nil
	}

	parsedRes := fmt.Sprintf("%dp", rel.Resolution)
	parsedSource := normalizeSource(rel.Source)

	// Try exact source + resolution match first.
	for _, d := range defs {
		defRes := strings.ToLower(d.Resolution)
		defSrc := strings.ToLower(d.Source)

		if defRes == strings.ToLower(parsedRes) && defSrc == parsedSource {
			return d
		}
	}

	// Fallback: resolution-only match (e.g., if source is unknown).
	for _, d := range defs {
		defRes := strings.ToLower(d.Resolution)
		if defRes == strings.ToLower(parsedRes) {
			return d
		}
	}

	return nil
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
		Size:       res.Size,
		Indexer:    res.IndexerID,
		Group:      rel.Group,
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
		Title: res.Title,
	}
	if res.MagnetURI != "" {
		req.Magnet = res.MagnetURI
	} else if res.Infohash != "" {
		req.Magnet = fmt.Sprintf("magnet:?xt=urn:btih:%s", res.Infohash)
	} else {
		req.TorrentURL = res.Link
	}
	if req.Magnet == "" && req.TorrentURL == "" {
		req.TorrentURL = res.Link
	}
	return req
}

// buildQuery builds an indexer query from a search request.
func (e *Engine) buildQuery(req SearchRequest) indexers.Query {
	q := indexers.Query{
		Term:   req.Title,
		IMDBID: req.IMDBID,
		TMDBID: req.TMDBID,
		TVDBID: req.TVDBID,
		Season: req.Season,
		Episode: req.Episode,
	}

	switch req.MediaType {
	case "movie":
		q.Categories = []indexers.Category{2000, 2010, 2020, 2030, 2040, 2045, 2050, 2060}
	case "series", "episode":
		q.Categories = []indexers.Category{5000, 5010, 5020, 5030, 5040, 5045, 5050, 5060, 5070, 5080}
	}

	return q
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
