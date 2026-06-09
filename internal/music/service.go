package music

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/metadata"
)

// Common service errors.
var (
	ErrNotFound = errors.New("music: not found")
	ErrInvalid  = errors.New("music: invalid request")
)

// Service defines the music business logic.
type Service interface {
	ListArtists(ctx context.Context) ([]*Artist, error)
	GetArtist(ctx context.Context, id string) (*Artist, error)
	GetArtistByMBID(ctx context.Context, mbid string) (*Artist, error)
	LookupArtists(ctx context.Context, query string, limit int) ([]*ArtistLookupResult, error)
	LookupArtistByMBID(ctx context.Context, mbid string) (*ArtistLookupResult, error)
	AddArtist(ctx context.Context, req AddArtistRequest) (*Artist, error)
	UpdateArtist(ctx context.Context, id string, req UpdateArtistRequest) (*Artist, error)
	DeleteArtist(ctx context.Context, id string) error
	SetArtistMonitoring(ctx context.Context, id string, status MonitoringStatus) (*Artist, error)

	// RefreshArtistAlbums re-fetches an artist's release-groups and adds any
	// newly released albums (new-release discovery).
	RefreshArtistAlbums(ctx context.Context, artistID string) (int, error)

	GetAlbum(ctx context.Context, id string) (*Album, error)
	ListAlbumsByArtist(ctx context.Context, artistID string) ([]*Album, error)
	SetAlbumMonitored(ctx context.Context, id string, monitored bool) (*Album, error)

	// ImportTrackFile links a physical audio file to a track and marks the
	// track as present. Used by the music scanner/import pipeline.
	ImportTrackFile(ctx context.Context, tf *TrackFile) error

	ListAudioQualityDefinitions(ctx context.Context) ([]*AudioQualityDefinition, error)
	ListAudioQualityProfiles(ctx context.Context) ([]*AudioQualityProfile, error)
	UpdateAudioQualityProfile(ctx context.Context, id string, req UpdateAudioQualityProfileRequest) (*AudioQualityProfile, error)
	ListMetadataProfiles(ctx context.Context) ([]*MetadataProfile, error)
}

type service struct {
	repo     Repository
	provider metadata.MusicMetadataProvider
	logger   *slog.Logger
}

// NewService builds the music service. The provider must be non-nil for
// lookup/add to work; CRUD over already-imported data works without it.
func NewService(repo Repository, provider metadata.MusicMetadataProvider, logger *slog.Logger) Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &service{repo: repo, provider: provider, logger: logger}
}

func (s *service) ListArtists(ctx context.Context) ([]*Artist, error) {
	artists, err := s.repo.ListArtists(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := s.repo.GetAllArtistStats(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range artists {
		if st, ok := stats[a.ID]; ok {
			a.Stats = st
		} else {
			a.Stats = &ArtistStats{}
		}
	}
	return artists, nil
}

func (s *service) GetArtist(ctx context.Context, id string) (*Artist, error) {
	a, err := s.repo.GetArtist(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrNotFound
	}
	albums, err := s.repo.ListAlbumsByArtist(ctx, id)
	if err != nil {
		return nil, err
	}
	a.Albums = albums
	st, err := s.repo.GetArtistStats(ctx, id)
	if err != nil {
		return nil, err
	}
	a.Stats = st
	return a, nil
}

// GetArtistByMBID returns the library artist with the given MusicBrainz id, or
// nil if none exists. Used by request fulfillment to detect duplicates.
func (s *service) GetArtistByMBID(ctx context.Context, mbid string) (*Artist, error) {
	return s.repo.GetArtistByMBID(ctx, strings.TrimSpace(mbid))
}

func (s *service) LookupArtists(ctx context.Context, query string, limit int) ([]*ArtistLookupResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInvalid)
	}
	if s.provider == nil {
		return nil, fmt.Errorf("%w: no music metadata provider configured", ErrInvalid)
	}
	if limit <= 0 {
		limit = 15
	}
	results, err := s.provider.SearchArtist(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*ArtistLookupResult, 0, len(results))
	for _, m := range results {
		if m == nil {
			continue
		}
		added := false
		if existing, _ := s.repo.GetArtistByMBID(ctx, m.MBID); existing != nil && existing.MonitoringStatus != "" {
			// Treat a live (non-deleted) row as already added.
			if a, _ := s.repo.GetArtist(ctx, existing.ID); a != nil {
				added = true
			}
		}
		out = append(out, &ArtistLookupResult{
			MBID:           m.MBID,
			Name:           m.Name,
			Disambiguation: m.Disambiguation,
			Type:           m.Type,
			Country:        m.Country,
			Genres:         m.Genres,
			ImageURL:       m.ImageURL,
			AlreadyAdded:   added,
		})
	}
	return out, nil
}

