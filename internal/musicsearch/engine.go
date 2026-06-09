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
	logger     *slog.Logger
}

// NewEngine constructs a music acquisition engine.
func NewEngine(indexerSvc *indexers.Service, dlRegistry *downloads.Registry, repo music.Repository, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{indexerSvc: indexerSvc, dlRegistry: dlRegistry, repo: repo, logger: logger}
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
	Result indexers.Result
	Parsed *music.MusicRelease
	Score  music.AudioScore
}

// SearchAlbum searches configured indexers for the given album and grabs the
// best-scoring, profile-allowed release.
func (e *Engine) SearchAlbum(ctx context.Context, albumID string) (*GrabResult, error) {
	album, err := e.repo.GetAlbum(ctx, albumID)
	if err != nil {
		return nil, fmt.Errorf("get album: %w", err)
	}
	if album == nil {
		return nil, ErrNotFound
	}
	artist, err := e.repo.GetArtist(ctx, album.ArtistID)
	if err != nil {
		return nil, fmt.Errorf("get artist: %w", err)
	}
	if artist == nil {
		return nil, ErrNotFound
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

	year := albumYear(album)
	candidates := e.gatherCandidates(ctx, artist.Name, album.Title, year, defs, profile)
	if len(candidates) == 0 {
		e.logger.Info("musicsearch: no matching releases", "artist", artist.Name, "album", album.Title)
		return nil, ErrNoResults
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score.Composite() > candidates[j].Score.Composite()
	})
	best := candidates[0]

	grab, err := e.grab(ctx, albumID, &best)
	if err != nil {
		return nil, err
	}
	e.logger.Info("musicsearch: grabbed album release",
		"artist", artist.Name, "album", album.Title,
		"title", grab.Title, "quality", grab.QualityName, "score", grab.CompositeScore,
	)
	return grab, nil
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
			candidates = append(candidates, Candidate{Result: res, Parsed: parsed, Score: score})
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
		CompositeScore: c.Score.Composite(),
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
