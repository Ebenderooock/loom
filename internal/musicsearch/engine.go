// Package musicsearch implements automated search and acquisition for music
// albums. It is a self-contained parallel to internal/autosearch (which is
// movie/TV specific): it reuses only the media-agnostic indexer transport
// (indexers.Service) and download handoff (downloads.Registry), and scores
// results with the audio quality model in internal/music.
package musicsearch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/music"
)

// Errors returned by the engine.
var (
	ErrNotFound      = errors.New("musicsearch: album not found")
	ErrNoResults     = errors.New("musicsearch: no matching releases found")
	ErrNoDownloaders = errors.New("musicsearch: no download clients configured")
)

// audioCategories are the Newznab audio category families queried for music.
var audioCategories = []indexers.Category{indexers.CategoryAudio, 3010, 3020, 3030, 3040}

// Engine searches indexers for an album and grabs the best release.
type Engine struct {
	indexerSvc *indexers.Service
	dlRegistry *downloads.Registry
	repo       music.Repository
	cfEngine   *customformats.Engine
	logger     *slog.Logger
}

// NewEngine constructs a music acquisition engine.
func NewEngine(indexerSvc *indexers.Service, dlRegistry *downloads.Registry, repo music.Repository, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{indexerSvc: indexerSvc, dlRegistry: dlRegistry, repo: repo, logger: logger}
}

// SetCustomFormats attaches a custom-format engine so audio releases are scored
// against the user's custom formats (per the audio quality profile's
// FormatItems). Optional; when nil, format scoring is skipped.
func (e *Engine) SetCustomFormats(cf *customformats.Engine) {
	e.cfEngine = cf
}

// formatScore evaluates a release against all custom formats and returns the
// aggregate score (per the profile's FormatItems) plus the matched formats.
// Returns (0, nil) when no custom-format engine is attached.
func (e *Engine) formatScore(profile *music.AudioQualityProfile, parsed *music.MusicRelease, res *indexers.Result) (int, []customformats.FormatMatch) {
	if e.cfEngine == nil {
		return 0, nil
	}
	ri := customformats.ParseReleaseName(res.Title)
	if parsed != nil && parsed.Format != "" {
		ri.Audio = parsed.Format
	}
	if parsed != nil && parsed.Media != "" && ri.Source == "" {
		ri.Source = parsed.Media
	}
	ri.Size = res.Size
	ri.Indexer = res.IndexerID
	if res.Freeleech {
		ri.IndexerFlags = append(ri.IndexerFlags, "freeleech")
	}
	if res.Internal {
		ri.IndexerFlags = append(ri.IndexerFlags, "internal")
	}
	if res.Scene {
		ri.IndexerFlags = append(ri.IndexerFlags, "scene")
	}
	matches := e.cfEngine.ScoreRelease(ri)
	if profile == nil || len(profile.FormatItems) == 0 {
		return 0, matches
	}
	scoreByID := make(map[string]int, len(profile.FormatItems))
	for _, fi := range profile.FormatItems {
		scoreByID[fi.FormatID] = fi.Score
	}
	total := 0
	for _, m := range matches {
		if s, ok := scoreByID[m.CustomFormatID]; ok {
			total += s
		}
	}
	return total, matches
}

// GrabResult describes a release that was sent to a download client.
type GrabResult struct {
	AlbumID        string  `json:"album_id"`
	Title          string  `json:"title"`
	IndexerID      string  `json:"indexer_id"`
	Size           int64   `json:"size"`
	QualityName    string  `json:"quality_name"`
	Tier           int     `json:"tier"`
	CompositeScore float64 `json:"composite_score"`
	ClientID       string  `json:"client_id"`
	DownloadID     string  `json:"download_id"`
}

// Candidate is a scored, identity-matched release (used for diagnostics/tests).
type Candidate struct {
	Result        indexers.Result
	Parsed        *music.MusicRelease
	Score         music.AudioScore
	FormatScore   int
	FormatMatches []customformats.FormatMatch
}

// rankScore combines the audio quality composite with the custom-format score
// so custom formats influence selection the way they do in Radarr/Sonarr.
func (c Candidate) rankScore() float64 {
	return c.Score.Composite() + float64(c.FormatScore)
}

