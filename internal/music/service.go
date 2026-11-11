package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"os"
	"path/filepath"

	"github.com/ebenderooock/loom/internal/metadata"
)

// Common service errors.
var (
	ErrNotFound = errors.New("music: not found")
	ErrInvalid  = errors.New("music: invalid request")
	// ErrAlbumTracksUnavailable means the album's track list could not be
	// resolved, usually because the metadata provider was unavailable.
	ErrAlbumTracksUnavailable = errors.New("music: album track list unavailable")
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

	LookupAlbums(ctx context.Context, query string, limit int) ([]*AlbumLookupResult, error)
	AddAlbum(ctx context.Context, req AddAlbumRequest) (*Album, error)
	ListAlbums(ctx context.Context) ([]*Album, error)

	// RefreshArtistAlbums re-fetches an artist's release-groups and adds any
	// newly released albums (new-release discovery).
	RefreshArtistAlbums(ctx context.Context, artistID string) (int, error)

	GetAlbum(ctx context.Context, id string) (*Album, error)
	GetTrack(ctx context.Context, id string) (*Track, error)
	// LoadAlbumTracks is GetAlbum for callers that must match files to tracks:
	// it returns an error wrapping ErrAlbumTracksUnavailable instead of an
	// album with no tracks when metadata cannot be fetched.
	LoadAlbumTracks(ctx context.Context, id string) (*Album, error)
	ListAlbumsByArtist(ctx context.Context, artistID string) ([]*Album, error)
	SetAlbumMonitored(ctx context.Context, id string, monitored bool) (*Album, error)
	// SetTrackMonitored toggles one track. Monitoring a track also monitors
	// its album so the album-level searcher will consider it.
	SetTrackMonitored(ctx context.Context, albumID, trackID string, monitored bool) (*Track, error)
	SelectAlbumRelease(ctx context.Context, albumID, releaseID string) (*Album, error)

	// ImportTrackFile links a physical audio file to a track and marks the
	// track as present. Used by the music scanner/import pipeline.
	ImportTrackFile(ctx context.Context, tf *TrackFile) error
	ListTrackFilesByAlbum(ctx context.Context, albumID string) ([]*TrackFile, error)
	ListTrackFilesByArtist(ctx context.Context, artistID string) ([]*TrackFile, error)
	ListArtistFiles(ctx context.Context, artistID string) ([]*ArtistFile, error)
	ListArtistHistory(ctx context.Context, artistID string, limit int) ([]*ArtistHistoryEntry, error)
	DeleteTrackFile(ctx context.Context, id string) error
	PruneMissingTrackFiles(ctx context.Context, artistID, root string) (int, error)
	PruneMissingAlbumTrackFiles(ctx context.Context, albumID, root string) (int, error)

	ListAudioQualityDefinitions(ctx context.Context) ([]*AudioQualityDefinition, error)
	ListAudioQualityProfiles(ctx context.Context) ([]*AudioQualityProfile, error)
	CreateAudioQualityProfile(ctx context.Context, req CreateAudioQualityProfileRequest) (*AudioQualityProfile, error)
	UpdateAudioQualityProfile(ctx context.Context, id string, req UpdateAudioQualityProfileRequest) (*AudioQualityProfile, error)
	DeleteAudioQualityProfile(ctx context.Context, id string) error
	ListMetadataProfiles(ctx context.Context) ([]*MetadataProfile, error)
	CreateMetadataProfile(ctx context.Context, req CreateMetadataProfileRequest) (*MetadataProfile, error)
	UpdateMetadataProfile(ctx context.Context, id string, req UpdateMetadataProfileRequest) (*MetadataProfile, error)
	DeleteMetadataProfile(ctx context.Context, id string) error
}

// PruneMissingTrackFiles removes stale track-file records under root and
// repairs their tracks' has_file state. It is called only after a successful
// scan of that folder, so inaccessible directories are never treated as empty.
func (s *service) PruneMissingTrackFiles(ctx context.Context, artistID, root string) (int, error) {
	files, err := s.repo.ListTrackFilesByArtist(ctx, artistID)
	if err != nil {
		return 0, err
	}
	return pruneMissingTrackFiles(ctx, s.repo, files, root)
}

// PruneMissingAlbumTrackFiles removes stale records for one album. This is
// used for standalone and Various Artists folders where no single managed
// artist owns the physical directory.
func (s *service) PruneMissingAlbumTrackFiles(ctx context.Context, albumID, root string) (int, error) {
	files, err := s.repo.ListTrackFilesByAlbum(ctx, albumID)
	if err != nil {
		return 0, err
	}
	return pruneMissingTrackFiles(ctx, s.repo, files, root)
}

func pruneMissingTrackFiles(ctx context.Context, repo Repository, files []*TrackFile, root string) (int, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		pathAbs, err := filepath.Abs(file.FilePath)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, pathAbs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(pathAbs); err == nil || !os.IsNotExist(err) {
			continue
		}
		if err := repo.DeleteTrackFile(ctx, file.ID); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

type service struct {
	repo        Repository
	provider    metadata.MusicMetadataProvider
	logger      *slog.Logger
	searchOnAdd func(context.Context, string) error
	artwork     metadata.MusicArtworkProvider
}

func (s *service) GetTrack(ctx context.Context, id string) (*Track, error) {
	track, err := s.repo.GetTrack(ctx, id)
	if err != nil {
		return nil, err
	}
	if track == nil {
		return nil, ErrNotFound
	}
	return track, nil
}

// artworkTimeout bounds best-effort artwork lookups so a slow artwork service
// never stalls an add, refresh or album load.
const artworkTimeout = 10 * time.Second

// NewService builds the music service. The provider must be non-nil for
// lookup/add to work; CRUD over already-imported data works without it.
func NewService(repo Repository, provider metadata.MusicMetadataProvider, logger *slog.Logger) Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &service{repo: repo, provider: provider, logger: logger}
}

