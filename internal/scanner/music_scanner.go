package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/kernel/telemetry"
	"github.com/ebenderooock/loom/internal/mediafiles"
	"github.com/ebenderooock/loom/internal/music"
)

// minAudioFileSize ignores tiny files (cover snippets, junk) under 64KB.
const minAudioFileSize = 64 * 1024

// MusicScanner orchestrates music library scanning: it walks artist folders,
// reads embedded audio tags, matches files to existing artists/albums/tracks in
// the library, and imports track files. Mirrors SeriesScanner but is
// folder/tag-driven rather than filename-parser driven.
type MusicScanner struct {
	musicSvc music.Service
	logger   *slog.Logger

	mu        sync.RWMutex
	scans     map[string]*ScanResult
	unmatched map[string][]*UnmatchedFile
}

// NewMusicScanner creates a new MusicScanner.
func NewMusicScanner(musicSvc music.Service, logger *slog.Logger) *MusicScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &MusicScanner{
		musicSvc:  musicSvc,
		logger:    logger,
		scans:     make(map[string]*ScanResult),
		unmatched: make(map[string][]*UnmatchedFile),
	}
}

// evictOldMusicScans removes the oldest completed scans beyond maxScanHistory.
// Must be called with s.mu held.
func (s *MusicScanner) evictOldMusicScans() {
	if len(s.scans) <= maxScanHistory {
		return
	}
	var oldest string
	var oldestTime time.Time
	for id, scan := range s.scans {
		if scan.Status != ScanStatusRunning && (oldest == "" || scan.StartedAt.Before(oldestTime)) {
			oldest = id
			oldestTime = scan.StartedAt
		}
	}
	if oldest != "" {
		delete(s.scans, oldest)
		delete(s.unmatched, oldest)
	}
}

// StartMusicScan begins an async scan of the given music library root folder.
func (s *MusicScanner) StartMusicScan(ctx context.Context, libraryID, rootFolder string) (string, error) {
	info, err := os.Stat(rootFolder)
	if err != nil {
		return "", fmt.Errorf("music scan: stat root folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("music scan: %s is not a directory", rootFolder)
	}

	scanID := uuid.New().String()
	result := &ScanResult{
		ID:             scanID,
		LibraryID:      libraryID,
		RootFolderPath: rootFolder,
		Status:         ScanStatusRunning,
		StartedAt:      time.Now(),
	}

	s.mu.Lock()
	s.scans[scanID] = result
	s.unmatched[scanID] = nil
	s.evictOldMusicScans()
	s.mu.Unlock()

	go s.runMusicScan(context.Background(), scanID, rootFolder) //nolint:gosec,contextcheck // background scan outlives the request context
	return scanID, nil
}

// GetMusicScanStatus returns the current state of a music scan job.
func (s *MusicScanner) GetMusicScanStatus(scanID string) *ScanResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scans[scanID]
}

// GetMusicUnmatched returns all unmatched files across all music scans.
func (s *MusicScanner) GetMusicUnmatched() []*UnmatchedFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []*UnmatchedFile
	for _, files := range s.unmatched {
		all = append(all, files...)
	}
	return all
}

