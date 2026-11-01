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
	"net/url"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	torrentdl "github.com/ebenderooock/loom/internal/downloads/torrent"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/music"
)

// Errors returned by the engine.
var (
	ErrNotFound               = errors.New("musicsearch: album not found")
	ErrNoResults              = errors.New("musicsearch: no matching releases found")
	ErrNoDownloaders          = errors.New("musicsearch: no download clients configured")
	ErrNoCompatibleDownloader = errors.New("musicsearch: no compatible download client configured")
	ErrInvalidReleaseLink     = errors.New("musicsearch: invalid release download link")
)

// audioCategories are the Newznab audio category families queried for music.
var audioCategories = []indexers.Category{indexers.CategoryAudio, 3010, 3020, 3030, 3040}

// Engine searches indexers for an album and grabs the best release.
type Engine struct {
	indexerSvc  *indexers.Service
	dlRegistry  *downloads.Registry
	repo        music.Repository
	cfEngine    *customformats.Engine
	logger      *slog.Logger
	albumLoader interface {
		GetAlbum(context.Context, string) (*music.Album, error)
	}
	searchMu  sync.Mutex
	searching map[string]bool
}

// NewEngine constructs a music acquisition engine.
func NewEngine(indexerSvc *indexers.Service, dlRegistry *downloads.Registry, repo music.Repository, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{indexerSvc: indexerSvc, dlRegistry: dlRegistry, repo: repo, logger: logger, searching: make(map[string]bool)}
}

// SetAlbumLoader attaches the music service used to load MusicBrainz releases
// and tracks before automated acquisition. A nil loader leaves unloaded albums
// ineligible for auto-search rather than treating an empty list as missing.
func (e *Engine) SetAlbumLoader(loader interface {
	GetAlbum(context.Context, string) (*music.Album, error)
}) {
	e.albumLoader = loader
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
// identity-matched releases scored against the album's effective quality
// profile (including profile-rejected ones, flagged via Allowed) sorted
// best-first. It does not grab anything.
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
	profileID := albumQualityProfileID(album, artist)
	if profileID != "" {
		profile, err = e.repo.GetAudioQualityProfile(ctx, profileID)
		if err != nil {
			return nil, fmt.Errorf("get audio quality profile: %w", err)
		}
		if profile == nil {
			return nil, fmt.Errorf("%w: audio quality profile %q", ErrNotFound, profileID)
		}
	}

	year := albumYear(album)
	queries := buildAlbumQueries(album, artist, year)

	seen := make(map[string]bool)
	var out []ReleaseCandidate
	for _, q := range queries {
		agg := e.indexerSvc.Search(ctx, q, nil, 0)
		for _, res := range agg.Results {
			key := res.GUID
			if key == "" {
				key = res.IndexerID + "|" + res.Title
			}
			if seen[key] {
				continue
			}

			parsed := music.ParseMusic(res.Title)
			if !albumIdentityMatch(res.Title, parsed, album, artist) {
				continue
			}
			seen[key] = true
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
	if err := validateReleaseCoordinates(req); err != nil {
		return nil, err
	}
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

func validateReleaseCoordinates(req GrabRequest) error {
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.IndexerID) == "" {
		return fmt.Errorf("%w: title and indexer_id are required", ErrInvalidReleaseLink)
	}
	if req.Link != "" {
		u, err := url.Parse(strings.TrimSpace(req.Link))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.User != nil || strings.ContainsAny(req.Link, "\r\n\t") {
			return fmt.Errorf("%w: link must be an absolute HTTP(S) URL without credentials", ErrInvalidReleaseLink)
		}
	}
	if req.MagnetURI != "" {
		u, err := url.Parse(strings.TrimSpace(req.MagnetURI))
		if err != nil || !strings.EqualFold(u.Scheme, "magnet") ||
			!strings.HasPrefix(strings.ToLower(u.Query().Get("xt")), "urn:btih:") {
			return fmt.Errorf("%w: invalid magnet URI", ErrInvalidReleaseLink)
		}
	}
	if req.Infohash != "" && !validInfohash(req.Infohash) {
		return fmt.Errorf("%w: invalid torrent infohash", ErrInvalidReleaseLink)
	}
	if req.Link == "" && req.MagnetURI == "" && req.Infohash == "" {
		return fmt.Errorf("%w: a link, magnet URI, or infohash is required", ErrInvalidReleaseLink)
	}
	return nil
}

func validInfohash(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if ('0' > r || r > '9') && ('a' > r || r > 'f') && ('A' > r || r > 'F') {
			return false
		}
	}
	return true
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

func (e *Engine) searchMissingTracks(ctx context.Context, album *music.Album, artist *music.Artist, tracks []*music.Track) error {
	defs, err := e.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("list quality definitions: %w", err)
	}
	var profile *music.AudioQualityProfile
	profileID := albumQualityProfileID(album, artist)
	if profileID != "" {
		profile, err = e.repo.GetAudioQualityProfile(ctx, profileID)
		if err != nil {
			return fmt.Errorf("get audio quality profile: %w", err)
		}
		if profile == nil {
			return fmt.Errorf("%w: audio quality profile %q", ErrNotFound, profileID)
		}
	}

	for _, track := range tracks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if track == nil || !track.Monitored || track.HasFile {
			continue
		}
		if !e.beginAlbumSearch(album.ID + "|track|" + track.ID) {
			continue
		}
		if err := e.searchAndGrabTrack(ctx, album, artist, track, defs, profile); err != nil {
			e.logger.Debug("musicsearch: missing-track search failed",
				"album", album.ID, "track", track.ID, "title", track.Title, "error", err)
		}
		e.endAlbumSearch(album.ID + "|track|" + track.ID)
	}
	return nil
}