// LookupArtistByMBID fetches a single artist's metadata by MusicBrainz ID for a
// trusted re-fetch (e.g. chat-bot request fulfilment). Marks already-added.
func (s *service) LookupArtistByMBID(ctx context.Context, mbid string) (*ArtistLookupResult, error) {
	mbid = strings.TrimSpace(mbid)
	if mbid == "" {
		return nil, fmt.Errorf("%w: mbid is required", ErrInvalid)
	}
	if s.provider == nil {
		return nil, fmt.Errorf("%w: no music metadata provider configured", ErrInvalid)
	}
	m, err := s.provider.GetArtist(ctx, mbid)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	added := false
	if existing, _ := s.repo.GetArtistByMBID(ctx, m.MBID); existing != nil && existing.MonitoringStatus != "" {
		if a, _ := s.repo.GetArtist(ctx, existing.ID); a != nil {
			added = true
		}
	}
	return &ArtistLookupResult{
		MBID:           m.MBID,
		Name:           m.Name,
		Disambiguation: m.Disambiguation,
		Type:           m.Type,
		Country:        m.Country,
		Genres:         m.Genres,
		ImageURL:       m.ImageURL,
		AlreadyAdded:   added,
	}, nil
}

func (s *service) AddArtist(ctx context.Context, req AddArtistRequest) (*Artist, error) {
	req.MBID = strings.TrimSpace(req.MBID)
	if req.MBID == "" {
		return nil, fmt.Errorf("%w: mbid is required", ErrInvalid)
	}
	if req.QualityProfileID == "" {
		return nil, fmt.Errorf("%w: qualityProfileId is required", ErrInvalid)
	}
	if req.LibraryID == "" {
		return nil, fmt.Errorf("%w: libraryId is required", ErrInvalid)
	}
	if s.provider == nil {
		return nil, fmt.Errorf("%w: no music metadata provider configured", ErrInvalid)
	}

	monitoring := MonitoringMonitored
	if req.MonitoringStatus == string(MonitoringUnmonitored) {
		monitoring = MonitoringUnmonitored
	}

	// Revive or short-circuit if this artist already exists (UNIQUE mbid).
	existing, err := s.repo.GetArtistByMBID(ctx, req.MBID)
	if err != nil {
		return nil, err
	}

	meta, err := s.provider.GetArtist(ctx, req.MBID)
	if err != nil {
		return nil, fmt.Errorf("fetch artist metadata: %w", err)
	}
	if meta == nil {
		return nil, ErrNotFound
	}

	var artist *Artist
	if existing != nil {
		// Update existing row (also clears deleted_at via UpdateArtist).
		existing.Name = meta.Name
		existing.SortName = meta.SortName
		existing.Disambiguation = meta.Disambiguation
		existing.ArtistType = meta.Type
		existing.Country = meta.Country
		existing.Overview = meta.Overview
		existing.Genres = meta.Genres
		existing.ImageURL = meta.ImageURL
		existing.LibraryID = req.LibraryID
		existing.QualityProfileID = req.QualityProfileID
		existing.MetadataProfileID = req.MetadataProfileID
		existing.MonitoringStatus = monitoring
		existing.MetadataProvider = s.provider.Name()
		if err := s.repo.UpdateArtist(ctx, existing); err != nil {
			return nil, err
		}
		artist = existing
	} else {
		artist = &Artist{
			ID:                uuid.New().String(),
			MBID:              meta.MBID,
			Name:              meta.Name,
			SortName:          meta.SortName,
			Disambiguation:    meta.Disambiguation,
			ArtistType:        meta.Type,
			Country:           meta.Country,
			Overview:          meta.Overview,
			Genres:            meta.Genres,
			ImageURL:          meta.ImageURL,
			LibraryID:         req.LibraryID,
			QualityProfileID:  req.QualityProfileID,
			MetadataProfileID: req.MetadataProfileID,
			MonitoringStatus:  monitoring,
			MetadataProvider:  s.provider.Name(),
		}
		if err := s.repo.CreateArtist(ctx, artist); err != nil {
			return nil, err
		}
	}

	// Persist albums (release-groups). Tracks are fetched lazily when an album
	// is opened/searched to respect the MusicBrainz rate limit.
	if n, err := s.syncArtistAlbums(ctx, artist, req.MetadataProfileID, monitoring); err != nil {
		s.logger.Warn("music: failed to fetch artist albums", "mbid", req.MBID, "error", err)
	} else if n > 0 {
		s.logger.Info("music: added albums", "artist", artist.Name, "count", n)
	}

	if req.Search {
		s.logger.Info("music: search-on-add requested (acquisition implemented in a later milestone)", "artist", artist.Name)
	}

	return s.GetArtist(ctx, artist.ID)
}