// SetSearchOnAdd installs the bounded acquisition hook used by the search-on-add
// option. It is intentionally optional so metadata-only/test configurations do
// not start searches without a wired indexer and download stack.
func (s *service) SetSearchOnAdd(search func(context.Context, string) error) {
	s.searchOnAdd = search
}

// SetArtwork installs the optional artwork source (fanart.tv / TheAudioDB).
func (s *service) SetArtwork(p metadata.MusicArtworkProvider) {
	s.artwork = p
}

// applyArtistMetadata copies provider metadata onto an artist record.
func applyArtistMetadata(a *Artist, meta *metadata.ArtistMetadata) {
	a.Name = meta.Name
	a.SortName = meta.SortName
	a.Disambiguation = meta.Disambiguation
	a.ArtistType = meta.Type
	a.Country = meta.Country
	a.Genres = meta.Genres
	a.BeginArea = meta.BeginArea
	a.BeginDate = meta.BeginDate
	a.EndDate = meta.EndDate
	a.Ended = meta.Ended
	a.Rating = meta.Rating
	a.RatingVotes = meta.RatingVotes
	a.Links = externalLinksFrom(meta.Links)
	if meta.Overview != "" {
		a.Overview = meta.Overview
	}
	if meta.ImageURL != "" && a.ImageURL == "" {
		a.ImageURL = meta.ImageURL
	}
}