func (s *MusicScanner) runMusicScan(ctx context.Context, scanID, rootFolder string) {
	scanStart := time.Now()
	s.logger.Info("starting music scan", "scanId", scanID, "path", rootFolder)

	artists, err := s.musicSvc.ListArtists(ctx)
	if err != nil {
		s.failMusicScan(scanID, fmt.Sprintf("list artists: %v", err))
		return
	}

	artistsByName := make(map[string]*music.Artist, len(artists))
	artistsByID := make(map[string]*music.Artist, len(artists))
	for _, a := range artists {
		artistsByName[normalizeTitle(a.Name)] = a
		artistsByID[a.ID] = a
	}
	result := s.GetMusicScanStatus(scanID)

	albums, err := s.musicSvc.ListAlbums(ctx)
	if err != nil {
		s.failMusicScan(scanID, fmt.Sprintf("list albums: %v", err))
		return
	}
	albumsByFolder := make(map[string][]*music.Album)
	for _, album := range albums {
		if album == nil {
			continue
		}
		owner := artistsByID[album.ArtistID]
		if album.ArtistID != "" && !music.IsVariousArtistAlbum(album) {
			continue
		}
		// Standalone albums must be assigned to this configured library. Older
		// artist-owned albums can inherit their artist's library assignment.
		inLibrary := album.LibraryID == result.LibraryID
		if album.ArtistID != "" && album.LibraryID == "" && owner != nil {
			inLibrary = owner.LibraryID == result.LibraryID
		}
		if !inLibrary {
			continue
		}
		folderName := albumOrganizationFolder(owner, album)
		albumsByFolder[normalizeTitle(folderName)] = append(albumsByFolder[normalizeTitle(folderName)], album)
	}

	folders, err := discoverShowFolders(rootFolder)
	if err != nil {
		s.failMusicScan(scanID, fmt.Sprintf("discover artist folders: %v", err))
		return
	}
	s.logger.Info("discovered artist folders", "scanId", scanID, "count", len(folders))

	for _, af := range folders {
		if ctx.Err() != nil {
			s.failMusicScan(scanID, ctx.Err().Error())
			return
		}
		artist := artistsByName[normalizeTitle(af.Name)]
		folderAlbums := albumsByFolder[normalizeTitle(af.Name)]
		if len(folderAlbums) > 0 {
			if normalizeTitle(af.Name) == normalizeTitle("Various Artists") {
				s.scanKnownAlbumsFolder(ctx, scanID, result, folderAlbums, af.Path, artistsByID)
				continue
			}
			standaloneAlbums := make([]*music.Album, 0, len(folderAlbums))
			for _, album := range folderAlbums {
				if album.ArtistID == "" {
					standaloneAlbums = appendAlbumOnce(standaloneAlbums, album)
				}
			}
			if artist != nil {
				if artist.Path != af.Path {
					path := af.Path
					updated, updateErr := s.musicSvc.UpdateArtist(ctx, artist.ID, music.UpdateArtistRequest{Path: &path})
					if updateErr != nil {
						s.logger.Warn("music: failed to update artist path", "artist", artist.ID, "error", updateErr)
					} else if updated != nil {
						artist = updated
						artistsByName[normalizeTitle(af.Name)] = artist
						artistsByID[artist.ID] = artist
					}
				}
				s.scanArtistFolder(ctx, scanID, result, artist, af.Path)
			} else {
				s.scanKnownAlbumsFolder(ctx, scanID, result, standaloneAlbums, af.Path, artistsByID)
			}
			if artist != nil && len(standaloneAlbums) > 0 {
				s.scanKnownAlbumsFolder(ctx, scanID, result, standaloneAlbums, af.Path, artistsByID)
			}
			continue
		}
		// Various Artists is an organization folder, never a managed artist.
		if normalizeTitle(af.Name) == normalizeTitle("Various Artists") {
			continue
		}
		if artist == nil {
			added, addErr := s.ensureArtistForFolder(ctx, result.LibraryID, af.Name)
			if addErr != nil {
				s.logger.Warn("music: failed to auto-add artist from folder", "folder", af.Name, "error", addErr)
				continue
			}
			if added == nil {
				s.logger.Debug("no library artist for folder, skipping", "folder", af.Name)
				continue
			}
			artistsByName[normalizeTitle(af.Name)] = added
			artist = added
		}
		if artist.Path != af.Path {
			path := af.Path
			updated, updateErr := s.musicSvc.UpdateArtist(ctx, artist.ID, music.UpdateArtistRequest{Path: &path})
			if updateErr != nil {
				s.logger.Warn("music: failed to update artist path", "artist", artist.ID, "error", updateErr)
			} else if updated != nil {
				artist = updated
				artistsByName[normalizeTitle(af.Name)] = artist
			}
		}
		s.scanArtistFolder(ctx, scanID, result, artist, af.Path)
	}

	now := time.Now()
	s.mu.Lock()
	result.Status = ScanStatusCompleted
	result.CompletedAt = &now
	total, matched, unmatched, imported := result.TotalFiles, result.Matched, result.Unmatched, result.Imported
	s.mu.Unlock()

	s.logger.Info("music scan completed",
		"scanId", scanID, "total", total, "matched", matched,
		"unmatched", unmatched, "imported", imported,
	)

	if m := telemetry.App(); m != nil {
		m.ScanTotal.WithLabelValues("music", "success").Inc()
		m.ScanDuration.WithLabelValues("music").Observe(time.Since(scanStart).Seconds())
		m.ScanFilesProcessed.WithLabelValues("music", "matched").Add(float64(matched))
		m.ScanFilesProcessed.WithLabelValues("music", "unmatched").Add(float64(unmatched))
		m.ScanFilesProcessed.WithLabelValues("music", "error").Add(float64(len(result.Errors)))
	}

}

