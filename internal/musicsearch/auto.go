package musicsearch

import (
	"context"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/music"
)

// Auto-search defaults. These mirror the conservative cadence used by the
// movie/TV rolling searcher so music acquisition does not overwhelm indexers.
const (
	defaultAutoInterval = 6 * time.Hour
	defaultAutoLimit    = 3
	defaultMinRecheck   = 12 * time.Hour
)

// touchLastSearch records that an album was searched, so the auto-searcher can
// honour a recheck interval. Best-effort; failures are logged at debug level.
func (e *Engine) touchLastSearch(ctx context.Context, album *music.Album) {
	now := time.Now()
	album.LastSearchAt = &now
	uctx := context.WithoutCancel(ctx)
	if err := e.repo.TouchAlbumSearch(uctx, album.ID, now); err != nil {
		e.logger.Debug("musicsearch: update album last_search_at failed", "album", album.ID, "error", err)
	}
}

// MissingAlbumCandidates returns monitored albums (under monitored artists) that
// have at least one missing tracked file and are due for a (re)search per
// minRecheck. Results are ordered oldest-search-first so attention rotates.
func (e *Engine) MissingAlbumCandidates(ctx context.Context, minRecheck time.Duration) ([]*music.Album, error) {
	artists, err := e.repo.ListArtists(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var due []*music.Album
	for _, ar := range artists {
		if ar.MonitoringStatus != music.MonitoringMonitored {
			continue
		}
		albums, err := e.repo.ListAlbumsByArtist(ctx, ar.ID)
		if err != nil {
			e.logger.Debug("musicsearch: list albums failed", "artist", ar.ID, "error", err)
			continue
		}
		for _, al := range albums {
			if !al.Monitored {
				continue
			}
			if al.LastSearchAt != nil && now.Sub(*al.LastSearchAt) < minRecheck {
				continue
			}
			tracks, err := e.ensureAlbumTracks(ctx, al)
			if err != nil {
				e.logger.Debug("musicsearch: track metadata check failed", "album", al.ID, "error", err)
				continue
			}
			al.Tracks = tracks
			if anyMissingTrack(tracks) {
				due = append(due, al)
			}
		}
	}
	sortAlbumsByLastSearch(due)
	return due, nil
}

func (e *Engine) albumMissingTracks(ctx context.Context, albumID string) (bool, error) {
	tracks, err := e.repo.ListTracksByAlbum(ctx, albumID)
	if err != nil {
		return false, err
	}
	hasTracked := false
	for _, t := range tracks {
		if !t.Monitored {
			continue
		}
		hasTracked = true
		if !t.HasFile {
			return true, nil
		}
	}
	return hasTracked && anyMissingTrack(tracks), nil
}

func (e *Engine) ensureAlbumTracks(ctx context.Context, album *music.Album) ([]*music.Track, error) {
	tracks, err := e.repo.ListTracksByAlbum(ctx, album.ID)
	if err != nil {
		return nil, err
	}
	if len(tracks) > 0 || album.TracksFetchedAt != nil || e.albumLoader == nil {
		return tracks, nil
	}
	loaded, err := e.albumLoader.GetAlbum(ctx, album.ID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, nil
	}
	album.TracksFetchedAt = loaded.TracksFetchedAt
	return loaded.Tracks, nil
}

func albumNeedsInitialGrab(tracks []*music.Track) bool {
	hasMonitored := false
	for _, track := range tracks {
		if track == nil || !track.Monitored {
			continue
		}
		hasMonitored = true
		if track.HasFile {
			return false
		}
	}
	return hasMonitored
}

func anyMissingTrack(tracks []*music.Track) bool {
	for _, track := range tracks {
		if track != nil && track.Monitored && !track.HasFile {
			return true
		}
	}
	return false
}

// AutoSearchMissing searches up to limit due albums, returning how many were
// searched. Errors on individual albums are logged and skipped.
func (e *Engine) AutoSearchMissing(ctx context.Context, limit int, minRecheck time.Duration) (int, error) {
	if limit <= 0 {
		limit = defaultAutoLimit
	}

	candidates, err := e.MissingAlbumCandidates(ctx, minRecheck)
	if err != nil {
		return 0, err
	}
	searched := 0
	for _, al := range candidates {
		if ctx.Err() != nil {
			break
		}
		if searched >= limit {
			break
		}
		if albumNeedsInitialGrab(al.Tracks) {
			if _, err := e.SearchAlbum(ctx, al.ID); err != nil {
				e.logger.Debug("musicsearch: auto-search album failed", "album", al.ID, "error", err)
			}
		} else {
			album, artist, err := e.loadAlbumArtist(ctx, al.ID)
			if err != nil {
				e.logger.Debug("musicsearch: auto-search album lookup failed", "album", al.ID, "error", err)
			} else if err := e.searchMissingTracks(ctx, album, artist, al.Tracks); err != nil {
				e.logger.Debug("musicsearch: auto-search missing tracks failed", "album", al.ID, "error", err)
			}
			e.touchLastSearch(ctx, al)
		}
		searched++
	}
	if searched > 0 {
		e.logger.Info("musicsearch: auto-search run complete", "searched", searched, "candidates", len(candidates))
	}
	return searched, nil
}

// SearchArtistAcquisition runs a bounded search-on-add pass for one artist.
// Only albums with known track listings are eligible. Complete missing albums
// are searched as a whole; partially imported albums use track-level queries.
func (e *Engine) SearchArtistAcquisition(ctx context.Context, artistID string, limit int) error {
	if limit <= 0 {
		limit = defaultAutoLimit
	}
	artist, err := e.repo.GetArtist(ctx, artistID)
	if err != nil {
		return err
	}
	if artist == nil {
		return ErrNotFound
	}
	if artist.MonitoringStatus != music.MonitoringMonitored {
		return nil
	}
	albums, err := e.repo.ListAlbumsByArtist(ctx, artistID)
	if err != nil {
		return err
	}
	attempted := 0
	for _, album := range albums {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !album.Monitored || attempted >= limit {
			continue
		}
		tracks, err := e.ensureAlbumTracks(ctx, album)
		if err != nil {
			e.logger.Debug("musicsearch: search-on-add metadata load failed", "album", album.ID, "error", err)
			continue
		}
		if !anyMissingTrack(tracks) {
			continue
		}
		attempted++
		if albumNeedsInitialGrab(tracks) {
			if _, err := e.SearchAlbum(ctx, album.ID); err != nil {
				e.logger.Debug("musicsearch: search-on-add album failed", "album", album.ID, "error", err)
			}
		} else {
			if err := e.searchMissingTracks(ctx, album, artist, tracks); err != nil {
				e.logger.Debug("musicsearch: search-on-add missing tracks failed", "album", album.ID, "error", err)
			}
			e.touchLastSearch(ctx, album)
			continue
		}
	}
	return nil
}

func sortAlbumsByLastSearch(albums []*music.Album) {
	// Insertion sort keeps it dependency-free; candidate lists are small.
	for i := 1; i < len(albums); i++ {
		for j := i; j > 0 && lastSearchBefore(albums[j], albums[j-1]); j-- {
			albums[j], albums[j-1] = albums[j-1], albums[j]
		}
	}
}

func lastSearchBefore(a, b *music.Album) bool {
	if a.LastSearchAt == nil {
		return true
	}
	if b.LastSearchAt == nil {
		return false
	}
	return a.LastSearchAt.Before(*b.LastSearchAt)
}

// AutoSearcher periodically runs AutoSearchMissing in the background. It is a
// self-contained analogue of scheduler.RollingSearcher for the music domain.
type AutoSearcher struct {
	engine     *Engine
	interval   time.Duration
	limit      int
	minRecheck time.Duration
	enabled    func() bool

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewAutoSearcher constructs a music auto-searcher. enabled gates whether a run
// executes (typically the "music" feature flag); a nil enabled means always on.
func NewAutoSearcher(engine *Engine, enabled func() bool) *AutoSearcher {
	return &AutoSearcher{
		engine:     engine,
		interval:   defaultAutoInterval,
		limit:      defaultAutoLimit,
		minRecheck: defaultMinRecheck,
		enabled:    enabled,
	}
}

// Start launches the background loop. It returns immediately.
func (a *AutoSearcher) Start(ctx context.Context) {
	if a == nil || a.engine == nil {
		return
	}
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.mu.Unlock()
	go a.loop(runCtx)
}

// Stop halts the background loop.
func (a *AutoSearcher) Stop() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *AutoSearcher) loop(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.enabled != nil && !a.enabled() {
				continue
			}
			if _, err := a.engine.AutoSearchMissing(ctx, a.limit, a.minRecheck); err != nil {
				a.engine.logger.Warn("musicsearch: auto-search run failed", "error", err)
			}
			if _, err := a.engine.AutoSearchUpgrades(ctx, a.limit, a.minRecheck); err != nil {
				a.engine.logger.Warn("musicsearch: auto-upgrade run failed", "error", err)
			}
		}
	}
}