func (s *service) UpdateArtist(ctx context.Context, id string, req UpdateArtistRequest) (*Artist, error) {
	a, err := s.repo.GetArtist(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrNotFound
	}
	if req.MonitoringStatus != nil {
		a.MonitoringStatus = MonitoringStatus(*req.MonitoringStatus)
	}
	if req.QualityProfileID != nil {
		a.QualityProfileID = *req.QualityProfileID
	}
	if req.LibraryID != nil {
		a.LibraryID = *req.LibraryID
	}
	if req.MetadataProfileID != nil {
		a.MetadataProfileID = *req.MetadataProfileID
	}
	if err := s.repo.UpdateArtist(ctx, a); err != nil {
		return nil, err
	}
	return s.GetArtist(ctx, id)
}

func (s *service) DeleteArtist(ctx context.Context, id string) error {
	a, err := s.repo.GetArtist(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrNotFound
	}
	return s.repo.SoftDeleteArtist(ctx, id)
}

func (s *service) SetArtistMonitoring(ctx context.Context, id string, status MonitoringStatus) (*Artist, error) {
	if status != MonitoringMonitored && status != MonitoringUnmonitored {
		return nil, fmt.Errorf("%w: invalid monitoring status %q", ErrInvalid, status)
	}
	a, err := s.repo.GetArtist(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrNotFound
	}
	a.MonitoringStatus = status
	if err := s.repo.UpdateArtist(ctx, a); err != nil {
		return nil, err
	}
	return s.GetArtist(ctx, id)
}

func (s *service) GetAlbum(ctx context.Context, id string) (*Album, error) {
	al, err := s.repo.GetAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	if al == nil {
		return nil, ErrNotFound
	}

	// Lazily fetch the concrete releases (editions) for this album.
	if al.ReleasesFetchedAt == nil && al.MBID != "" && s.provider != nil {
		if err := s.fetchReleases(ctx, al); err != nil {
			s.logger.Warn("music: failed to fetch album releases", "album", al.ID, "error", err)
		}
	}
	releases, err := s.repo.ListReleasesByAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	al.Releases = releases

	// Lazily fetch the track list of the selected release.
	if al.TracksFetchedAt == nil && al.SelectedReleaseID != "" && s.provider != nil {
		if err := s.fetchTracks(ctx, al); err != nil {
			s.logger.Warn("music: failed to fetch album tracks", "album", al.ID, "error", err)
		}
	}
	tracks, err := s.repo.ListTracksByAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	al.Tracks = tracks
	return al, nil
}

// fetchReleases pulls the album's editions from the provider, stores them, and
// selects a preferred release for acquisition/track matching.
func (s *service) fetchReleases(ctx context.Context, al *Album) error {
	_, releaseMetas, err := s.provider.GetAlbum(ctx, al.MBID)
	if err != nil {
		return err
	}
	releases := make([]*AlbumRelease, 0, len(releaseMetas))
	for _, rm := range releaseMetas {
		if rm == nil || rm.MBID == "" {
			continue
		}
		releases = append(releases, &AlbumRelease{
			ID:             uuid.New().String(),
			MBID:           rm.MBID,
			AlbumID:        al.ID,
			Title:          rm.Title,
			Disambiguation: rm.Disambiguation,
			Status:         rm.Status,
			ReleaseDate:    rm.ReleaseDate,
			Country:        rm.Country,
			Label:          rm.Label,
			Format:         rm.Format,
			MediaCount:     rm.MediaCount,
			TrackCount:     rm.TrackCount,
		})
	}
	if err := s.repo.ReplaceReleases(ctx, al.ID, releases); err != nil {
		return err
	}
	now := time.Now().UTC()
	al.ReleasesFetchedAt = &now
	if selected := pickRelease(releases); selected != nil {
		al.SelectedReleaseID = selected.ID
	}
	// Re-fetching releases invalidates any previously fetched track list.
	al.TracksFetchedAt = nil
	return s.repo.UpdateAlbum(ctx, al)
}