func albumOrganizationFolder(owner *music.Artist, album *music.Album) string {
	trackPath := music.BuildTrackPath(owner, album, nil, false, "")
	return filepath.Base(filepath.Dir(filepath.Dir(trackPath)))
}

func appendAlbumOnce(albums []*music.Album, album *music.Album) []*music.Album {
	if album == nil {
		return albums
	}
	for _, existing := range albums {
		if existing != nil && existing.ID == album.ID {
			return albums
		}
	}
	return append(albums, album)
}

func (s *MusicScanner) ensureArtistForFolder(ctx context.Context, libraryID, folderName string) (*music.Artist, error) {
	name := strings.TrimSpace(folderName)
	if name == "" || libraryID == "" {
		return nil, nil
	}

	candidates, err := s.musicSvc.LookupArtists(ctx, name, 10)
	if err != nil {
		return nil, fmt.Errorf("lookup artist %q: %w", name, err)
	}
	match := pickExactArtistLookup(name, candidates)
	if match == nil {
		return nil, nil
	}

	qualityProfiles, err := s.musicSvc.ListAudioQualityProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list audio quality profiles: %w", err)
	}
	if len(qualityProfiles) == 0 {
		return nil, fmt.Errorf("no audio quality profiles configured")
	}

	req := music.AddArtistRequest{
		MBID:             match.MBID,
		LibraryID:        libraryID,
		QualityProfileID: qualityProfiles[0].ID,
		MonitoringStatus: string(music.MonitoringUnmonitored),
		Search:           false,
	}
	metadataProfiles, err := s.musicSvc.ListMetadataProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list metadata profiles: %w", err)
	}
	if len(metadataProfiles) > 0 {
		req.MetadataProfileID = metadataProfiles[0].ID
	}

	artist, err := s.musicSvc.AddArtist(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("add artist %q: %w", name, err)
	}
	s.logger.Info("music: auto-added artist from library folder", "folder", name, "artist", artist.Name, "artist_id", artist.ID)
	return artist, nil
}

func pickExactArtistLookup(name string, candidates []*music.ArtistLookupResult) *music.ArtistLookupResult {
	target := normalizeTitle(name)
	for _, c := range candidates {
		if c == nil {
			continue
		}
		if normalizeTitle(c.Name) == target {
			return c
		}
	}
	return nil
}

