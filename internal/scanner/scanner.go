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
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
)

// videoExtensions lists recognized video file extensions.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true,
	".wmv": true, ".flv": true, ".mov": true,
}

// ignoredNames lists folder/file name fragments that should be skipped.
var ignoredNames = []string{
	"sample", "trailer", "extras", "featurette", "behind the scenes",
	"deleted scenes", "bonus", "interview", "subs", "subtitles",
	"@eadir", ".ds_store", "thumbs.db",
}

// ScanStatus represents the state of a scan job.
type ScanStatus string

const (
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
)

// ScanResult holds the outcome of a library scan.
type ScanResult struct {
	ID             string     `json:"id"`
	LibraryID      string     `json:"libraryId"`
	RootFolderPath string     `json:"rootFolderPath"`
	Status         ScanStatus `json:"status"`
	TotalFiles     int        `json:"totalFiles"`
	Matched        int        `json:"matched"`
	Unmatched      int        `json:"unmatched"`
	Imported       int        `json:"imported"`
	Errors         []string   `json:"errors,omitempty"`
	StartedAt      time.Time  `json:"startedAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
}

// UnmatchedFile represents a scanned file that couldn't be auto-matched.
type UnmatchedFile struct {
	ID         string `json:"id"`
	ScanID     string `json:"scanId"`
	FilePath   string `json:"filePath"`
	Size       int64  `json:"size"`
	ParsedTitle string `json:"parsedTitle"`
	ParsedYear  int    `json:"parsedYear"`
	Quality    string `json:"quality"`
	Source     string `json:"source"`
}

// MetadataSearcher abstracts TMDB lookups for the scanner.
type MetadataSearcher interface {
	FindMovieByQuery(ctx context.Context, query string, year int) ([]*metadata.MovieMetadata, error)
}

// Scanner orchestrates library scanning and import.
type Scanner struct {
	movieSvc movies.Service
	metadata MetadataSearcher
	logger   *slog.Logger

	mu    sync.RWMutex
	scans map[string]*ScanResult
	unmatched map[string][]*UnmatchedFile // scanID -> files
}

// New creates a new Scanner.
func New(movieSvc movies.Service, meta MetadataSearcher, logger *slog.Logger) *Scanner {
	return &Scanner{
		movieSvc:  movieSvc,
		metadata:  meta,
		logger:    logger,
		scans:     make(map[string]*ScanResult),
		unmatched: make(map[string][]*UnmatchedFile),
	}
}

// StartScan begins an async scan of the given root folder.
func (s *Scanner) StartScan(ctx context.Context, libraryID, libraryPath string) string {
	scanID := uuid.New().String()[:8]
	result := &ScanResult{
		ID:             scanID,
		LibraryID:      libraryID,
		RootFolderPath: libraryPath,
		Status:         ScanStatusRunning,
		StartedAt:      time.Now(),
	}

	s.mu.Lock()
	s.scans[scanID] = result
	s.unmatched[scanID] = nil
	s.mu.Unlock()

	go s.runScan(context.Background(), scanID, libraryPath)

	return scanID
}

// GetScan returns the current state of a scan job.
func (s *Scanner) GetScan(scanID string) *ScanResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scans[scanID]
}

// GetUnmatched returns unmatched files for a scan.
func (s *Scanner) GetUnmatched(scanID string) []*UnmatchedFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unmatched[scanID]
}

// GetAllUnmatched returns all unmatched files across all scans.
func (s *Scanner) GetAllUnmatched() []*UnmatchedFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []*UnmatchedFile
	for _, files := range s.unmatched {
		all = append(all, files...)
	}
	return all
}

// MatchFile manually matches an unmatched file to a TMDB movie.
func (s *Scanner) MatchFile(ctx context.Context, unmatchedID string, tmdbID string, libraryID string, qualityProfileID string) error {
	s.mu.Lock()
	var target *UnmatchedFile
	var scanID string
	for sid, files := range s.unmatched {
		for _, f := range files {
			if f.ID == unmatchedID {
				target = f
				scanID = sid
				break
			}
		}
		if target != nil {
			break
		}
	}
	s.mu.Unlock()

	if target == nil {
		return fmt.Errorf("unmatched file not found: %s", unmatchedID)
	}

	// Search TMDB by ID
	results, err := s.metadata.FindMovieByQuery(ctx, tmdbID, 0)
	if err != nil || len(results) == 0 {
		return fmt.Errorf("TMDB lookup failed for %q", tmdbID)
	}

	meta := results[0]
	if err := s.importFile(ctx, target.FilePath, target.Size, target.Quality, target.Source, meta, libraryID, qualityProfileID); err != nil {
		return err
	}

	// Remove from unmatched
	s.mu.Lock()
	files := s.unmatched[scanID]
	for i, f := range files {
		if f.ID == unmatchedID {
			s.unmatched[scanID] = append(files[:i], files[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	return nil
}

func (s *Scanner) failScan(scanID string, errMsg string) {
	now := time.Now()
	s.mu.Lock()
	if result, ok := s.scans[scanID]; ok {
		result.Status = ScanStatusFailed
		result.Errors = append(result.Errors, errMsg)
		result.CompletedAt = &now
	}
	s.mu.Unlock()
	s.logger.Error("scan failed", "scanId", scanID, "error", errMsg)
}

func (s *Scanner) runScan(ctx context.Context, scanID string, libraryPath string) {
	s.logger.Info("starting library scan", "scanId", scanID, "path", libraryPath)

	// Walk the root folder and find video files
	scanned, err := walkFolder(libraryPath)
	if err != nil {
		s.failScan(scanID, fmt.Sprintf("walk error: %v", err))
		return
	}

	result := s.GetScan(scanID)
	s.mu.Lock()
	result.TotalFiles = len(scanned)
	s.mu.Unlock()

	s.logger.Info("found video files", "scanId", scanID, "count", len(scanned))

	for _, sf := range scanned {
		if err := s.processFile(ctx, scanID, sf, result.LibraryID); err != nil {
			s.logger.Warn("failed to process file", "path", sf.Path, "err", err)
			s.mu.Lock()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", sf.Path, err))
			s.mu.Unlock()
		}
	}

	now := time.Now()
	s.mu.Lock()
	result.Status = ScanStatusCompleted
	result.CompletedAt = &now
	s.mu.Unlock()

	s.logger.Info("scan completed",
		"scanId", scanID,
		"total", result.TotalFiles,
		"matched", result.Matched,
		"unmatched", result.Unmatched,
		"imported", result.Imported,
	)
}

type scannedFile struct {
	Path string
	Size int64
	Rel  *parser.Release
}

func walkFolder(root string) ([]scannedFile, error) {
	var files []scannedFile

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if info.IsDir() {
			if shouldIgnore(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !videoExtensions[ext] {
			return nil
		}
		if shouldIgnore(strings.ToLower(info.Name())) {
			return nil
		}
		// Skip very small files (likely samples)
		if info.Size() < 50*1024*1024 { // < 50MB
			return nil
		}

		// Parse from the parent folder name first (more reliable), fallback to filename
		rel := filepath.Dir(path)
		folderName := filepath.Base(rel)
		fileName := strings.TrimSuffix(info.Name(), ext)

		parsed := parser.Parse(folderName)
		if parsed.Title == "" || parsed.Year == 0 {
			// Folder didn't parse well, try the filename
			parsed = parser.Parse(fileName)
		}

		files = append(files, scannedFile{
			Path: path,
			Size: info.Size(),
			Rel:  parsed,
		})
		return nil
	})

	return files, err
}

func shouldIgnore(name string) bool {
	lower := strings.ToLower(name)
	for _, ignored := range ignoredNames {
		if strings.Contains(lower, ignored) {
			return true
		}
	}
	return false
}

func (s *Scanner) processFile(ctx context.Context, scanID string, sf scannedFile, libraryID string) error {
	result := s.GetScan(scanID)

	// Check if this file is already in the DB
	existingFiles, err := s.findExistingFileByPath(ctx, sf.Path)
	if err == nil && existingFiles {
		s.logger.Debug("file already imported, skipping", "path", sf.Path)
		return nil
	}

	title := sf.Rel.Title
	year := sf.Rel.Year
	quality := qualityFromResolution(sf.Rel.Resolution)

	if title == "" {
		s.addUnmatched(scanID, result, sf, "")
		return nil
	}

	// Search TMDB
	results, err := s.metadata.FindMovieByQuery(ctx, title, year)
	if err != nil {
		s.addUnmatched(scanID, result, sf, "")
		return fmt.Errorf("TMDB search failed: %w", err)
	}

	// Try to auto-match: exact normalized title + year
	matched := autoMatch(title, year, results)
	if matched == nil {
		s.addUnmatched(scanID, result, sf, title)
		return nil
	}

	s.mu.Lock()
	result.Matched++
	s.mu.Unlock()

	if err := s.importFile(ctx, sf.Path, sf.Size, quality, sf.Rel.Source, matched, libraryID, ""); err != nil {
		return err
	}

	s.mu.Lock()
	result.Imported++
	s.mu.Unlock()

	return nil
}

func (s *Scanner) addUnmatched(scanID string, result *ScanResult, sf scannedFile, parsedTitle string) {
	uf := &UnmatchedFile{
		ID:          uuid.New().String()[:8],
		ScanID:      scanID,
		FilePath:    sf.Path,
		Size:        sf.Size,
		ParsedTitle: parsedTitle,
		ParsedYear:  sf.Rel.Year,
		Quality:     qualityFromResolution(sf.Rel.Resolution),
		Source:      sf.Rel.Source,
	}
	s.mu.Lock()
	result.Unmatched++
	s.unmatched[scanID] = append(s.unmatched[scanID], uf)
	s.mu.Unlock()
}

func (s *Scanner) findExistingFileByPath(ctx context.Context, path string) (bool, error) {
	// Check all movies for this file path — use the repository's GetMovieFileByPath
	// Since we don't have direct repo access, we'll check via service if available
	// For now, this is a best-effort check
	return false, fmt.Errorf("not implemented")
}

func (s *Scanner) importFile(ctx context.Context, filePath string, size int64, quality, source string, meta *metadata.MovieMetadata, libraryID, qualityProfileID string) error {
	tmdbID := ""
	if meta.TMDBID != nil {
		tmdbID = *meta.TMDBID
	}

	// Check if movie already exists by TMDB ID
	existingMovies, err := s.movieSvc.ListMovies(ctx, 1000, 0)
	if err != nil {
		return fmt.Errorf("list movies: %w", err)
	}

	var movie *movies.Movie
	for _, m := range existingMovies {
		if m.TMDBID != nil && *m.TMDBID == tmdbID {
			movie = m
			break
		}
	}

	if movie == nil {
		// Create the movie
		slug := slugify(meta.Title)
		if meta.Year > 0 {
			slug = fmt.Sprintf("%s-%d", slug, meta.Year)
		}

		now := time.Now()
		movie = &movies.Movie{
			ID:               slug,
			Title:            meta.Title,
			Year:             meta.Year,
			Overview:         meta.Overview,
			PosterPath:       meta.PosterPath,
			ReleaseDate:      meta.ReleaseDate,
			Runtime:          meta.Runtime,
			Rating:           meta.Rating,
			Status:           movies.MovieStatusMissing,
			MonitoringStatus: movies.MonitoringStatusMonitored,
			LibraryID:        libraryID,
			QualityProfileID: qualityProfileID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if meta.TMDBID != nil {
			movie.TMDBID = meta.TMDBID
		}
		if meta.IMDBID != nil {
			movie.IMDBID = meta.IMDBID
		}
		movie.Genres = meta.Genres

		if err := s.movieSvc.AddMovie(ctx, movie); err != nil {
			// If duplicate, try to find existing
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
				existing, _ := s.movieSvc.GetMovie(ctx, slug)
				if existing != nil {
					movie = existing
				} else {
					return fmt.Errorf("add movie: %w", err)
				}
			} else {
				return fmt.Errorf("add movie: %w", err)
			}
		}
	}

	// Add the movie file
	now := time.Now()
	mf := &movies.MovieFile{
		ID:        uuid.New().String()[:8],
		MovieID:   movie.ID,
		FilePath:  filePath,
		Size:      size,
		Quality:   quality,
		Format:    source,
		DateAdded: now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Update movie status based on file quality vs profile
	movie.Status = movies.MovieStatusAvailableRightQuality
	movie.UpdatedAt = now
	if err := s.movieSvc.UpdateMovie(ctx, movie); err != nil {
		s.logger.Warn("failed to update movie status", "movieId", movie.ID, "err", err)
	}

	// We need to add the file via service layer
	if err := s.movieSvc.AddMovieFile(ctx, mf); err != nil {
		// Skip if duplicate
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			s.logger.Debug("file already imported", "path", filePath)
			return nil
		}
		return fmt.Errorf("add movie file: %w", err)
	}

	s.logger.Info("imported movie file",
		"movie", movie.Title,
		"file", filePath,
		"quality", quality,
	)

	return nil
}

// autoMatch tries to find an exact match from TMDB results.
func autoMatch(title string, year int, results []*metadata.MovieMetadata) *metadata.MovieMetadata {
	normTitle := normalizeTitle(title)

	for _, r := range results {
		normResult := normalizeTitle(r.Title)
		if normTitle == normResult {
			if year == 0 || r.Year == 0 || r.Year == year || abs(r.Year-year) <= 1 {
				return r
			}
		}
	}
	return nil
}

func normalizeTitle(title string) string {
	t := strings.ToLower(title)
	// Remove articles
	for _, article := range []string{"the ", "a ", "an "} {
		t = strings.TrimPrefix(t, article)
	}
	// Remove non-alphanumeric
	var b strings.Builder
	for _, r := range t {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func qualityFromResolution(res int) string {
	switch {
	case res >= 2160:
		return "2160p"
	case res >= 1080:
		return "1080p"
	case res >= 720:
		return "720p"
	case res >= 480:
		return "480p"
	default:
		return "unknown"
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func slugify(s string) string {
	lower := strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}

// RescanMovie rescans a single movie's folder for updated files.
func (s *Scanner) RescanMovie(ctx context.Context, movieID, libraryPath string) (*ScanResult, error) {
	movie, err := s.movieSvc.GetMovie(ctx, movieID)
	if err != nil {
		return nil, fmt.Errorf("get movie: %w", err)
	}

	scanID := uuid.New().String()[:8]
	result := &ScanResult{
		ID:             scanID,
		LibraryID:      movie.LibraryID,
		RootFolderPath: libraryPath,
		Status:         ScanStatusRunning,
		StartedAt:      time.Now(),
	}

	s.mu.Lock()
	s.scans[scanID] = result
	s.unmatched[scanID] = nil
	s.mu.Unlock()

	// Walk the library path looking for files matching this movie
	scanned, walkErr := walkFolder(libraryPath)
	if walkErr != nil {
		s.failScan(scanID, walkErr.Error())
		return result, walkErr
	}

	s.mu.Lock()
	result.TotalFiles = len(scanned)
	s.mu.Unlock()

	normMovieTitle := normalizeTitle(movie.Title)

	for _, sf := range scanned {
		normParsed := normalizeTitle(sf.Rel.Title)
		if normParsed != normMovieTitle {
			continue
		}
		// Year check (allow ±1)
		if movie.Year > 0 && sf.Rel.Year > 0 && abs(movie.Year-sf.Rel.Year) > 1 {
			continue
		}

		quality := qualityFromResolution(sf.Rel.Resolution)
		if err := s.importFile(ctx, sf.Path, sf.Size, quality, sf.Rel.Source, &metadata.MovieMetadata{
			Title: movie.Title,
			Year:  movie.Year,
			TMDBID: movie.TMDBID,
			IMDBID: movie.IMDBID,
		}, movie.LibraryID, movie.QualityProfileID); err != nil {
			s.logger.Warn("rescan: failed to import", "movie", movie.Title, "path", sf.Path, "err", err)
			continue
		}

		s.mu.Lock()
		result.Matched++
		result.Imported++
		s.mu.Unlock()
	}

	now := time.Now()
	s.mu.Lock()
	result.Status = ScanStatusCompleted
	result.CompletedAt = &now
	s.mu.Unlock()

	s.logger.Info("movie rescan completed", "movie", movie.Title, "matched", result.Matched)
	return result, nil
}
