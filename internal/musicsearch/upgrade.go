package musicsearch

import (
	"context"
	"fmt"
	"time"

	"github.com/ebenderooock/loom/internal/music"
)

// albumCurrentTier returns the minimum quality tier across an album's track
// files and whether the album currently has any files. The album's "current
// quality" is the weakest track, mirroring how upgrade-eligibility is judged
// per-album (the whole album is re-grabbed as a unit).
func (e *Engine) albumCurrentTier(ctx context.Context, album *music.Album, defs []*music.AudioQualityDefinition) (tier int, hasFiles bool, err error) {
	files, err := e.repo.ListTrackFilesByAlbum(ctx, album.ID)
	if err != nil {
		return 0, false, err
	}
	for _, f := range files {
		if f.AlbumID != album.ID {
			continue
		}
		rel := &music.MusicRelease{Format: f.Format, Bitrate: f.Bitrate}
		switch bits := f.MediaInfo["bits"].(type) {
		case int:
			rel.Bits = bits
		case int64:
			rel.Bits = int(bits)
		case float64:
			rel.Bits = int(bits)
		}
		t := 0
		if def := music.MatchAudioQuality(rel, defs); def != nil {
			t = def.TierOrder
		}
		if !hasFiles || t < tier {
			tier = t
		}
		hasFiles = true
	}
	return tier, hasFiles, nil
}

// cutoffTierOf resolves a profile's cutoff definition to its tier order. An
// unknown non-empty ID is invalid and must not silently become the lowest tier.
func cutoffTierOf(profile *music.AudioQualityProfile, defs []*music.AudioQualityDefinition) (int, bool) {
	if profile == nil || profile.Cutoff == "" {
		return 0, false
	}
	for _, d := range defs {
		if d.ID == profile.Cutoff {
			return d.TierOrder, true
		}
	}
	return 0, false
}

// CutoffUnmetCandidates returns complete monitored albums whose on-disk quality
// is below their effective profile cutoff and whose profile allows upgrades.
// Artist-owned albums require a monitored artist; standalone albums use their
// own profile and monitoring state. The recheck interval is honoured via
// album.LastSearchAt so upgrades do not hammer indexers.
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
		profile, err := e.repo.GetAudioQualityProfile(ctx, ar.QualityProfileID)
		if err != nil {
			return nil, fmt.Errorf("get audio quality profile for artist %s: %w", ar.ID, err)
		}
		if profile == nil || !profile.UpgradeAllowed {
			continue
		}
		cutoffTier, validCutoff := cutoffTierOf(profile, defs)
		if !validCutoff {
			continue
		}
		albums, err := e.repo.ListAlbumsByArtist(ctx, ar.ID)
		if err != nil {
			e.logger.Debug("musicsearch: list albums failed", "artist", ar.ID, "error", err)
			continue
		}
		for _, al := range albums {
			// A standalone album may appear in an artist's linked catalogue
			// through contributor credits. Its profile and monitoring are
			// evaluated independently below.
			if al.ArtistID != ar.ID {
				continue
			}
			isDue, err := e.isCutoffUnmetAlbum(ctx, al, cutoffTier, defs, minRecheck, now)
			if err != nil {
				return nil, err
			}
			if isDue {
				due = append(due, al)
			}
		}
	}
	standalone, err := e.repo.ListAlbums(ctx)
	if err != nil {
		return nil, err
	}
	for _, al := range standalone {
		if al.ArtistID != "" || !al.Monitored || al.QualityProfileID == "" {
			continue
		}
		profile, err := e.repo.GetAudioQualityProfile(ctx, al.QualityProfileID)
		if err != nil {
			return nil, fmt.Errorf("get audio quality profile for album %s: %w", al.ID, err)
		}
		if profile == nil || !profile.UpgradeAllowed {
			continue
		}
		cutoffTier, validCutoff := cutoffTierOf(profile, defs)
		if !validCutoff {
			continue
		}
		isDue, err := e.isCutoffUnmetAlbum(ctx, al, cutoffTier, defs, minRecheck, now)
		if err != nil {
			return nil, err
		}
		if isDue {
			due = append(due, al)
		}
	}
	sortAlbumsByLastSearch(due)
	return due, nil
}

func (e *Engine) isCutoffUnmetAlbum(
	ctx context.Context,
	album *music.Album,
	cutoffTier int,
	defs []*music.AudioQualityDefinition,
	minRecheck time.Duration,
	now time.Time,
) (bool, error) {
	if album == nil || !album.Monitored {
		return false, nil
	}
	if album.LastSearchAt != nil && now.Sub(*album.LastSearchAt) < minRecheck {
		return false, nil
	}
	// Only complete albums are upgrade candidates; incomplete albums are
	// handled by the missing-search pass.
	missing, err := e.albumMissingTracks(ctx, album.ID)
	if err != nil {
		return false, fmt.Errorf("check missing tracks for album %s: %w", album.ID, err)
	}
	if missing {
		return false, nil
	}
	tier, hasFiles, err := e.albumCurrentTier(ctx, album, defs)
	if err != nil {
		return false, fmt.Errorf("get track files for album %s: %w", album.ID, err)
	}
	return hasFiles && tier < cutoffTier, nil
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