// scanArtistFolder walks an artist's folder, reads tags and imports matched
// track files. Albums are matched by embedded album tag (falling back to the
// containing folder name); tracks are matched by disc+track number, then title.
func (s *MusicScanner) scanArtistFolder(ctx context.Context, scanID string, result *ScanResult, artist *music.Artist, folderPath string) {
	albums, err := s.musicSvc.ListAlbumsByArtist(ctx, artist.ID)
	if err != nil {
		s.logger.Warn("music: list albums failed", "artist", artist.ID, "error", err)
		s.mu.Lock()
		result.Errors = append(result.Errors, fmt.Sprintf("list albums %q: %v", artist.Name, err))
		s.mu.Unlock()
		return
	}
	albumsByTitle := make(map[string]*music.Album, len(albums))
	for _, al := range albums {
		if al.ArtistID == artist.ID && !music.IsVariousArtistAlbum(al) {
			albumsByTitle[normalizeTitle(al.Title)] = al
		}
	}

	// Cache album track lists (lazy-fetched via GetAlbum) keyed by album ID.
	trackCache := make(map[string][]*music.Track)

	files, walkErr := walkAudioFolder(ctx, folderPath)
	if walkErr != nil {
		s.logger.Warn("music: error walking artist folder", "path", folderPath, "error", walkErr)
		if ctx.Err() != nil {
			return
		}
	}

	s.mu.Lock()
	result.TotalFiles += len(files)
	s.mu.Unlock()

	for _, f := range files {
		if ctx.Err() != nil {
			return
		}
		tags, err := music.ReadAudioTags(f.Path)
		if err != nil {
			s.logger.Debug("music: failed to read tags", "path", f.Path, "error", err)
			s.addMusicUnmatched(scanID, result, f.Path, f.Size, artist.Name, "", "")
			continue
		}

		albumName := strings.TrimSpace(tags.Album)
		if albumName == "" {
			albumName = filepath.Base(filepath.Dir(f.Path))
		}
		album := albumsByTitle[normalizeTitle(albumName)]
		if album == nil {
			s.addMusicUnmatched(scanID, result, f.Path, f.Size, artist.Name, albumName, tags.Title)
			continue
		}

		tracks, ok := trackCache[album.ID]
		if !ok {
			full, gerr := s.musicSvc.LoadAlbumTracks(ctx, album.ID)
			if gerr != nil || full == nil {
				s.logger.Warn("music: load album tracks failed", "album", album.ID, "error", gerr)
				tracks = nil
			} else {
				tracks = full.Tracks
			}
			trackCache[album.ID] = tracks
		}
		if len(tracks) == 0 {
			// Metadata is unavailable; leave the file untouched so a later
			// scan can match it instead of reporting it as unmatched.
			s.mu.Lock()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: track list for album %q is unavailable", f.Path, album.Title))
			s.mu.Unlock()
			continue
		}

		track := matchTrack(tracks, tags)
		if track == nil {
			s.addMusicUnmatched(scanID, result, f.Path, f.Size, artist.Name, albumName, tags.Title)
			continue
		}

		s.mu.Lock()
		result.Matched++
		s.mu.Unlock()

		if err := s.importTrackFile(ctx, artist, album, track, f.Path, f.Size, tags); err != nil {
			s.logger.Warn("music: failed to import track file", "path", f.Path, "error", err)
			s.mu.Lock()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", f.Path, err))
			s.mu.Unlock()
			continue
		}

		s.mu.Lock()
		result.Imported++
		s.mu.Unlock()
	}
	if walkErr == nil {
		if removed, err := s.musicSvc.PruneMissingTrackFiles(ctx, artist.ID, folderPath); err != nil {
			s.logger.Warn("music: failed to prune missing track files", "artist", artist.ID, "error", err)
		} else if removed > 0 {
			s.logger.Info("music: pruned missing track files", "artist", artist.ID, "count", removed)
		}
	}
}

