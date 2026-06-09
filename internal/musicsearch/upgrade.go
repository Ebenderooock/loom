package musicsearch

import (
	"context"
	"time"

	"github.com/ebenderooock/loom/internal/music"
)

// albumCurrentTier returns the minimum quality tier across an album's track
// files and whether the album currently has any files. The album's "current
// quality" is the weakest track, mirroring how upgrade-eligibility is judged
// per-album (the whole album is re-grabbed as a unit).
func (e *Engine) albumCurrentTier(ctx context.Context, album *music.Album, defs []*music.AudioQualityDefinition) (tier int, hasFiles bool) {
	files, err := e.repo.ListTrackFilesByArtist(ctx, album.ArtistID)
	if err != nil {
		return 0, false
	}
	for _, f := range files {
		if f.AlbumID != album.ID {
			continue
		}
		rel := &music.MusicRelease{Format: f.Format, Bitrate: f.Bitrate}
		t := 0
		if def := music.MatchAudioQuality(rel, defs); def != nil {
			t = def.TierOrder
		}
		if !hasFiles || t < tier {
			tier = t
		}
		hasFiles = true
	}
	return tier, hasFiles
}

// cutoffTierOf resolves a profile's cutoff definition to its tier order. A blank
// or unknown cutoff yields 0 (no effective cutoff).
func cutoffTierOf(profile *music.AudioQualityProfile, defs []*music.AudioQualityDefinition) int {
	if profile == nil || profile.Cutoff == "" {
		return 0
	}
	for _, d := range defs {
		if d.ID == profile.Cutoff {
			return d.TierOrder
		}
	}
	return 0
}

// CutoffUnmetCandidates returns complete monitored albums whose on-disk quality
// is below their artist's profile cutoff and whose profile allows upgrades. The
// recheck interval is honoured via album.LastSearchAt so upgrades do not hammer
// indexers.
func (e *Engine) CutoffUnmetCandidates(ctx context.Context, minRecheck time.Duration) ([]*music.Album, error) {
	artists, err := e.repo.ListArtists(ctx)
	if err != nil {
		return nil, err
	}
	defs, err := e.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var due []*music.Album
	for _, ar := range artists {
		if ar.MonitoringStatus != music.MonitoringMonitored || ar.QualityProfileID == "" {
			continue
		}
		profile, _ := e.repo.GetAudioQualityProfile(ctx, ar.QualityProfileID)
		if profile == nil || !profile.UpgradeAllowed {
			continue
		}
		cutoffTier := cutoffTierOf(profile, defs)
		if cutoffTier <= 0 {
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
			// Only complete albums are upgrade candidates; incomplete albums are
			// handled by the missing-search pass.
			missing, err := e.albumMissingTracks(ctx, al.ID)
			if err != nil || missing {
				continue
			}
			tier, hasFiles := e.albumCurrentTier(ctx, al, defs)
			if hasFiles && tier < cutoffTier {
				due = append(due, al)
			}
		}
	}
	sortAlbumsByLastSearch(due)
	return due, nil
}

// AutoSearchUpgrades searches up to limit cutoff-unmet albums for upgrades,
// returning how many were searched. Individual failures are logged and skipped.
func (e *Engine) AutoSearchUpgrades(ctx context.Context, limit int, minRecheck time.Duration) (int, error) {
	if limit <= 0 {
		limit = defaultAutoLimit
	}
	candidates, err := e.CutoffUnmetCandidates(ctx, minRecheck)
	if err != nil {
		return 0, err
	}
	searched := 0
	for _, al := range candidates {
		if ctx.Err() != nil || searched >= limit {
			break
		}
		if _, err := e.SearchAlbumUpgrade(ctx, al.ID); err != nil {
			e.logger.Debug("musicsearch: auto-upgrade album failed", "album", al.ID, "error", err)
		}
		searched++
	}
	if searched > 0 {
		e.logger.Info("musicsearch: auto-upgrade run complete", "searched", searched, "candidates", len(candidates))
	}
	return searched, nil
}
