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

	scanID := uuid.New().String()[:8]
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
	for _, a := range artists {
		artistsByName[normalizeTitle(a.Name)] = a
	}

	folders, err := discoverShowFolders(rootFolder)
	if err != nil {
		s.failMusicScan(scanID, fmt.Sprintf("discover artist folders: %v", err))
		return
	}
	s.logger.Info("discovered artist folders", "scanId", scanID, "count", len(folders))

	result := s.GetMusicScanStatus(scanID)
	for _, af := range folders {
		artist := artistsByName[normalizeTitle(af.Name)]
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
		MonitoringStatus: string(music.MonitoringMonitored),
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
		s.logger.Warn("music: list albums failed", "artist", artist.ID, "err", err)
		s.mu.Lock()
		result.Errors = append(result.Errors, fmt.Sprintf("list albums %q: %v", artist.Name, err))
		s.mu.Unlock()
		return
	}
	albumsByTitle := make(map[string]*music.Album, len(albums))
	for _, al := range albums {
		albumsByTitle[normalizeTitle(al.Title)] = al
	}

	// Cache album track lists (lazy-fetched via GetAlbum) keyed by album ID.
	trackCache := make(map[string][]*music.Track)

	files, walkErr := walkAudioFolder(folderPath)
	if walkErr != nil {
		s.logger.Warn("music: error walking artist folder", "path", folderPath, "err", walkErr)
	}

	s.mu.Lock()
	result.TotalFiles += len(files)
	s.mu.Unlock()

	for _, f := range files {
		tags, err := music.ReadAudioTags(f.Path)
		if err != nil {
			s.logger.Debug("music: failed to read tags", "path", f.Path, "err", err)
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
			full, gerr := s.musicSvc.GetAlbum(ctx, album.ID)
			if gerr != nil || full == nil {
				s.logger.Warn("music: get album failed", "album", album.ID, "err", gerr)
				tracks = nil
			} else {
				tracks = full.Tracks
			}
			trackCache[album.ID] = tracks
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
			s.logger.Warn("music: failed to import track file", "path", f.Path, "err", err)
			s.mu.Lock()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", f.Path, err))
			s.mu.Unlock()
			continue
		}

		s.mu.Lock()
		result.Imported++
		s.mu.Unlock()
	}
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

	tf := &music.TrackFile{
		ID:        uuid.New().String()[:8],
		TrackID:   track.ID,
		AlbumID:   album.ID,
		ArtistID:  artist.ID,
		FilePath:  path,
		Size:      size,
		Quality:   tags.Format,
		Format:    tags.Format,
		MediaInfo: mediaInfo,
		FileDate:  fileDate,
	}

	if err := s.musicSvc.ImportTrackFile(ctx, tf); err != nil {
		return fmt.Errorf("import track file: %w", err)
	}

	s.logger.Info("imported track file",
		"artist", artist.Name, "album", album.Title,
		"track", track.TrackNumber, "title", track.Title, "file", path,
	)
	return nil
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
		ID:          uuid.New().String()[:8],
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

	scanID := uuid.New().String()[:8]
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

func walkAudioFolder(root string) ([]audioScannedFile, error) {
	var files []audioScannedFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