// scanKnownAlbumsFolder handles folders such as a standalone credited artist
// directory or the shared Various Artists directory without creating managed
// artists for the folder name.
func (s *MusicScanner) scanKnownAlbumsFolder(ctx context.Context, scanID string, result *ScanResult, albums []*music.Album, folderPath string, artistsByID map[string]*music.Artist) {
	trackCache := make(map[string][]*music.Track)
	files, walkErr := walkAudioFolder(ctx, folderPath)
	if walkErr != nil {
		s.logger.Warn("music: error walking known album folder", "path", folderPath, "error", walkErr)
		if ctx.Err() != nil {
			return
		}
	}

	s.mu.Lock()
	result.TotalFiles += len(files)
	s.mu.Unlock()

	for _, file := range files {
		if ctx.Err() != nil {
			return
		}
		tags, err := music.ReadAudioTags(file.Path)
		if err != nil {
			s.addMusicUnmatched(scanID, result, file.Path, file.Size, filepath.Base(folderPath), "", "")
			continue
		}
		albumName := strings.TrimSpace(tags.Album)
		candidates := matchingAlbumCandidates(albums, albumName, filepath.Base(filepath.Dir(file.Path)))
		if len(candidates) == 0 {
			if albumName == "" {
				albumName = filepath.Base(filepath.Dir(file.Path))
			}
			s.addMusicUnmatched(scanID, result, file.Path, file.Size, filepath.Base(folderPath), albumName, tags.Title)
			continue
		}

		type candidateMatch struct {
			album *music.Album
			track *music.Track
		}
		var matches []candidateMatch
		metadataUnavailable := false
		for _, candidate := range candidates {
			tracks, ok := trackCache[candidate.ID]
			if !ok {
				full, loadErr := s.musicSvc.LoadAlbumTracks(ctx, candidate.ID)
				if loadErr != nil || full == nil {
					s.logger.Warn("music: load album tracks failed", "album", candidate.ID, "error", loadErr)
					s.mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("%s: track list for album %q is unavailable", file.Path, candidate.Title))
					s.mu.Unlock()
					metadataUnavailable = true
					continue
				}
				tracks = full.Tracks
				trackCache[candidate.ID] = tracks
			}
			if track := matchTrackForAlbum(candidate, tracks, tags); track != nil {
				matches = append(matches, candidateMatch{album: candidate, track: track})
			}
		}
		if metadataUnavailable {
			// A candidate with unavailable metadata could be a competing
			// same-title album, so do not import a partial-confidence match.
			continue
		}
		if len(matches) != 1 {
			if len(matches) > 1 {
				s.mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("%s: ambiguous album %q matched %d known albums", file.Path, albumName, len(matches)))
				s.mu.Unlock()
			}
			if albumName == "" {
				albumName = filepath.Base(filepath.Dir(file.Path))
			}
			s.addMusicUnmatched(scanID, result, file.Path, file.Size, filepath.Base(folderPath), albumName, tags.Title)
			continue
		}
		album, track := matches[0].album, matches[0].track
		s.mu.Lock()
		result.Matched++
		s.mu.Unlock()

		if err := s.importTrackFile(ctx, artistsByID[album.ArtistID], album, track, file.Path, file.Size, tags); err != nil {
			s.logger.Warn("music: failed to import track file", "path", file.Path, "error", err)
			s.mu.Lock()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Path, err))
			s.mu.Unlock()
			continue
		}
		s.mu.Lock()
		result.Imported++
		s.mu.Unlock()
	}

	if walkErr == nil {
		pruned := 0
		for _, album := range albums {
			if album == nil {
				continue
			}
			removed, err := s.musicSvc.PruneMissingAlbumTrackFiles(ctx, album.ID, folderPath)
			if err != nil {
				s.logger.Warn("music: failed to prune missing track files", "album", album.ID, "error", err)
				continue
			}
			pruned += removed
		}
		if pruned > 0 {
			s.logger.Info("music: pruned missing track files", "path", folderPath, "count", pruned)
		}
	}
}

func matchingAlbumCandidates(albums []*music.Album, albumTag, albumFolder string) []*music.Album {
	var candidates []*music.Album
	for _, album := range albums {
		if album == nil {
			continue
		}
		matches := false
		if strings.TrimSpace(albumTag) != "" {
			matches = normalizeTitle(album.Title) == normalizeTitle(albumTag)
		} else {
			expectedFolder := filepath.Base(filepath.Dir(music.BuildTrackPath(nil, album, nil, false, "")))
			matches = normalizeTitle(expectedFolder) == normalizeTitle(albumFolder)
		}
		if matches {
			candidates = appendAlbumOnce(candidates, album)
		}
	}
	return candidates
}

func albumCreditMatches(album *music.Album, artistName string) bool {
	name := normalizeTitle(artistName)
	if name == "" || name == normalizeTitle("Various Artists") {
		return false
	}
	for _, credit := range album.Artists {
		if normalizeTitle(credit.Name) == name {
			return true
		}
	}
	return false
}