// fetchTracks pulls the track list for the album's selected release.
func (s *service) fetchTracks(ctx context.Context, al *Album) error {
	releases, err := s.repo.ListReleasesByAlbum(ctx, al.ID)
	if err != nil {
		return err
	}
	var selectedMBID string
	for _, rel := range releases {
		if rel.ID == al.SelectedReleaseID {
			selectedMBID = rel.MBID
			break
		}
	}
	if selectedMBID == "" {
		return nil
	}
	relMeta, err := s.provider.GetAlbumRelease(ctx, selectedMBID)
	if err != nil {
		return err
	}
	tracks := make([]*Track, 0, len(relMeta.Tracks))
	for _, tm := range relMeta.Tracks {
		disc := tm.DiscNumber
		if disc == 0 {
			disc = 1
		}
		tracks = append(tracks, &Track{
			ID:            uuid.New().String(),
			RecordingMBID: tm.MBID,
			TrackMBID:     tm.TrackID,
			AlbumID:       al.ID,
			ReleaseID:     al.SelectedReleaseID,
			Title:         tm.Title,
			TrackNumber:   tm.TrackNumber,
			DiscNumber:    disc,
			DurationMs:    tm.DurationMs,
			ArtistName:    tm.ArtistName,
			Monitored:     al.Monitored,
		})
	}
	if err := s.repo.ReplaceTracks(ctx, al.ID, tracks); err != nil {
		return err
	}
	now := time.Now().UTC()
	al.TracksFetchedAt = &now
	return s.repo.UpdateAlbum(ctx, al)
}

func (s *service) SetAlbumMonitored(ctx context.Context, id string, monitored bool) (*Album, error) {
	al, err := s.repo.GetAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	if al == nil {
		return nil, ErrNotFound
	}
	al.Monitored = monitored
	if err := s.repo.UpdateAlbum(ctx, al); err != nil {
		return nil, err
	}
	return s.GetAlbum(ctx, id)
}

// ListAlbumsByArtist returns the albums belonging to an artist (no lazy fetch).
func (s *service) ListAlbumsByArtist(ctx context.Context, artistID string) ([]*Album, error) {
	return s.repo.ListAlbumsByArtist(ctx, artistID)
}

// ImportTrackFile persists a track file and flags its track as present.
func (s *service) ImportTrackFile(ctx context.Context, tf *TrackFile) error {
	if tf == nil || tf.FilePath == "" {
		return ErrInvalid
	}
	if existing, err := s.repo.GetTrackFileByPath(ctx, tf.FilePath); err == nil && existing != nil {
		return nil // already imported
	}
	if err := s.repo.CreateTrackFile(ctx, tf); err != nil {
		return err
	}
	if tf.TrackID != "" {
		if err := s.repo.MarkTrackHasFile(ctx, tf.TrackID, true); err != nil {
			s.logger.Warn("music: failed to mark track has_file", "track", tf.TrackID, "error", err)
		}
	}
	return nil
}

func (s *service) ListAudioQualityDefinitions(ctx context.Context) ([]*AudioQualityDefinition, error) {
	return s.repo.ListAudioQualityDefinitions(ctx)
}

func (s *service) ListAudioQualityProfiles(ctx context.Context) ([]*AudioQualityProfile, error) {
	return s.repo.ListAudioQualityProfiles(ctx)
}

// UpdateAudioQualityProfile updates the custom-format scoring, cutoff and
// upgrade policy of an existing audio quality profile.
func (s *service) UpdateAudioQualityProfile(ctx context.Context, id string, req UpdateAudioQualityProfileRequest) (*AudioQualityProfile, error) {
	p, err := s.repo.GetAudioQualityProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNotFound
	}
	if req.Cutoff != nil {
		p.Cutoff = *req.Cutoff
	}
	if req.UpgradeAllowed != nil {
		p.UpgradeAllowed = *req.UpgradeAllowed
	}
	if req.FormatItems != nil {
		p.FormatItems = req.FormatItems
	}
	if req.MinFormatScore != nil {
		p.MinFormatScore = *req.MinFormatScore
	}
	if err := s.repo.UpdateAudioQualityProfile(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.GetAudioQualityProfile(ctx, id)
}