// ReleaseCandidate is a scored release surfaced to the interactive search UI.
// Unlike automated acquisition it includes profile-rejected releases (flagged
// via Allowed) so the user can make an informed manual choice.
type ReleaseCandidate struct {
	GUID        string  `json:"guid"`
	Title       string  `json:"title"`
	IndexerID   string  `json:"indexer_id"`
	Size        int64   `json:"size"`
	Seeders     *int    `json:"seeders,omitempty"`
	Protocol    string  `json:"protocol"`
	QualityName string  `json:"quality_name"`
	Tier        int     `json:"tier"`
	Score       float64 `json:"score"`
	FormatScore int     `json:"format_score"`
	Allowed     bool    `json:"allowed"`
	MeetsCutoff bool    `json:"meets_cutoff"`
	Link        string  `json:"link,omitempty"`
	MagnetURI   string  `json:"magnet_uri,omitempty"`
	Infohash    string  `json:"infohash,omitempty"`

	FormatMatches []customformats.FormatMatch `json:"format_matches,omitempty"`
}

// GrabRequest identifies a specific release to grab in an interactive search.
// The frontend echoes back the chosen release's download coordinates.
type GrabRequest struct {
	Title     string `json:"title"`
	IndexerID string `json:"indexer_id"`
	Link      string `json:"link,omitempty"`
	MagnetURI string `json:"magnet_uri,omitempty"`
	Infohash  string `json:"infohash,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Seeders   *int   `json:"seeders,omitempty"`
}

// ListReleases performs an interactive search for an album, returning all
// identity-matched releases scored against the artist's profile (including
// profile-rejected ones, flagged via Allowed) sorted best-first. It does not
// grab anything.
func (e *Engine) ListReleases(ctx context.Context, albumID string) ([]ReleaseCandidate, error) {
	album, artist, err := e.loadAlbumArtist(ctx, albumID)
	if err != nil {
		return nil, err
	}
	defs, err := e.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quality definitions: %w", err)
	}
	var profile *music.AudioQualityProfile
	if artist.QualityProfileID != "" {
		profile, _ = e.repo.GetAudioQualityProfile(ctx, artist.QualityProfileID)
	}

	normArtist := normalize(artist.Name)
	normAlbum := normalize(album.Title)
	year := albumYear(album)

	seen := make(map[string]bool)
	var out []ReleaseCandidate
	for _, q := range buildAudioQueries(artist.Name, album.Title, year) {
		agg := e.indexerSvc.Search(ctx, q, nil, 0)
		for _, res := range agg.Results {
			key := res.GUID
			if key == "" {
				key = res.IndexerID + "|" + res.Title
			}
			if seen[key] {
				continue
			}
			seen[key] = true

			parsed := music.ParseMusic(res.Title)
			if !identityMatch(res.Title, parsed, normArtist, normAlbum) {
				continue
			}
			score := music.ScoreAudioRelease(parsed, defs, profile, seeders(res), res.Size)
			fmtScore, fmtMatches := e.formatScore(profile, parsed, &res)
			qn := ""
			if score.Definition != nil {
				qn = score.Definition.Name
			}
			out = append(out, ReleaseCandidate{
				GUID:        res.GUID,
				Title:       res.Title,
				IndexerID:   res.IndexerID,
				Size:        res.Size,
				Seeders:     res.Seeders,
				Protocol:    string(inferProtocol(&res)),
				QualityName: qn,
				Tier:        score.Tier,
				Score:       score.Composite() + float64(fmtScore),
				FormatScore: fmtScore,
				Allowed:     score.Allowed,
				MeetsCutoff: score.MeetsCutoff,
				Link:        res.Link,
				MagnetURI:   res.MagnetURI,
				Infohash:    res.Infohash,

				FormatMatches: fmtMatches,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// GrabRelease grabs a specific, user-chosen release for an album (interactive
// search). The release's quality is scored for reporting but not gated.
func (e *Engine) GrabRelease(ctx context.Context, albumID string, req GrabRequest) (*GrabResult, error) {
	if _, _, err := e.loadAlbumArtist(ctx, albumID); err != nil {
		return nil, err
	}
	defs, err := e.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quality definitions: %w", err)
	}
	res := indexers.Result{
		Title:     req.Title,
		IndexerID: req.IndexerID,
		Link:      req.Link,
		MagnetURI: req.MagnetURI,
		Infohash:  req.Infohash,
		Size:      req.Size,
		Seeders:   req.Seeders,
	}
	parsed := music.ParseMusic(req.Title)
	score := music.ScoreAudioRelease(parsed, defs, nil, seeders(res), res.Size)
	c := Candidate{Result: res, Parsed: parsed, Score: score}
	grab, err := e.grab(ctx, albumID, &c)
	if err != nil {
		return nil, err
	}
	e.logger.Info("musicsearch: grabbed manual release", "album", albumID, "title", grab.Title)
	return grab, nil
}

// SearchAlbum searches configured indexers for the given album and grabs the
// best-scoring, profile-allowed release.
func (e *Engine) SearchAlbum(ctx context.Context, albumID string) (*GrabResult, error) {
	return e.searchAndGrab(ctx, albumID, false)
}

// SearchAlbumUpgrade searches for a strictly better-quality release than the
// album currently has on disk. It only grabs when the profile permits upgrades
// and the best candidate beats the album's current quality tier; otherwise it
// returns ErrNoResults.
func (e *Engine) SearchAlbumUpgrade(ctx context.Context, albumID string) (*GrabResult, error) {
	return e.searchAndGrab(ctx, albumID, true)
}

func (e *Engine) searchAndGrab(ctx context.Context, albumID string, upgradeOnly bool) (*GrabResult, error) {
	album, artist, err := e.loadAlbumArtist(ctx, albumID)
	if err != nil {
		return nil, err
	}

	// Record the search attempt regardless of outcome so the auto-searcher
	// honours a recheck interval and does not hammer indexers.
	defer e.touchLastSearch(ctx, album)

	defs, err := e.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quality definitions: %w", err)
	}
	var profile *music.AudioQualityProfile
	if artist.QualityProfileID != "" {
		profile, _ = e.repo.GetAudioQualityProfile(ctx, artist.QualityProfileID)
	}

	// In upgrade mode the profile must allow upgrades and the album must already
	// have files; the grab is gated on beating the current tier.
	minTier := -1
	if upgradeOnly {
		if profile == nil || !profile.UpgradeAllowed {
			return nil, ErrNoResults
		}
		cur, hasFiles := e.albumCurrentTier(ctx, album, defs)
		if !hasFiles {
			return nil, ErrNoResults
		}
		minTier = cur
	}

	year := albumYear(album)
	candidates := e.gatherCandidates(ctx, artist.Name, album.Title, year, defs, profile)
	if len(candidates) == 0 {
		e.logger.Info("musicsearch: no matching releases", "artist", artist.Name, "album", album.Title)
		return nil, ErrNoResults
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].rankScore() > candidates[j].rankScore()
	})
	best := candidates[0]

	if minTier >= 0 && best.Score.Tier <= minTier {
		e.logger.Debug("musicsearch: no upgrade available",
			"artist", artist.Name, "album", album.Title, "current_tier", minTier, "best_tier", best.Score.Tier)
		return nil, ErrNoResults
	}

	grab, err := e.grab(ctx, albumID, &best)
	if err != nil {
		return nil, err
	}
	e.logger.Info("musicsearch: grabbed album release",
		"artist", artist.Name, "album", album.Title, "upgrade", upgradeOnly,
		"title", grab.Title, "quality", grab.QualityName, "score", grab.CompositeScore,
	)
	return grab, nil
}

// loadAlbumArtist fetches an album and its artist, returning ErrNotFound if
// either is missing.
func (e *Engine) loadAlbumArtist(ctx context.Context, albumID string) (*music.Album, *music.Artist, error) {
	album, err := e.repo.GetAlbum(ctx, albumID)
	if err != nil {
		return nil, nil, fmt.Errorf("get album: %w", err)
	}
	if album == nil {
		return nil, nil, ErrNotFound
	}
	artist, err := e.repo.GetArtist(ctx, album.ArtistID)
	if err != nil {
		return nil, nil, fmt.Errorf("get artist: %w", err)
	}
	if artist == nil {
		return nil, nil, ErrNotFound
	}
	return album, artist, nil
}

// gatherCandidates runs the tiered query chain and returns identity-matched,
// profile-allowed, scored releases. It stops at the first tier that yields
// candidates.
func (e *Engine) gatherCandidates(ctx context.Context, artist, album string, year int, defs []*music.AudioQualityDefinition, profile *music.AudioQualityProfile) []Candidate {
	normArtist := normalize(artist)
	normAlbum := normalize(album)

	for _, q := range buildAudioQueries(artist, album, year) {
		agg := e.indexerSvc.Search(ctx, q, nil, 0)
		var candidates []Candidate
		seen := make(map[string]bool)
		for _, res := range agg.Results {
			key := res.IndexerID + "|" + res.Title
			if seen[key] {
				continue
			}
			seen[key] = true

			parsed := music.ParseMusic(res.Title)
			if !identityMatch(res.Title, parsed, normArtist, normAlbum) {
				continue
			}
			score := music.ScoreAudioRelease(parsed, defs, profile, seeders(res), res.Size)
			if !score.Allowed {
				continue
			}
			fmtScore, fmtMatches := e.formatScore(profile, parsed, &res)
			if profile != nil && fmtScore < profile.MinFormatScore {
				continue
			}
			candidates = append(candidates, Candidate{
				Result: res, Parsed: parsed, Score: score,
				FormatScore: fmtScore, FormatMatches: fmtMatches,
			})
		}
		if len(candidates) > 0 {
			return candidates
		}
	}
	return nil
}

// buildAudioQueries returns the tiered free-text queries for an album. Torznab
// has no music ID search, so all tiers are text-based over the audio categories.
func buildAudioQueries(artist, album string, year int) []indexers.Query {
	base := func(term string) indexers.Query {
		return indexers.Query{
			Term:       strings.TrimSpace(term),
			Mode:       indexers.ModeSearch,
			Categories: audioCategories,
		}
	}
	var queries []indexers.Query
	queries = append(queries, base(artist+" "+album))
	if year > 0 {
		queries = append(queries, base(fmt.Sprintf("%s %s %d", artist, album, year)))
	}
	queries = append(queries, base(album))
	return queries
}

// identityMatch verifies a result plausibly belongs to the requested album by
// requiring both the artist and album tokens to appear in the normalized title
// (or the parsed fields).
func identityMatch(title string, parsed *music.MusicRelease, normArtist, normAlbum string) bool {
	normTitle := normalize(title)
	albumOK := normAlbum != "" && (strings.Contains(normTitle, normAlbum) || strings.Contains(normalize(parsed.Album), normAlbum))
	if !albumOK {
		return false
	}
	if normArtist == "" {
		return true
	}
	return strings.Contains(normTitle, normArtist) || strings.Contains(normalize(parsed.Artist), normArtist)
}

func (e *Engine) grab(ctx context.Context, albumID string, c *Candidate) (*GrabResult, error) {
	clients := e.dlRegistry.List()
	if len(clients) == 0 {
		return nil, ErrNoDownloaders
	}
	protocol := inferProtocol(&c.Result)
	var target downloads.DownloadClient
	for _, cl := range clients {
		if cl.Protocol() == protocol {
			target = cl
			break
		}
	}
	if target == nil {
		target = clients[0]
	}

	req := buildDownloadRequest(&c.Result)
	req.MediaType = "music"
	if req.Magnet == "" && req.TorrentURL == "" && req.NZBURL == "" && len(req.RawBytes) == 0 {
		return nil, fmt.Errorf("musicsearch: release %q has no downloadable link", c.Result.Title)
	}

	addResult, err := target.Add(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("add to %s: %w", target.Name(), err)
	}

	qualityName := ""
	if c.Score.Definition != nil {
		qualityName = c.Score.Definition.Name
	}
	return &GrabResult{
		AlbumID:        albumID,
		Title:          c.Result.Title,
		IndexerID:      c.Result.IndexerID,
		Size:           c.Result.Size,
		QualityName:    qualityName,
		Tier:           c.Score.Tier,
		CompositeScore: c.Score.Composite() + float64(c.FormatScore),
		ClientID:       target.ID(),
		DownloadID:     addResult.ItemID,
	}, nil
}

func inferProtocol(res *indexers.Result) downloads.Protocol {
	if res.MagnetURI != "" || res.Infohash != "" || res.Seeders != nil {
		return downloads.ProtocolTorrent
	}
	link := strings.ToLower(res.Link)
	if strings.HasSuffix(link, ".nzb") || strings.Contains(link, "nzb") {
		return downloads.ProtocolUsenet
	}
	return downloads.ProtocolTorrent
}

func buildDownloadRequest(res *indexers.Result) downloads.AddRequest {
	req := downloads.AddRequest{Title: res.Title, Infohash: res.Infohash}
	if res.MagnetURI != "" {
		req.Magnet = res.MagnetURI
	} else if res.Infohash != "" {
		req.Magnet = fmt.Sprintf("magnet:?xt=urn:btih:%s", res.Infohash)
	}
	if res.Link != "" {
		switch {
		case strings.HasPrefix(strings.ToLower(res.Link), "magnet:"):
			if req.Magnet == "" {
				req.Magnet = res.Link
			}
		case inferProtocol(res) == downloads.ProtocolUsenet:
			req.NZBURL = res.Link
		default:
			req.TorrentURL = res.Link
		}
	}
	req.Normalize()
	return req
}

func seeders(res indexers.Result) int {
	if res.Seeders != nil {
		return *res.Seeders
	}
	return 0
}

// albumYear extracts a 4-digit year from an album's release date.
func albumYear(al *music.Album) int {
	d := strings.TrimSpace(al.ReleaseDate)
	if len(d) >= 4 {
		y := 0
		for _, r := range d[:4] {
			if r < '0' || r > '9' {
				return 0
			}
			y = y*10 + int(r-'0')
		}
		return y
	}
	return 0
}

// normalize lowercases and strips non-alphanumerics for loose token matching.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