func matchTrackForAlbum(album *music.Album, tracks []*music.Track, tags *music.AudioTags) *music.Track {
	if album == nil || tags == nil || normalizeTitle(tags.Title) == "" {
		return nil
	}
	various := music.IsVariousArtistAlbum(album)
	albumIdentity := albumCreditMatches(album, tags.AlbumArtist)
	if !albumIdentity && !various && strings.TrimSpace(tags.AlbumArtist) == "" {
		albumIdentity = albumCreditMatches(album, tags.Artist)
	}
	requireTrackArtist := various || !albumIdentity
	tagArtist := normalizeTitle(tags.Artist)
	title := normalizeTitle(tags.Title)

	var eligible []*music.Track
	var exact []*music.Track
	for _, track := range tracks {
		if track == nil {
			continue
		}
		trackArtist := normalizeTitle(track.ArtistName)
		if tagArtist != "" && trackArtist != "" && tagArtist != trackArtist {
			continue
		}
		if requireTrackArtist && (tagArtist == "" || trackArtist == "" || tagArtist != trackArtist) {
			continue
		}
		eligible = append(eligible, track)
		if normalizeTitle(track.Title) == title {
			exact = append(exact, track)
		}
	}
	if len(exact) == 1 {
		return exact[0]
	}
	if len(exact) > 1 {
		return matchTrackPosition(exact, tags)
	}
	return music.MatchTrackByBaseTitle(eligible, tags.Title, normalizeTitle)
}

func matchTrackPosition(tracks []*music.Track, tags *music.AudioTags) *music.Track {
	if tags.TrackNumber <= 0 {
		return nil
	}
	disc := tags.DiscNumber
	if disc < 1 {
		disc = 1
	}
	var match *music.Track
	for _, track := range tracks {
		trackDisc := track.DiscNumber
		if trackDisc < 1 {
			trackDisc = 1
		}
		if track.TrackNumber != tags.TrackNumber || trackDisc != disc {
			continue
		}
		if match != nil {
			return nil
		}
		match = track
	}
	return match
}

func (s *MusicScanner) importTrackFile(ctx context.Context, artist *music.Artist, album *music.Album, track *music.Track, path string, size int64, tags *music.AudioTags) error {
	var fileDate *time.Time
	if info, err := os.Stat(path); err == nil {
		mt := info.ModTime()
		fileDate = &mt
	}

	mediaInfo := music.MediaInfoMap{}
	if tags.Genre != "" {
		mediaInfo["genre"] = tags.Genre
	}
	if tags.Year > 0 {
		mediaInfo["year"] = tags.Year
	}
	bitrate := music.EstimateAudioBitrate(size, track.DurationMs)
	if bitrate > 0 {
		mediaInfo["bitrate"] = bitrate
	}
	if tags.Bits > 0 {
		mediaInfo["bits"] = tags.Bits
	}

	artistID := album.ArtistID
	if artistID == "" && artist != nil {
		artistID = artist.ID
	}
	artistName := albumArtistLogName(album)
	if artist != nil && (album.ArtistID == "" || artist.ID == album.ArtistID) {
		artistName = artist.Name
	}
	tf := &music.TrackFile{
		ID:        uuid.New().String(),
		TrackID:   track.ID,
		AlbumID:   album.ID,
		ArtistID:  artistID,
		FilePath:  path,
		Size:      size,
		Quality:   tags.Format,
		Format:    tags.Format,
		Bitrate:   bitrate,
		MediaInfo: mediaInfo,
		FileDate:  fileDate,
	}

	if err := s.musicSvc.ImportTrackFile(ctx, tf); err != nil {
		return fmt.Errorf("import track file: %w", err)
	}

	s.logger.Info("imported track file",
		"artist", artistName, "album", album.Title,
		"track", track.TrackNumber, "title", track.Title, "file", path,
	)
	return nil
}

func albumArtistLogName(album *music.Album) string {
	if music.IsVariousArtistAlbum(album) {
		return "Various Artists"
	}
	for _, credit := range album.Artists {
		if strings.TrimSpace(credit.Name) != "" {
			return credit.Name
		}
	}
	return "Unknown Artist"
}

// matchTrack finds the best track for a tagged file: disc+track number first,
// then a normalized title match.
func matchTrack(tracks []*music.Track, tags *music.AudioTags) *music.Track {
	if len(tracks) == 0 {
		return nil
	}
	disc := tags.DiscNumber
	if disc == 0 {
		disc = 1
	}
	if tags.TrackNumber > 0 {
		for _, t := range tracks {
			td := t.DiscNumber
			if td == 0 {
				td = 1
			}
			if t.TrackNumber == tags.TrackNumber && td == disc {
				return t
			}
		}
	}
	if title := normalizeTitle(tags.Title); title != "" {
		for _, t := range tracks {
			if normalizeTitle(t.Title) == title {
				return t
			}
		}
		return music.MatchTrackByBaseTitle(tracks, tags.Title, normalizeTitle)
	}
	return nil
}