func (s *service) ListMetadataProfiles(ctx context.Context) ([]*MetadataProfile, error) {
	return s.repo.ListMetadataProfiles(ctx)
}

// syncArtistAlbums fetches the artist's release-groups from the metadata
// provider and inserts any that are not yet stored, returning the number added.
// New albums respect the metadata profile and overall monitoring status. It is
// shared by AddArtist and the new-release refresher.
func (s *service) syncArtistAlbums(ctx context.Context, artist *Artist, metadataProfileID string, monitoring MonitoringStatus) (int, error) {
	if s.provider == nil {
		return 0, fmt.Errorf("no music metadata provider configured")
	}
	albums, err := s.provider.GetArtistAlbums(ctx, artist.MBID)
	if err != nil {
		return 0, err
	}
	var metaProfile *MetadataProfile
	if metadataProfileID != "" {
		metaProfile, _ = s.repo.GetMetadataProfile(ctx, metadataProfileID)
	}
	monitorAlbums := monitoring == MonitoringMonitored
	added := 0
	for _, am := range albums {
		if am == nil || am.MBID == "" {
			continue
		}
		if existingAl, _ := s.repo.GetAlbumByMBID(ctx, am.MBID); existingAl != nil {
			continue
		}
		// Album types outside the metadata profile are still stored but left
		// unmonitored, mirroring Lidarr's behaviour.
		monitored := monitorAlbums && albumMatchesMetadataProfile(am.Type, am.SecondaryTypes, metaProfile)
		al := &Album{
			ID:             uuid.New().String(),
			MBID:           am.MBID,
			ArtistID:       artist.ID,
			Title:          am.Title,
			AlbumType:      am.Type,
			SecondaryTypes: am.SecondaryTypes,
			ReleaseDate:    am.ReleaseDate,
			Genres:         am.Genres,
			CoverArtURL:    am.CoverArtURL,
			Monitored:      monitored,
		}
		if err := s.repo.CreateAlbum(ctx, al); err != nil {
			s.logger.Warn("music: failed to create album", "mbid", am.MBID, "error", err)
			continue
		}
		added++
	}
	return added, nil
}

// RefreshArtistAlbums re-fetches an artist's release-groups and adds any newly
// released albums (Lidarr-style new-release discovery). Returns the count added.
func (s *service) RefreshArtistAlbums(ctx context.Context, artistID string) (int, error) {
	artist, err := s.repo.GetArtist(ctx, artistID)
	if err != nil {
		return 0, err
	}
	if artist == nil {
		return 0, ErrNotFound
	}
	return s.syncArtistAlbums(ctx, artist, artist.MetadataProfileID, artist.MonitoringStatus)
}

// albumMatchesMetadataProfile reports whether an album's primary/secondary types
// are permitted by the metadata profile. A nil profile permits all types (so
// artists added before profiles were enforced keep their behaviour). The rule
// mirrors Lidarr: the primary type must be allowed (when the profile lists any),
// and every secondary type the album carries must also be allowed.
func albumMatchesMetadataProfile(primaryType string, secondaryTypes []string, profile *MetadataProfile) bool {
	if profile == nil {
		return true
	}
	if len(profile.PrimaryTypes) > 0 && !containsFold(profile.PrimaryTypes, primaryType) {
		return false
	}
	for _, st := range secondaryTypes {
		if strings.TrimSpace(st) == "" {
			continue
		}
		if !containsFold(profile.SecondaryTypes, st) {
			return false
		}
	}
	return true
}

func containsFold(set []string, v string) bool {
	for _, s := range set {
		if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

// pickRelease chooses a preferred edition: official status first, then the one
// with the most tracks, then the earliest release date.
func pickRelease(releases []*AlbumRelease) *AlbumRelease {
	var best *AlbumRelease
	for _, rel := range releases {
		if best == nil {
			best = rel
			continue
		}
		if releaseScore(rel) > releaseScore(best) {
			best = rel
		}
	}
	return best
}

func releaseScore(r *AlbumRelease) int {
	score := r.TrackCount
	if strings.EqualFold(r.Status, "official") {
		score += 10000
	}
	return score
}
