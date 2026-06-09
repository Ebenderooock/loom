package music

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Defaults for the new-release refresher. The cadence is deliberately slow to
// respect the MusicBrainz 1 req/s policy across a large library.
const (
	defaultRefreshInterval = 12 * time.Hour
	defaultRefreshSpacing  = 2 * time.Second
)

// ReleaseRefresher periodically re-fetches monitored artists' release-groups so
// newly released albums are discovered and (when matching the metadata profile)
// monitored. The music auto-searcher then acquires them. It is the music
// equivalent of an RSS new-release sync.
type ReleaseRefresher struct {
	svc      Service
	repo     Repository
	logger   *slog.Logger
	interval time.Duration
	spacing  time.Duration
	enabled  func() bool

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewReleaseRefresher constructs a refresher. enabled gates whether a run
// executes (typically the music feature flag); a nil enabled means always on.
func NewReleaseRefresher(svc Service, repo Repository, enabled func() bool, logger *slog.Logger) *ReleaseRefresher {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReleaseRefresher{
		svc:      svc,
		repo:     repo,
		logger:   logger,
		interval: defaultRefreshInterval,
		spacing:  defaultRefreshSpacing,
		enabled:  enabled,
	}
}

// Start launches the background loop. It returns immediately.
func (r *ReleaseRefresher) Start(ctx context.Context) {
	if r == nil || r.svc == nil || r.repo == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Unlock()
	go r.loop(runCtx)
}

// Stop halts the background loop.
func (r *ReleaseRefresher) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func (r *ReleaseRefresher) loop(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.enabled != nil && !r.enabled() {
				continue
			}
			r.runOnce(ctx)
		}
	}
}

// runOnce refreshes every monitored artist, spacing requests to respect the
// MusicBrainz rate limit. Errors on individual artists are logged and skipped.
func (r *ReleaseRefresher) runOnce(ctx context.Context) {
	artists, err := r.repo.ListArtists(ctx)
	if err != nil {
		r.logger.Warn("music: release refresh: list artists failed", "error", err)
		return
	}
	total := 0
	for _, a := range artists {
		if ctx.Err() != nil {
			return
		}
		if a.MonitoringStatus != MonitoringMonitored {
			continue
		}
		added, err := r.svc.RefreshArtistAlbums(ctx, a.ID)
		if err != nil {
			r.logger.Debug("music: release refresh failed", "artist", a.ID, "error", err)
		} else {
			total += added
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.spacing):
		}
	}
	if total > 0 {
		r.logger.Info("music: release refresh added new albums", "count", total)
	}
}