func (s *MusicScanner) addMusicUnmatched(scanID string, result *ScanResult, path string, size int64, artist, album, title string) {
	parsed := strings.TrimSpace(title)
	if parsed == "" {
		parsed = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	label := parsed
	if album != "" {
		label = fmt.Sprintf("%s — %s", album, parsed)
	}
	uf := &UnmatchedFile{
		ID:          uuid.New().String(),
		ScanID:      scanID,
		FilePath:    path,
		Size:        size,
		ParsedTitle: label,
		Source:      artist,
	}
	s.mu.Lock()
	result.Unmatched++
	s.unmatched[scanID] = append(s.unmatched[scanID], uf)
	s.mu.Unlock()
}

func (s *MusicScanner) failMusicScan(scanID, errMsg string) {
	now := time.Now()
	s.mu.Lock()
	var startedAt time.Time
	if result, ok := s.scans[scanID]; ok {
		result.Status = ScanStatusFailed
		result.Errors = append(result.Errors, errMsg)
		result.CompletedAt = &now
		startedAt = result.StartedAt
	}
	s.mu.Unlock()
	s.logger.Error("music scan failed", "scanId", scanID, "error", errMsg)
	if m := telemetry.App(); m != nil {
		m.ScanTotal.WithLabelValues("music", "failed").Inc()
		if !startedAt.IsZero() {
			m.ScanDuration.WithLabelValues("music").Observe(time.Since(startedAt).Seconds())
		}
	}
}

// RescanArtist rescans a single artist's folder for new/changed track files.
func (s *MusicScanner) RescanArtist(ctx context.Context, artistID, libraryPath string) (*ScanResult, error) {
	artist, err := s.musicSvc.GetArtist(ctx, artistID)
	if err != nil {
		return nil, fmt.Errorf("get artist: %w", err)
	}
	if artist == nil {
		return nil, fmt.Errorf("artist %s not found", artistID)
	}

	folders, err := discoverShowFolders(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("discover artist folders: %w", err)
	}
	var artistPath string
	normTarget := normalizeTitle(artist.Name)
	for _, af := range folders {
		if normalizeTitle(af.Name) == normTarget {
			artistPath = af.Path
			break
		}
	}
	if artistPath == "" {
		return nil, fmt.Errorf("no folder found for artist %q in %s", artist.Name, libraryPath)
	}

	scanID := uuid.New().String()
	result := &ScanResult{
		ID:             scanID,
		LibraryID:      artist.LibraryID,
		RootFolderPath: artistPath,
		Status:         ScanStatusRunning,
		StartedAt:      time.Now(),
	}
	s.mu.Lock()
	s.scans[scanID] = result
	s.unmatched[scanID] = nil
	s.evictOldMusicScans()
	s.mu.Unlock()

	s.scanArtistFolder(ctx, scanID, result, artist, artistPath)

	now := time.Now()
	s.mu.Lock()
	result.Status = ScanStatusCompleted
	result.CompletedAt = &now
	s.mu.Unlock()

	s.logger.Info("artist rescan completed", "artist", artist.Name, "matched", result.Matched, "imported", result.Imported)
	return result, nil
}

type audioScannedFile struct {
	Path string
	Size int64
}

func walkAudioFolder(ctx context.Context, root string) ([]audioScannedFile, error) {
	var files []audioScannedFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries and keep walking
		}
		if info.IsDir() {
			if shouldIgnore(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !mediafiles.IsAudio(ext) {
			return nil
		}
		if shouldIgnore(strings.ToLower(info.Name())) {
			return nil
		}
		if info.Size() < minAudioFileSize {
			return nil
		}
		files = append(files, audioScannedFile{Path: path, Size: info.Size()})
		return nil
	})
	return files, err
}