func (e *Engine) searchAndGrabTrack(
	ctx context.Context,
	album *music.Album,
	artist *music.Artist,
	track *music.Track,
	defs []*music.AudioQualityDefinition,
	profile *music.AudioQualityProfile,
) error {
	trackArtists := albumSearchArtists(album, artist)
	if track.ArtistName != "" {
		trackArtists = []string{track.ArtistName}
	}
	if len(trackArtists) == 0 {
		return ErrNoResults
	}
	var queries []indexers.Query
	seenTerms := make(map[string]bool)
	for _, trackArtist := range trackArtists {
		for _, query := range buildTrackQueries(trackArtist, album.Title, track.Title) {
			key := normalize(query.Term)
			if !seenTerms[key] {
				seenTerms[key] = true
				queries = append(queries, query)
			}
		}
	}
	seen := make(map[string]bool)
	var candidates []Candidate
	for _, query := range queries {
		requestArtist := firstTrackArtist(track, album, artist)
		result, err := e.dlRegistry.SearchAcquisition(ctx, e.indexerSvc, query, downloads.AddRequest{
			Title: requestArtist + " - " + track.Title, MediaType: "music", MediaID: album.ID,
		})
		if err != nil {
			return err
		}
		for _, res := range result.Results {
			key := res.IndexerID + "|" + res.Title
			if seen[key] {
				continue
			}
			seen[key] = true
			if !trackIdentityMatchAny(res.Title, trackArtists, track.Title) {
				continue
			}
			parsed := music.ParseMusic(res.Title)
			score := music.ScoreAudioRelease(parsed, defs, profile, seeders(res), res.Size)
			if !score.Allowed {
				continue
			}
			formatScore, formatMatches := e.formatScore(profile, parsed, &res)
			if profile != nil && formatScore < profile.MinFormatScore {
				continue
			}
			candidates = append(candidates, Candidate{
				Result: res, Parsed: parsed, Score: score,
				FormatScore: formatScore, FormatMatches: formatMatches,
			})
		}
		if len(candidates) > 0 {
			break
		}
	}
	if len(candidates) == 0 {
		return ErrNoResults
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].rankScore() > candidates[j].rankScore()
	})
	_, err := e.grab(ctx, album.ID, &candidates[0])
	return err
}

func (e *Engine) searchAndGrab(ctx context.Context, albumID string, upgradeOnly bool) (*GrabResult, error) {
	if !e.beginAlbumSearch(albumID) {
		return nil, ErrNoResults
	}
	defer e.endAlbumSearch(albumID)

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
	profileID := albumQualityProfileID(album, artist)
	if profileID != "" {
		profile, err = e.repo.GetAudioQualityProfile(ctx, profileID)
		if err != nil {
			return nil, fmt.Errorf("get audio quality profile: %w", err)
		}
		if profile == nil {
			return nil, fmt.Errorf("%w: audio quality profile %q", ErrNotFound, profileID)
		}
	}

	// In upgrade mode the profile must allow upgrades and the album must already
	// have files; the grab is gated on beating the current tier.
	minTier := -1
	if upgradeOnly {
		if profile == nil || !profile.UpgradeAllowed {
			return nil, ErrNoResults
		}
		cur, hasFiles, err := e.albumCurrentTier(ctx, album, defs)
		if err != nil {
			return nil, fmt.Errorf("get current album quality: %w", err)
		}
		if !hasFiles {
			return nil, ErrNoResults
		}
		minTier = cur
	}

	year := albumYear(album)
	candidates, err := e.gatherCandidates(ctx, albumID, album, artist, year, defs, profile)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		e.logger.Info("musicsearch: no matching releases", "artist", firstAlbumArtist(album, artist), "album", album.Title)
		return nil, ErrNoResults
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].rankScore() > candidates[j].rankScore()
	})
	best := candidates[0]

	if minTier >= 0 && best.Score.Tier <= minTier {
		e.logger.Debug("musicsearch: no upgrade available",
			"artist", firstAlbumArtist(album, artist), "album", album.Title, "current_tier", minTier, "best_tier", best.Score.Tier)
		return nil, ErrNoResults
	}

	grab, err := e.grab(ctx, albumID, &best)
	if err != nil {
		return nil, err
	}
	e.logger.Info("musicsearch: grabbed album release",
		"artist", firstAlbumArtist(album, artist), "album", album.Title, "upgrade", upgradeOnly,
		"title", grab.Title, "quality", grab.QualityName, "score", grab.CompositeScore,
	)
	return grab, nil
}

