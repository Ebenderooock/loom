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
	if err := e.repo.UpdateAlbum(uctx, album); err != nil {
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
			missing, err := e.albumMissingTracks(ctx, al.ID)
			if err != nil {
				e.logger.Debug("musicsearch: track check failed", "album", al.ID, "error", err)
				continue
			}
			if missing {
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
	// If an album has no track listing yet, treat it as missing so a search can
	// kick off acquisition; if it has tracks and all are present, it's complete.
	return !hasTracked, nil
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
		if _, err := e.SearchAlbum(ctx, al.ID); err != nil {
			e.logger.Debug("musicsearch: auto-search album failed", "album", al.ID, "error", err)
		}
		searched++
	}
	if searched > 0 {
		e.logger.Info("musicsearch: auto-search run complete", "searched", searched, "candidates", len(candidates))
	}
	return searched, nil
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
		}
	}
}