// enrichArtist applies artwork and biography from the artwork source. Failures
// are logged and leave the existing values untouched. It returns any album
// covers the source supplied, keyed by release-group MBID.
func (s *service) enrichArtist(ctx context.Context, a *Artist, fallbackImage string) map[string]string {
	if s.artwork == nil || a.MBID == "" {
		if a.ImageURL == "" {
			a.ImageURL = fallbackImage
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, artworkTimeout)
	defer cancel()
	art, err := s.artwork.ArtistArtwork(ctx, a.MBID)
	if err != nil {
		s.logger.Warn("music: artist artwork lookup failed", "artist", a.Name, "error", err)
	}
	if art == nil {
		if a.ImageURL == "" {
			a.ImageURL = fallbackImage
		}
		return nil
	}
	if art.ThumbURL != "" {
		a.ImageURL = art.ThumbURL
	} else if a.ImageURL == "" {
		a.ImageURL = fallbackImage
	}
	if art.BackgroundURL != "" {
		a.BackgroundURL = art.BackgroundURL
	}
	if art.LogoURL != "" {
		a.LogoURL = art.LogoURL
	}
	if art.Biography != "" {
		a.Overview = art.Biography
	}
	return art.AlbumCovers
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
	for _, album := range albums {
		setAlbumOwnerCredit(album, a)
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

// LookupAlbums searches MusicBrainz release-groups without requiring a
// managed primary artist. Credited contributors remain metadata rather than
// implicit library entities.
func (s *service) LookupAlbums(ctx context.Context, query string, limit int) ([]*AlbumLookupResult, error) {
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
	results, err := s.provider.SearchAlbum(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*AlbumLookupResult, 0, len(results))
	for _, result := range results {
		if result == nil || result.MBID == "" {
			continue
		}
		credits, err := s.albumCredits(ctx, result.Artists, result.ArtistMBID, result.ArtistName)
		if err != nil {
			return nil, err
		}
		existing, err := s.repo.GetAlbumByMBID(ctx, result.MBID)
		if err != nil {
			return nil, err
		}
		out = append(out, &AlbumLookupResult{
			MBID: result.MBID, Title: result.Title, Artists: credits,
			AlbumType: result.Type, ReleaseDate: result.ReleaseDate,
			CoverArtURL: result.CoverArtURL, AlreadyAdded: existing != nil,
		})
	}
	return out, nil
}

// AddAlbum adds a release-group as an independent library item. The complete
// artist credit is stored, but no contributor is promoted to a managed artist.
func (s *service) AddAlbum(ctx context.Context, req AddAlbumRequest) (*Album, error) {
	req.MBID = strings.TrimSpace(req.MBID)
	if req.MBID == "" {
		return nil, fmt.Errorf("%w: mbid is required", ErrInvalid)
	}
	req.QualityProfileID = strings.TrimSpace(req.QualityProfileID)
	if req.QualityProfileID == "" {
		return nil, fmt.Errorf("%w: qualityProfileId is required", ErrInvalid)
	}
	req.LibraryID = strings.TrimSpace(req.LibraryID)
	if req.LibraryID == "" {
		return nil, fmt.Errorf("%w: libraryId is required", ErrInvalid)
	}
	if s.provider == nil {
		return nil, fmt.Errorf("%w: no music metadata provider configured", ErrInvalid)
	}
	existing, err := s.repo.GetAlbumByMBID(ctx, req.MBID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return s.GetAlbum(ctx, existing.ID)
	}
	meta, _, err := s.provider.GetAlbum(ctx, req.MBID)
	if err != nil {
		return nil, fmt.Errorf("fetch album metadata: %w", err)
	}
	if meta == nil {
		return nil, ErrNotFound
	}
	credits, err := s.albumCredits(ctx, meta.Artists, meta.ArtistMBID, meta.ArtistName)
	if err != nil {
		return nil, err
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}
	album := &Album{
		ID: uuid.New().String(), MBID: meta.MBID, Title: meta.Title,
		AlbumType: meta.Type, SecondaryTypes: meta.SecondaryTypes,
		ReleaseDate: meta.ReleaseDate, Genres: meta.Genres,
		CoverArtURL: meta.CoverArtURL, Rating: meta.Rating,
		RatingVotes: meta.RatingVotes, Links: externalLinksFrom(meta.Links),
		Monitored: monitored, Artists: credits, LibraryID: req.LibraryID,
		QualityProfileID:  req.QualityProfileID,
		MetadataProfileID: req.MetadataProfileID,
	}
	if album.MBID == "" {
		album.MBID = req.MBID
	}
	s.applyAlbumDetails(ctx, album, meta)
	if err := s.repo.CreateAlbum(ctx, album); err != nil {
		return nil, err
	}
	return s.GetAlbum(ctx, album.ID)
}

func (s *service) ListAlbums(ctx context.Context) ([]*Album, error) {
	albums, err := s.repo.ListAlbums(ctx)
	if err != nil {
		return nil, err
	}
	artists, err := s.repo.ListArtists(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Artist, len(artists))
	for _, artist := range artists {
		byID[artist.ID] = artist
	}
	for _, album := range albums {
		setAlbumOwnerCredit(album, byID[album.ArtistID])
	}
	return albums, nil
}

func setAlbumOwnerCredit(album *Album, artist *Artist) {
	if album == nil || artist == nil || album.ArtistID != artist.ID || len(album.Artists) > 0 {
		return
	}
	album.Artists = AlbumArtistCredits{{
		ArtistID: artist.ID, MBID: artist.MBID, Name: artist.Name,
	}}
}

func (s *service) populateAlbumOwnerCredit(ctx context.Context, album *Album) error {
	if album == nil || album.ArtistID == "" || len(album.Artists) > 0 {
		return nil
	}
	artist, err := s.repo.GetArtist(ctx, album.ArtistID)
	if err != nil {
		return err
	}
	setAlbumOwnerCredit(album, artist)
	return nil
}

// albumCredits links only artists that are already managed. It never creates
// artist records as a side effect of adding an album.
func (s *service) albumCredits(ctx context.Context, artists []metadata.AlbumArtistMetadata, fallbackMBID, fallbackName string) (AlbumArtistCredits, error) {
	if len(artists) == 0 && (fallbackMBID != "" || fallbackName != "") {
		artists = []metadata.AlbumArtistMetadata{{ArtistMBID: fallbackMBID, Name: fallbackName}}
	}
	credits := make(AlbumArtistCredits, 0, len(artists))
	seen := make(map[string]struct{}, len(artists))
	for _, artist := range artists {
		name := strings.TrimSpace(artist.Name)
		mbid := strings.TrimSpace(artist.ArtistMBID)
		if name == "" && mbid == "" {
			continue
		}
		if mbid != "" {
			if _, ok := seen[mbid]; ok {
				continue
			}
			seen[mbid] = struct{}{}
		}
		credit := AlbumArtist{MBID: mbid, Name: name, JoinPhrase: artist.JoinPhrase}
		if mbid != "" {
			managed, err := s.repo.GetArtistByMBID(ctx, mbid)
			if err != nil {
				return nil, err
			}
			if managed != nil {
				active, err := s.repo.GetArtist(ctx, managed.ID)
				if err != nil {
					return nil, err
				}
				if active != nil {
					credit.ArtistID = active.ID
				}
			}
		}
		credits = append(credits, credit)
	}
	return credits, nil
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
	if req.MonitoringStatus != "" && req.MonitoringStatus != string(MonitoringMonitored) &&
		req.MonitoringStatus != string(MonitoringUnmonitored) {
		return nil, fmt.Errorf("%w: invalid monitoring status %q", ErrInvalid, req.MonitoringStatus)
	}
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
	var albumCovers map[string]string
	if existing != nil {
		// Update existing row (also clears deleted_at via UpdateArtist).
		applyArtistMetadata(existing, meta)
		albumCovers = s.enrichArtist(ctx, existing, meta.ImageURL)
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
			LibraryID:         req.LibraryID,
			QualityProfileID:  req.QualityProfileID,
			MetadataProfileID: req.MetadataProfileID,
			MonitoringStatus:  monitoring,
			MetadataProvider:  s.provider.Name(),
		}
		applyArtistMetadata(artist, meta)
		albumCovers = s.enrichArtist(ctx, artist, meta.ImageURL)
		if err := s.repo.CreateArtist(ctx, artist); err != nil {
			return nil, err
		}
	}

	// Persist albums (release-groups). Tracks are fetched lazily when an album
	// is opened/searched to respect the MusicBrainz rate limit.
	n, err := s.syncArtistAlbums(ctx, artist, req.MetadataProfileID, monitoring, albumCovers)
	if err != nil {
		return nil, fmt.Errorf("sync artist albums: %w", err)
	}
	if n > 0 {
		s.logger.Info("music: added albums", "artist", artist.Name, "count", n)
	}

	if req.Search {
		if s.searchOnAdd == nil {
			s.logger.Warn("music: search-on-add requested but acquisition is not wired", "artist", artist.Name)
		} else if err := s.searchOnAdd(ctx, artist.ID); err != nil {
			s.logger.Warn("music: search-on-add did not complete", "artist", artist.Name, "error", err)
		}
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
		if *req.MonitoringStatus != string(MonitoringMonitored) && *req.MonitoringStatus != string(MonitoringUnmonitored) {
			return nil, fmt.Errorf("%w: invalid monitoring status %q", ErrInvalid, *req.MonitoringStatus)
		}
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
	if req.Path != nil {
		a.Path = strings.TrimSpace(*req.Path)
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
	al, _, err := s.loadAlbum(ctx, id)
	return al, err
}

func (s *service) LoadAlbumTracks(ctx context.Context, id string) (*Album, error) {
	al, fetchErr, err := s.loadAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(al.Tracks) > 0 {
		return al, nil
	}
	if fetchErr != nil {
		return nil, fmt.Errorf("%w for %q: %w", ErrAlbumTracksUnavailable, al.Title, fetchErr)
	}
	return nil, fmt.Errorf("%w for %q: no tracks found for the selected release", ErrAlbumTracksUnavailable, al.Title)
}

// loadAlbum returns the album with releases and tracks, lazily fetching them
// from the provider. Provider failures are logged and returned separately as
// fetchErr so read-only callers still get the stored album.
func (s *service) loadAlbum(ctx context.Context, id string) (al *Album, fetchErr error, err error) {
	al, err = s.repo.GetAlbum(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if al == nil {
		return nil, nil, ErrNotFound
	}
	if err := s.populateAlbumOwnerCredit(ctx, al); err != nil {
		return nil, nil, err
	}

	// Lazily fetch the concrete releases (editions) for this album.
	// Albums stored before details were tracked are refreshed once to pick up
	// links, rating and artwork; release IDs and selection are preserved.
	if (al.ReleasesFetchedAt == nil || al.DetailsFetchedAt == nil) && al.MBID != "" && s.provider != nil {
		if err := s.fetchReleases(ctx, al); err != nil {
			s.logger.Warn("music: failed to fetch album releases", "album", al.ID, "error", err)
			fetchErr = fmt.Errorf("fetch releases: %w", err)
		}
	}
	releases, err := s.repo.ListReleasesByAlbum(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	al.Releases = releases

	tracks, err := s.repo.ListTracksByAlbum(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if len(tracks) == 0 {
		al.TracksFetchedAt = nil
	}
	// A historical empty response may have stamped TracksFetchedAt without
	// storing any tracks. Retry it rather than permanently hiding the album's
	// tracks from details, matching and imports.
	if al.TracksFetchedAt == nil && al.SelectedReleaseID != "" && s.provider != nil {
		if err := s.fetchTracks(ctx, al); err != nil {
			s.logger.Warn("music: failed to fetch album tracks", "album", al.ID, "error", err)
			fetchErr = fmt.Errorf("fetch tracks: %w", err)
		}
		tracks, err = s.repo.ListTracksByAlbum(ctx, id)
		if err != nil {
			return nil, nil, err
		}
	}
	al.Tracks = tracks
	return al, fetchErr, nil
}

// fetchReleases pulls the album's editions from the provider, stores them, and
// selects a preferred release for acquisition/track matching.
func (s *service) fetchReleases(ctx context.Context, al *Album) error {
	oldSelectedID := al.SelectedReleaseID
	albumMeta, releaseMetas, err := s.provider.GetAlbum(ctx, al.MBID)
	if err != nil {
		return err
	}
	s.applyAlbumDetails(ctx, al, albumMeta)
	priorReleases, err := s.repo.ListReleasesByAlbum(ctx, al.ID)
	if err != nil {
		return err
	}
	priorSelectedMBID := ""
	for _, prior := range priorReleases {
		if prior.ID == al.SelectedReleaseID {
			priorSelectedMBID = prior.MBID
			break
		}
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

	// Select the preferred release; if none match criteria, pick the first one
	var selected *AlbumRelease
	if priorSelectedMBID != "" {
		for _, rel := range releases {
			if rel.MBID == priorSelectedMBID {
				selected = rel
				break
			}
		}
	}
	if selected == nil {
		var releaseStatuses []string
		profileID := al.MetadataProfileID
		if al.ArtistID != "" {
			artist, err := s.repo.GetArtist(ctx, al.ArtistID)
			if err != nil {
				return fmt.Errorf("get album artist: %w", err)
			}
			if artist != nil {
				profileID = artist.MetadataProfileID
			}
		}
		if profileID != "" {
			profile, err := s.repo.GetMetadataProfile(ctx, profileID)
			if err != nil {
				return fmt.Errorf("get metadata profile %q: %w", profileID, err)
			}
			if profile != nil {
				releaseStatuses = profile.ReleaseStatuses
			}
		}
		selected = pickRelease(releases, releaseStatuses)
	}
	if selected == nil && len(releases) > 0 {
		selected = releases[0]
		s.logger.Warn("music: no ideal release found, using first available", "album", al.ID, "count", len(releases))
	}
	if selected != nil {
		al.SelectedReleaseID = selected.ID
	}

	// A release refresh does not invalidate the selected edition's tracks unless
	// the selected edition actually changed. Track replacement is merge-safe.
	if selected == nil || selected.ID != oldSelectedID {
		al.TracksFetchedAt = nil
	}
	return s.repo.UpdateAlbum(ctx, al)
}

// applyAlbumDetails stores MusicBrainz links and rating plus best-effort
// artwork. The Cover Art Archive URL is built without checking that a cover
// exists, so a cover confirmed by the artwork source takes precedence.
func (s *service) applyAlbumDetails(ctx context.Context, al *Album, meta *metadata.AlbumMetadata) {
	now := time.Now().UTC()
	al.DetailsFetchedAt = &now
	if meta != nil {
		al.Rating = meta.Rating
		al.RatingVotes = meta.RatingVotes
		al.Links = externalLinksFrom(meta.Links)
		if al.CoverArtURL == "" {
			al.CoverArtURL = meta.CoverArtURL
		}
	}
	if s.artwork == nil {
		return
	}
	actx, cancel := context.WithTimeout(ctx, artworkTimeout)
	defer cancel()
	art, err := s.artwork.AlbumArtwork(actx, al.MBID)
	if err != nil {
		s.logger.Warn("music: album artwork lookup failed", "album", al.Title, "error", err)
		return
	}
	if art == nil {
		return
	}
	if art.Description != "" {
		al.Overview = art.Description
	}
	if art.CoverURL != "" {
		al.CoverArtURL = art.CoverURL
	}
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
		return fmt.Errorf("selected release %q is unavailable", al.SelectedReleaseID)
	}
	relMeta, err := s.provider.GetAlbumRelease(ctx, selectedMBID)
	if err != nil {
		return err
	}
	if relMeta == nil {
		return fmt.Errorf("no metadata for release %q", selectedMBID)
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

	if len(tracks) == 0 {
		// An empty provider response is not a successful fetch. In particular,
		// do not remove stored tracks or prevent a later retry.
		return fmt.Errorf("no tracks found for release %q", selectedMBID)
	}
	s.logger.Info("music: fetched tracks", "album", al.ID, "count", len(tracks))

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
	// Tracks inherit their monitored flag when fetched, so an album toggled
	// after its tracks were loaded must cascade or search would ignore them.
	if err := s.repo.SetAlbumTracksMonitored(ctx, id, monitored); err != nil {
		return nil, err
	}
	return s.GetAlbum(ctx, id)
}

func (s *service) SetTrackMonitored(ctx context.Context, albumID, trackID string, monitored bool) (*Track, error) {
	t, err := s.repo.GetTrack(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if t == nil || t.AlbumID != albumID {
		return nil, ErrNotFound
	}
	if monitored {
		al, err := s.repo.GetAlbum(ctx, albumID)
		if err != nil {
			return nil, err
		}
		if al == nil {
			return nil, ErrNotFound
		}
		if !al.Monitored {
			al.Monitored = true
			if err := s.repo.UpdateAlbum(ctx, al); err != nil {
				return nil, err
			}
		}
	}
	if err := s.repo.SetTrackMonitored(ctx, trackID, monitored); err != nil {
		return nil, err
	}
	t.Monitored = monitored
	return t, nil
}

func (s *service) SelectAlbumRelease(ctx context.Context, albumID, releaseID string) (*Album, error) {
	al, err := s.repo.GetAlbum(ctx, albumID)
	if err != nil {
		return nil, err
	}
	if al == nil {
		return nil, ErrNotFound
	}
	releases, err := s.repo.ListReleasesByAlbum(ctx, albumID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, release := range releases {
		if release.ID == releaseID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: release %q does not belong to album %q", ErrInvalid, releaseID, albumID)
	}
	if al.SelectedReleaseID != releaseID {
		al.SelectedReleaseID = releaseID
		al.TracksFetchedAt = nil
		if err := s.repo.UpdateAlbum(ctx, al); err != nil {
			return nil, err
		}
	}
	return s.GetAlbum(ctx, albumID)
}

// ListAlbumsByArtist returns the albums belonging to an artist (no lazy fetch).
func (s *service) ListAlbumsByArtist(ctx context.Context, artistID string) ([]*Album, error) {
	albums, err := s.repo.ListAlbumsByArtist(ctx, artistID)
	if err != nil {
		return nil, err
	}
	artist, err := s.repo.GetArtist(ctx, artistID)
	if err != nil {
		return nil, err
	}
	for _, album := range albums {
		setAlbumOwnerCredit(album, artist)
	}
	return albums, nil
}

// ImportTrackFile persists a track file and flags its track as present.
func (s *service) ImportTrackFile(ctx context.Context, tf *TrackFile) error {
	if tf == nil || tf.FilePath == "" {
		return ErrInvalid
	}
	existing, err := s.repo.GetTrackFileByPath(ctx, tf.FilePath)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // already imported
	}
	if err := s.repo.CreateTrackFile(ctx, tf); err != nil {
		return err
	}
	if tf.TrackID != "" {
		if err := s.repo.MarkTrackHasFile(ctx, tf.TrackID, true); err != nil {
			return fmt.Errorf("mark track %s as present: %w", tf.TrackID, err)
		}
	}
	return nil
}

func (s *service) ListTrackFilesByArtist(ctx context.Context, artistID string) ([]*TrackFile, error) {
	return s.repo.ListTrackFilesByArtist(ctx, artistID)
}

func (s *service) ListTrackFilesByAlbum(ctx context.Context, albumID string) ([]*TrackFile, error) {
	return s.repo.ListTrackFilesByAlbum(ctx, albumID)
}

func (s *service) ListArtistFiles(ctx context.Context, artistID string) ([]*ArtistFile, error) {
	if err := s.requireArtist(ctx, artistID); err != nil {
		return nil, err
	}
	return s.repo.ListArtistFiles(ctx, artistID)
}

func (s *service) ListArtistHistory(ctx context.Context, artistID string, limit int) ([]*ArtistHistoryEntry, error) {
	if err := s.requireArtist(ctx, artistID); err != nil {
		return nil, err
	}
	return s.repo.ListArtistHistory(ctx, artistID, limit)
}

func (s *service) requireArtist(ctx context.Context, id string) error {
	a, err := s.repo.GetArtist(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrNotFound
	}
	return nil
}

func (s *service) DeleteTrackFile(ctx context.Context, id string) error {
	return s.repo.DeleteTrackFile(ctx, id)
}

func (s *service) ListAudioQualityDefinitions(ctx context.Context) ([]*AudioQualityDefinition, error) {
	return s.repo.ListAudioQualityDefinitions(ctx)
}

func (s *service) ListAudioQualityProfiles(ctx context.Context) ([]*AudioQualityProfile, error) {
	return s.repo.ListAudioQualityProfiles(ctx)
}

func (s *service) CreateAudioQualityProfile(ctx context.Context, req CreateAudioQualityProfileRequest) (*AudioQualityProfile, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("%w: profile name is required", ErrInvalid)
	}
	definitions, err := s.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	if req.Cutoff != "" && !hasQualityDefinition(definitions, req.Cutoff) {
		return nil, fmt.Errorf("%w: unknown audio quality cutoff %q", ErrInvalid, req.Cutoff)
	}
	seen := make(map[string]bool)
	for _, item := range req.Items {
		if !hasQualityDefinition(definitions, item.DefinitionID) {
			return nil, fmt.Errorf("%w: unknown audio quality definition %q", ErrInvalid, item.DefinitionID)
		}
		if seen[item.DefinitionID] {
			return nil, fmt.Errorf("%w: duplicate audio quality definition %q", ErrInvalid, item.DefinitionID)
		}
		seen[item.DefinitionID] = true
	}
	if req.Items == nil {
		req.Items = []AudioProfileItem{}
	}
	items, err := json.Marshal(req.Items)
	if err != nil {
		return nil, fmt.Errorf("encode audio quality profile items: %w", err)
	}
	upgradeAllowed := true
	if req.UpgradeAllowed != nil {
		upgradeAllowed = *req.UpgradeAllowed
	}
	now := time.Now().UTC()
	profile := &AudioQualityProfile{
		ID: uuid.New().String(), Name: req.Name, Items: items, Cutoff: req.Cutoff,
		UpgradeAllowed: upgradeAllowed, FormatItems: req.FormatItems,
		MinFormatScore: req.MinFormatScore, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateAudioQualityProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func hasQualityDefinition(definitions []*AudioQualityDefinition, id string) bool {
	for _, definition := range definitions {
		if definition.ID == id {
			return true
		}
	}
	return false
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
	definitions, err := s.repo.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	if req.Cutoff != nil {
		if *req.Cutoff != "" && !hasQualityDefinition(definitions, *req.Cutoff) {
			return nil, fmt.Errorf("%w: unknown audio quality cutoff %q", ErrInvalid, *req.Cutoff)
		}
		p.Cutoff = *req.Cutoff
	}
	if req.UpgradeAllowed != nil {
		p.UpgradeAllowed = *req.UpgradeAllowed
	}
	if req.FormatItems != nil {
		p.FormatItems = req.FormatItems
	}
	if req.Items != nil {
		for _, item := range *req.Items {
			if !hasQualityDefinition(definitions, item.DefinitionID) {
				return nil, fmt.Errorf("%w: unknown audio quality definition %q", ErrInvalid, item.DefinitionID)
			}
		}
		items, err := json.Marshal(*req.Items)
		if err != nil {
			return nil, fmt.Errorf("encode audio quality profile items: %w", err)
		}
		p.Items = items
	}
	if req.MinFormatScore != nil {
		p.MinFormatScore = *req.MinFormatScore
	}
	if err := s.repo.UpdateAudioQualityProfile(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.GetAudioQualityProfile(ctx, id)
}

func (s *service) DeleteAudioQualityProfile(ctx context.Context, id string) error {
	profile, err := s.repo.GetAudioQualityProfile(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return ErrNotFound
	}
	artists, err := s.repo.ListArtists(ctx)
	if err != nil {
		return err
	}
	for _, artist := range artists {
		if artist.QualityProfileID == id {
			return fmt.Errorf("%w: audio quality profile is assigned to artist %q", ErrInvalid, artist.Name)
		}
	}
	albums, err := s.repo.ListAlbums(ctx)
	if err != nil {
		return err
	}
	for _, album := range albums {
		if album.ArtistID == "" && album.QualityProfileID == id {
			return fmt.Errorf("%w: audio quality profile is assigned to album %q", ErrInvalid, album.Title)
		}
	}
	if err := s.repo.SoftDeleteAudioQualityProfile(ctx, id); err != nil {
		return err
	}
	return nil
}

func (s *service) ListMetadataProfiles(ctx context.Context) ([]*MetadataProfile, error) {
	return s.repo.ListMetadataProfiles(ctx)
}

func (s *service) CreateMetadataProfile(ctx context.Context, req CreateMetadataProfileRequest) (*MetadataProfile, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("%w: profile name is required", ErrInvalid)
	}
	now := time.Now().UTC()
	profile := &MetadataProfile{
		ID: uuid.New().String(), Name: req.Name,
		PrimaryTypes:    cleanProfileTypes(req.PrimaryTypes),
		SecondaryTypes:  cleanProfileTypes(req.SecondaryTypes),
		ReleaseStatuses: cleanProfileTypes(req.ReleaseStatuses),
		CreatedAt:       now, UpdatedAt: now,
	}
	if err := s.repo.CreateMetadataProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *service) UpdateMetadataProfile(ctx context.Context, id string, req UpdateMetadataProfileRequest) (*MetadataProfile, error) {
	profile, err := s.repo.GetMetadataProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrNotFound
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: profile name is required", ErrInvalid)
		}
		profile.Name = name
	}
	if req.PrimaryTypes != nil {
		profile.PrimaryTypes = cleanProfileTypes(*req.PrimaryTypes)
	}
	if req.SecondaryTypes != nil {
		profile.SecondaryTypes = cleanProfileTypes(*req.SecondaryTypes)
	}
	if req.ReleaseStatuses != nil {
		profile.ReleaseStatuses = cleanProfileTypes(*req.ReleaseStatuses)
	}
	profile.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateMetadataProfile(ctx, profile); err != nil {
		return nil, err
	}
	return s.repo.GetMetadataProfile(ctx, id)
}

func (s *service) DeleteMetadataProfile(ctx context.Context, id string) error {
	profile, err := s.repo.GetMetadataProfile(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return ErrNotFound
	}
	artists, err := s.repo.ListArtists(ctx)
	if err != nil {
		return err
	}
	for _, artist := range artists {
		if artist.MetadataProfileID == id {
			return fmt.Errorf("%w: metadata profile is assigned to artist %q", ErrInvalid, artist.Name)
		}
	}
	albums, err := s.repo.ListAlbums(ctx)
	if err != nil {
		return err
	}
	for _, album := range albums {
		if album.ArtistID == "" && album.MetadataProfileID == id {
			return fmt.Errorf("%w: metadata profile is assigned to album %q", ErrInvalid, album.Title)
		}
	}
	return s.repo.SoftDeleteMetadataProfile(ctx, id)
}

func cleanProfileTypes(values []string) StringSlice {
	cleaned := make(StringSlice, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !containsFold(cleaned, value) {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

// syncArtistAlbums fetches the artist's release-groups from the metadata
// provider and inserts any that are not yet stored, returning the number added.
// New albums respect the metadata profile and overall monitoring status. It is
// shared by AddArtist and the new-release refresher.
func (s *service) syncArtistAlbums(ctx context.Context, artist *Artist, metadataProfileID string, monitoring MonitoringStatus, covers map[string]string) (int, error) {
	if s.provider == nil {
		return 0, fmt.Errorf("no music metadata provider configured")
	}
	albums, err := s.provider.GetArtistAlbums(ctx, artist.MBID)
	if err != nil {
		return 0, err
	}
	var metaProfile *MetadataProfile
	if metadataProfileID != "" {
		metaProfile, err = s.repo.GetMetadataProfile(ctx, metadataProfileID)
		if err != nil {
			return 0, fmt.Errorf("get metadata profile %q: %w", metadataProfileID, err)
		}
		if metaProfile == nil {
			return 0, fmt.Errorf("%w: metadata profile %q", ErrNotFound, metadataProfileID)
		}
	}
	monitorAlbums := monitoring == MonitoringMonitored
	added := 0
	catalogIDs := make([]string, 0, len(albums))
	for _, am := range albums {
		if am == nil || am.MBID == "" {
			continue
		}
		credits, err := s.albumCredits(ctx, am.Artists, am.ArtistMBID, am.ArtistName)
		if err != nil {
			return added, fmt.Errorf("resolve album artist credits %q: %w", am.MBID, err)
		}
		if len(credits) == 0 {
			credits = AlbumArtistCredits{{ArtistID: artist.ID, MBID: artist.MBID, Name: artist.Name}}
		}
		// The catalog is being fetched for this managed artist. Keep that
		// relationship even when the provider's credit lacks a linked MBID.
		linkedPrimary := false
		for i := range credits {
			if credits[i].ArtistID == artist.ID || credits[i].MBID == artist.MBID {
				credits[i].ArtistID = artist.ID
				linkedPrimary = true
			}
		}
		if !linkedPrimary {
			credits = append(credits, AlbumArtist{ArtistID: artist.ID, MBID: artist.MBID, Name: artist.Name})
		}
		existingAl, err := s.repo.GetAlbumByMBID(ctx, am.MBID)
		if err != nil {
			return added, fmt.Errorf("get album %q: %w", am.MBID, err)
		}
		if existingAl != nil {
			if cover := covers[am.MBID]; cover != "" {
				existingAl.CoverArtURL = cover
			}
			existingAl.Artists = credits
			if existingAl.LibraryID == "" {
				existingAl.LibraryID = artist.LibraryID
			}
			if existingAl.QualityProfileID == "" {
				existingAl.QualityProfileID = artist.QualityProfileID
			}
			if existingAl.MetadataProfileID == "" {
				existingAl.MetadataProfileID = metadataProfileID
			}
			if err := s.repo.UpdateAlbum(ctx, existingAl); err != nil {
				return added, fmt.Errorf("update album metadata %q: %w", existingAl.ID, err)
			}
			if err := s.repo.LinkAlbumArtist(ctx, existingAl.ID, artist.ID); err != nil {
				return added, fmt.Errorf("link existing album %q to artist %q: %w", existingAl.ID, artist.ID, err)
			}
			for _, credit := range credits {
				if credit.ArtistID != "" {
					if err := s.repo.LinkAlbumArtist(ctx, existingAl.ID, credit.ArtistID); err != nil {
						return added, fmt.Errorf("link credited artist %q to album %q: %w", credit.ArtistID, existingAl.ID, err)
					}
				}
			}
			catalogIDs = append(catalogIDs, existingAl.ID)
			continue
		}
		// Album types outside the metadata profile are still stored but left
		// unmonitored, mirroring Lidarr's behaviour.
		monitored := monitorAlbums && albumMatchesMetadataProfile(am.Type, am.SecondaryTypes, metaProfile)
		al := &Album{
			ID:                uuid.New().String(),
			MBID:              am.MBID,
			ArtistID:          artist.ID,
			LibraryID:         artist.LibraryID,
			QualityProfileID:  artist.QualityProfileID,
			MetadataProfileID: metadataProfileID,
			Title:             am.Title,
			AlbumType:         am.Type,
			SecondaryTypes:    am.SecondaryTypes,
			ReleaseDate:       am.ReleaseDate,
			Genres:            am.Genres,
			CoverArtURL:       firstNonEmpty(covers[am.MBID], am.CoverArtURL),
			Rating:            am.Rating,
			RatingVotes:       am.RatingVotes,
			Links:             externalLinksFrom(am.Links),
			Monitored:         monitored,
			Artists:           credits,
		}
		if err := s.repo.CreateAlbum(ctx, al); err != nil {
			return added, fmt.Errorf("create album %q: %w", am.MBID, err)
		}
		catalogIDs = append(catalogIDs, al.ID)
		added++
	}
	if err := s.repo.SetArtistAlbumCatalog(ctx, artist.ID, catalogIDs); err != nil {
		return added, fmt.Errorf("update artist album catalog: %w", err)
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
	covers := s.refreshArtistDetails(ctx, artist)
	return s.syncArtistAlbums(ctx, artist, artist.MetadataProfileID, artist.MonitoringStatus, covers)
}

// refreshArtistDetails re-fetches artist metadata and artwork. It is
// best-effort: failures are logged and the album refresh continues.
func (s *service) refreshArtistDetails(ctx context.Context, artist *Artist) map[string]string {
	if s.provider == nil || artist.MBID == "" {
		return nil
	}
	meta, err := s.provider.GetArtist(ctx, artist.MBID)
	if err != nil || meta == nil {
		s.logger.Warn("music: artist metadata refresh failed", "artist", artist.Name, "error", err)
		return nil
	}
	applyArtistMetadata(artist, meta)
	covers := s.enrichArtist(ctx, artist, meta.ImageURL)
	if err := s.repo.UpdateArtist(ctx, artist); err != nil {
		s.logger.Warn("music: failed to save refreshed artist details", "artist", artist.Name, "error", err)
	}
	return covers
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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

// pickRelease prefers configured release statuses, then the earliest known
// date, using track count only to break ties between otherwise equivalent editions.
func pickRelease(releases []*AlbumRelease, preferredStatuses []string) *AlbumRelease {
	if len(preferredStatuses) == 0 {
		preferredStatuses = []string{"official"}
	}
	var best *AlbumRelease
	for _, rel := range releases {
		if best == nil {
			best = rel
			continue
		}
		relStatusRank := releaseStatusRank(rel.Status, preferredStatuses)
		bestStatusRank := releaseStatusRank(best.Status, preferredStatuses)
		if relStatusRank < bestStatusRank ||
			(relStatusRank == bestStatusRank && releaseDateBefore(rel.ReleaseDate, best.ReleaseDate)) ||
			(relStatusRank == bestStatusRank && rel.ReleaseDate == best.ReleaseDate && rel.TrackCount > best.TrackCount) {
			best = rel
		}
	}
	return best
}

func releaseStatusRank(status string, preferred []string) int {
	for i, candidate := range preferred {
		if strings.EqualFold(strings.TrimSpace(status), strings.TrimSpace(candidate)) {
			return i
		}
	}
	return len(preferred)
}

func releaseDateBefore(a, b string) bool {
	if a == "" {
		return false
	}
	if b == "" {
		return true
	}
	return a < b
}