// loadAlbumArtist fetches an album and its optional managed owner. Standalone
// albums intentionally have no managed artist record; their credited artists
// live on the album.
func (e *Engine) loadAlbumArtist(ctx context.Context, albumID string) (*music.Album, *music.Artist, error) {
	album, err := e.repo.GetAlbum(ctx, albumID)
	if err != nil {
		return nil, nil, fmt.Errorf("get album: %w", err)
	}
	if album == nil {
		return nil, nil, ErrNotFound
	}
	if album.ArtistID == "" {
		return album, nil, nil
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
func (e *Engine) gatherCandidates(ctx context.Context, albumID string, album *music.Album, artist *music.Artist, year int, defs []*music.AudioQualityDefinition, profile *music.AudioQualityProfile) ([]Candidate, error) {
	for _, q := range buildAlbumQueries(album, artist, year) {
		artistName := firstAlbumArtist(album, artist)
		agg, err := e.dlRegistry.SearchAcquisition(ctx, e.indexerSvc, q, downloads.AddRequest{
			Title: artistName + " - " + album.Title, MediaType: "music", MediaID: albumID,
		})
		if err != nil {
			return nil, err
		}
		var candidates []Candidate
		seen := make(map[string]bool)
		for _, res := range agg.Results {
			key := res.IndexerID + "|" + res.Title
			if seen[key] {
				continue
			}
			seen[key] = true

			parsed := music.ParseMusic(res.Title)
			if !albumIdentityMatch(res.Title, parsed, album, artist) {
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
			return candidates, nil
		}
	}
	return nil, nil
}

func albumQualityProfileID(album *music.Album, artist *music.Artist) string {
	if artist != nil {
		return artist.QualityProfileID
	}
	if album == nil {
		return ""
	}
	return album.QualityProfileID
}

func albumSearchArtists(album *music.Album, artist *music.Artist) []string {
	if artist != nil && strings.TrimSpace(artist.Name) != "" {
		return []string{strings.TrimSpace(artist.Name)}
	}
	if album == nil {
		return nil
	}
	names := make([]string, 0, len(album.Artists)+1)
	seen := make(map[string]bool)
	add := func(name string) {
		name = strings.TrimSpace(name)
		key := normalize(name)
		if name != "" && !seen[key] {
			seen[key] = true
			names = append(names, name)
		}
	}
	if music.IsVariousArtistAlbum(album) {
		add("Various Artists")
	}
	for _, credit := range album.Artists {
		add(credit.Name)
	}
	return names
}

func firstAlbumArtist(album *music.Album, artist *music.Artist) string {
	names := albumSearchArtists(album, artist)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func firstTrackArtist(track *music.Track, album *music.Album, artist *music.Artist) string {
	if track != nil && strings.TrimSpace(track.ArtistName) != "" {
		return strings.TrimSpace(track.ArtistName)
	}
	return firstAlbumArtist(album, artist)
}

func buildAlbumQueries(album *music.Album, artist *music.Artist, year int) []indexers.Query {
	var queries []indexers.Query
	seen := make(map[string]bool)
	for _, name := range albumSearchArtists(album, artist) {
		for _, query := range buildAudioQueries(name, album.Title, year) {
			key := normalize(query.Term)
			if !seen[key] {
				seen[key] = true
				queries = append(queries, query)
			}
		}
	}
	return queries
}

func albumIdentityMatch(title string, parsed *music.MusicRelease, album *music.Album, artist *music.Artist) bool {
	if album == nil {
		return false
	}
	for _, name := range albumSearchArtists(album, artist) {
		if identityMatch(title, parsed, name, album.Title) {
			return true
		}
	}
	return false
}

func trackIdentityMatchAny(title string, artists []string, track string) bool {
	for _, artist := range artists {
		if trackIdentityMatch(title, artist, track) {
			return true
		}
	}
	return false
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
	return queries
}

func buildTrackQueries(artist, album, track string) []indexers.Query {
	base := func(term string) indexers.Query {
		return indexers.Query{
			Term: strings.TrimSpace(term), Mode: indexers.ModeSearch, Categories: audioCategories,
		}
	}
	return []indexers.Query{
		base(artist + " " + album + " " + track),
		base(artist + " " + track),
	}
}

func trackIdentityMatch(title, artist, track string) bool {
	return containsPhrase(title, artist) && containsPhrase(title, track)
}

// identityMatch verifies a result plausibly belongs to the requested album by
// requiring both the artist and album tokens to appear in the normalized title
// (or the parsed fields).
func identityMatch(title string, parsed *music.MusicRelease, artist, album string) bool {
	if parsed == nil {
		parsed = &music.MusicRelease{}
	}
	albumOK := album != "" && (containsPhrase(title, album) || containsPhrase(parsed.Album, album))
	if !albumOK {
		return false
	}
	if artist == "" {
		return true
	}
	if strings.TrimSpace(parsed.Artist) != "" {
		return containsPhrase(parsed.Artist, artist)
	}
	return containsPhrase(title, artist)
}

func (e *Engine) grab(ctx context.Context, albumID string, c *Candidate) (*GrabResult, error) {
	clients := e.dlRegistry.List()
	protocol := inferProtocol(&c.Result)
	var target downloads.DownloadClient
	if protocol != "" {
		for _, cl := range clients {
			if cl.Protocol() == protocol {
				target = cl
				break
			}
		}
	}

	req := buildDownloadRequest(&c.Result)
	req.MediaType = "music"
	req.Category = "music"
	req.MediaID = albumID
	req.IndexerID = c.Result.IndexerID
	// Never fall back to a client of a different protocol; Acquire still
	// records the failed attempt in download history when no client matches.
	clientID := ""
	if target != nil {
		clientID = target.ID()
	}
	addResult, err := e.dlRegistry.Acquire(ctx, clientID, req, nil)
	if err != nil {
		if target == nil {
			if len(clients) == 0 {
				return nil, fmt.Errorf("%w: %w", ErrNoDownloaders, err)
			}
			if protocol == "" {
				return nil, fmt.Errorf("%w: cannot determine release protocol for %q: %w", ErrNoCompatibleDownloader, c.Result.Title, err)
			}
			return nil, fmt.Errorf("%w: protocol %q: %w", ErrNoCompatibleDownloader, protocol, err)
		}
		return nil, fmt.Errorf("acquire album: %w", err)
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
	if res == nil {
		return ""
	}
	if res.MagnetURI != "" || res.Infohash != "" || res.Seeders != nil {
		return downloads.ProtocolTorrent
	}
	u, err := url.Parse(strings.TrimSpace(res.Link))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	path := strings.ToLower(u.EscapedPath())
	if strings.HasSuffix(path, ".nzb") {
		return downloads.ProtocolUsenet
	}
	if strings.HasSuffix(path, ".torrent") {
		return downloads.ProtocolTorrent
	}
	return ""
}

func buildDownloadRequest(res *indexers.Result) downloads.AddRequest {
	req := downloads.AddRequest{Title: res.Title, Infohash: res.Infohash}
	if res.MagnetURI != "" {
		req.Magnet = res.MagnetURI
	} else if res.Infohash != "" {
		req.Magnet = torrentdl.BuildPublicMagnet(res.Infohash, res.Title)
	}
	if res.Link != "" {
		switch {
		case strings.HasPrefix(strings.ToLower(res.Link), "magnet:"):
			if req.Magnet == "" {
				req.Magnet = res.Link
			}
		case inferProtocol(res) == downloads.ProtocolUsenet:
			req.NZBURL = res.Link
		case inferProtocol(res) == downloads.ProtocolTorrent:
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
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.IsMark(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsPhrase(value, normalizedPhrase string) bool {
	if normalizedPhrase == "" {
		return false
	}
	// Compare complete words, not substrings ("art" must not match "artist").
	var words []string
	var current strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		if unicode.IsMark(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	phrase := strings.Fields(normalizeToWords(normalizedPhrase))
	if len(phrase) == 0 || len(phrase) > len(words) {
		return false
	}
	for i := 0; i+len(phrase) <= len(words); i++ {
		matched := true
		for j := range phrase {
			if words[i+j] != phrase[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func normalizeToWords(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.IsMark(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), " ") {
			b.WriteByte(' ')
		}
	}
	return strings.TrimSpace(b.String())
}

func (e *Engine) beginAlbumSearch(albumID string) bool {
	e.searchMu.Lock()
	defer e.searchMu.Unlock()
	if e.searching[albumID] {
		return false
	}
	e.searching[albumID] = true
	return true
}

func (e *Engine) endAlbumSearch(albumID string) {
	e.searchMu.Lock()
	delete(e.searching, albumID)
	e.searchMu.Unlock()
}
