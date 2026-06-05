package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/kernel/telemetry"
	"github.com/ebenderooock/loom/internal/mediafiles"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
)

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
	ID          string `json:"id"`
	ScanID      string `json:"scanId"`
	FilePath    string `json:"filePath"`
	Size        int64  `json:"size"`
	ParsedTitle string `json:"parsedTitle"`
	ParsedYear  int    `json:"parsedYear"`
	Quality     string `json:"quality"`
	Source      string `json:"source"`
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
	audit    *auditlog.Logger

	mu        sync.RWMutex
	scans     map[string]*ScanResult
	unmatched map[string][]*UnmatchedFile // scanID -> files
}

// New creates a new Scanner.
func New(movieSvc movies.Service, meta MetadataSearcher, logger *slog.Logger, opts ...Option) *Scanner {
	s := &Scanner{
		movieSvc:  movieSvc,
		metadata:  meta,
		logger:    logger,
		scans:     make(map[string]*ScanResult),
		unmatched: make(map[string][]*UnmatchedFile),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option configures a Scanner.
type Option func(*Scanner)

// WithAuditLogger sets the audit logger for scan events.
func WithAuditLogger(a *auditlog.Logger) Option {
	return func(s *Scanner) { s.audit = a }
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
	var startedAt time.Time
	if result, ok := s.scans[scanID]; ok {
		result.Status = ScanStatusFailed
		result.Errors = append(result.Errors, errMsg)
		result.CompletedAt = &now
		startedAt = result.StartedAt
	}
	s.mu.Unlock()
	s.logger.Error("scan failed", "scanId", scanID, "error", errMsg)
	if m := telemetry.App(); m != nil {
		m.ScanTotal.WithLabelValues("movie", "failed").Inc()
		if !startedAt.IsZero() {
			m.ScanDuration.WithLabelValues("movie").Observe(time.Since(startedAt).Seconds())
		}
	}
}

func (s *Scanner) runScan(ctx context.Context, scanID string, libraryPath string) {
	scanStart := time.Now()
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

	if m := telemetry.App(); m != nil {
		m.ScanTotal.WithLabelValues("movie", "success").Inc()
		m.ScanDuration.WithLabelValues("movie").Observe(time.Since(scanStart).Seconds())
		m.ScanFilesProcessed.WithLabelValues("movie", "matched").Add(float64(result.Matched))
		m.ScanFilesProcessed.WithLabelValues("movie", "unmatched").Add(float64(result.Unmatched))
		m.ScanFilesProcessed.WithLabelValues("movie", "error").Add(float64(len(result.Errors)))
	}
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
		if !mediafiles.IsVideo(ext) {
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
	quality := qualityFromParsedInfo(sf.Rel.Resolution, sf.Rel.Source, sf.Rel.IsRemux)

	if title == "" {
		s.addUnmatched(scanID, result, sf, "")
		return nil
	}

	// Try local library match first to avoid unnecessary TMDB calls
	localMovie, localErr := s.matchLocalMovie(ctx, title, year)
	if localErr == nil && localMovie != nil {
		s.mu.Lock()
		result.Matched++
		s.mu.Unlock()

		meta := &metadata.MovieMetadata{
			Title:      localMovie.Title,
			Year:       localMovie.Year,
			Overview:   localMovie.Overview,
			PosterPath: localMovie.PosterPath,
			Runtime:    localMovie.Runtime,
			Rating:     localMovie.Rating,
			TMDBID:     localMovie.TMDBID,
			IMDBID:     localMovie.IMDBID,
		}

		if err := s.importFile(ctx, sf.Path, sf.Size, quality, sf.Rel.Source, meta, libraryID, ""); err != nil {
			return err
		}

		s.mu.Lock()
		result.Imported++
		s.mu.Unlock()
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
		Quality:     qualityFromParsedInfo(sf.Rel.Resolution, sf.Rel.Source, sf.Rel.IsRemux),
		Source:      sf.Rel.Source,
	}
	s.mu.Lock()
	result.Unmatched++
	s.unmatched[scanID] = append(s.unmatched[scanID], uf)
	s.mu.Unlock()
}

func (s *Scanner) findExistingFileByPath(ctx context.Context, path string) (bool, error) {
	mf, err := s.movieSvc.GetMovieFileByPath(ctx, path)
	if err != nil {
		return false, err
	}
	return mf != nil, nil
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

	// Try to add the file first — only update status if it persists
	if err := s.movieSvc.AddMovieFile(ctx, mf); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			// A soft-deleted row may be blocking — try to revive it
			existing, _ := s.movieSvc.GetMovieFileByPathIncludingDeleted(ctx, filePath)
			if existing != nil && existing.DeletedAt != nil {
				if reviveErr := s.movieSvc.ReviveMovieFile(ctx, existing.ID, mf); reviveErr != nil {
					return fmt.Errorf("revive movie file: %w", reviveErr)
				}
				s.logger.Info("revived soft-deleted movie file", "path", filePath)
			} else {
				s.logger.Debug("file already imported", "path", filePath)
				// Still ensure movie status is updated — file exists but
				// status may be stale (e.g. "missing" or "unavailable").
				if err := s.movieSvc.SetMovieStatus(ctx, movie.ID, movies.MovieStatusAvailableRightQuality); err != nil {
					s.logger.Warn("failed to update movie status for existing file", "movie", movie.ID, "error", err)
				}
				return nil
			}
		} else {
			return fmt.Errorf("add movie file: %w", err)
		}
	}

	// File persisted — now update movie status
	if err := s.movieSvc.SetMovieStatus(ctx, movie.ID, movies.MovieStatusAvailableRightQuality); err != nil {
		return fmt.Errorf("update movie status after import: %w", err)
	}

	s.logger.Info("imported movie file",
		"movie", movie.Title,
		"file", filePath,
		"quality", quality,
	)

	// Write audit entry
	if s.audit != nil {
		s.audit.LogBackground(auditlog.Entry{
			Category:   "scan",
			EventType:  "scan.imported",
			Message:    fmt.Sprintf("File imported: %s (%s)", movie.Title, quality),
			Detail:     auditlog.DetailJSON(map[string]any{"quality": quality, "source": source, "file_path": filePath, "size": size}),
			EntityType: auditlog.StrPtr("movie"),
			EntityID:   auditlog.StrPtr(movie.ID),
			EntityName: auditlog.StrPtr(movie.Title),
			Level:      "info",
			Source:     auditlog.StrPtr("scanner"),
		})
	}

	return nil
}

// autoMatch tries to find a match from TMDB results using multiple strategies.
func autoMatch(title string, year int, results []*metadata.MovieMetadata) *metadata.MovieMetadata {
	normTitle := normalizeTitle(title)

	// Pass 1: exact normalized match (most reliable)
	for _, r := range results {
		normResult := normalizeTitle(r.Title)
		if normTitle == normResult {
			if year == 0 || r.Year == 0 || r.Year == year || abs(r.Year-year) <= 1 {
				return r
			}
		}
	}

	// Pass 2: containment match — if one title contains the other
	for _, r := range results {
		normResult := normalizeTitle(r.Title)
		if strings.Contains(normTitle, normResult) || strings.Contains(normResult, normTitle) {
			if year == 0 || r.Year == 0 || r.Year == year || abs(r.Year-year) <= 1 {
				return r
			}
		}
	}

	// Pass 3: token overlap with high threshold
	for _, r := range results {
		score := tokenSimilarity(title, r.Title)
		if score >= 80 {
			if year == 0 || r.Year == 0 || r.Year == year || abs(r.Year-year) <= 1 {
				return r
			}
		}
	}

	return nil
}

func normalizeTitle(title string) string {
	t := strings.ToLower(title)
	// Collapse acronyms: "m.i.a" → "mia"
	t = collapseAcronymsScan(t)
	// Expand & to "and" before stripping punctuation
	t = strings.ReplaceAll(t, "&", " and ")
	// Strip possessives
	t = strings.ReplaceAll(t, "'s", "s")
	t = strings.ReplaceAll(t, "\u2019s", "s")
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

// normalizeTitleForTokens normalizes a title but preserves spaces for tokenization.
func normalizeTitleForTokens(title string) string {
	t := strings.ToLower(title)
	t = strings.ReplaceAll(t, "&", " and ")
	t = strings.ReplaceAll(t, "'s", "s")
	t = strings.ReplaceAll(t, "\u2019s", "s")
	for _, article := range []string{"the ", "a ", "an "} {
		t = strings.TrimPrefix(t, article)
	}
	var b strings.Builder
	for _, r := range t {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// collapseAcronymsScan replaces dot- and space-separated single-letter
// sequences (e.g., "m.i.a" → "mia", "m i a" → "mia").
func collapseAcronymsScan(s string) string {
	dotRe := regexp.MustCompile(`(?:^|[^a-zA-Z])((?:[a-zA-Z]\.){2,}[a-zA-Z]?)`)
	s = dotRe.ReplaceAllStringFunc(s, func(m string) string {
		prefix := ""
		start := 0
		if len(m) > 0 && !unicode.IsLetter(rune(m[0])) {
			prefix = string(m[0])
			start = 1
		}
		return prefix + strings.ReplaceAll(m[start:], ".", "")
	})

	spaceRe := regexp.MustCompile(`(?:^|[^a-zA-Z])((?:[a-zA-Z] ){2,}[a-zA-Z])(?:[^a-zA-Z]|$)`)
	s = spaceRe.ReplaceAllStringFunc(s, func(m string) string {
		prefix := ""
		suffix := ""
		start := 0
		end := len(m)
		if len(m) > 0 && !unicode.IsLetter(rune(m[0])) {
			prefix = string(m[0])
			start = 1
		}
		if end > 0 && !unicode.IsLetter(rune(m[end-1])) {
			suffix = string(m[end-1])
			end--
		}
		return prefix + strings.ReplaceAll(m[start:end], " ", "") + suffix
	})

	return s
}

// tokenize splits a title into meaningful words, removing stop words.
func tokenize(s string) []string {
	words := strings.Fields(normalizeTitleForTokens(s))
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true,
		"or": true, "of": true, "in": true, "to": true,
		"for": true, "is": true,
	}
	out := make([]string, 0, len(words))
	for _, w := range words {
		if !stopWords[w] && len(w) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// tokenSimilarity computes token-level similarity between two titles.
func tokenSimilarity(a, b string) int {
	aToks := tokenize(a)
	bToks := tokenize(b)
	if len(aToks) == 0 || len(bToks) == 0 {
		return 0
	}
	aSet := make(map[string]bool, len(aToks))
	for _, w := range aToks {
		aSet[w] = true
	}
	bSet := make(map[string]bool, len(bToks))
	for _, w := range bToks {
		bSet[w] = true
	}
	intersection := 0
	for w := range aSet {
		if bSet[w] {
			intersection++
		}
	}
	minSize := len(aSet)
	if len(bSet) < minSize {
		minSize = len(bSet)
	}
	if minSize == 0 {
		return 0
	}
	return intersection * 100 / minSize
}

// qualityFromParsedInfo maps parsed release info to a canonical quality
// definition name matching the seeded QualityDefinition.Name values.
func qualityFromParsedInfo(resolution int, source string, isRemux bool) string {
	res := ""
	switch {
	case resolution >= 2160:
		res = "2160p"
	case resolution >= 1080:
		res = "1080p"
	case resolution >= 720:
		res = "720p"
	case resolution >= 480:
		res = "480p"
	default:
		return "unknown"
	}

	// Remux takes priority over source
	if isRemux {
		switch res {
		case "2160p":
			return "bluray-2160p-remux"
		case "1080p":
			return "bluray-1080p-remux"
		default:
			return "bluray-" + res
		}
	}

	src := strings.ToLower(source)
	switch {
	case strings.Contains(src, "bluray") || strings.Contains(src, "blu-ray"):
		return "bluray-" + res
	case strings.Contains(src, "webdl") || strings.Contains(src, "web-dl"):
		return "webdl-" + res
	case strings.Contains(src, "webrip") || strings.Contains(src, "web"):
		return "webrip-" + res
	case strings.Contains(src, "hdtv"):
		return "hdtv-" + res
	case strings.Contains(src, "dvd"):
		if res == "480p" {
			return "dvd"
		}
		return "sdtv"
	default:
		// Bare resolution fallback — pick the most common source for the resolution
		switch res {
		case "2160p":
			return "webdl-2160p"
		case "1080p":
			return "webdl-1080p"
		case "720p":
			return "webdl-720p"
		case "480p":
			return "sdtv"
		default:
			return "unknown"
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// matchLocalMovie checks the local library for a movie matching the given title and year.
func (s *Scanner) matchLocalMovie(ctx context.Context, title string, year int) (*movies.Movie, error) {
	allMovies, err := s.movieSvc.ListMovies(ctx, 0, 0)
	if err != nil {
		return nil, err
	}

	normTitle := normalizeTitle(title)

	// Pass 1: exact normalized title + year
	for _, m := range allMovies {
		normM := normalizeTitle(m.Title)
		if normTitle == normM {
			if year == 0 || m.Year == 0 || m.Year == year || abs(m.Year-year) <= 1 {
				return m, nil
			}
		}
	}

	// Pass 2: fuzzy token match with high threshold
	for _, m := range allMovies {
		score := tokenSimilarity(title, m.Title)
		if score >= 80 {
			if year == 0 || m.Year == 0 || m.Year == year || abs(m.Year-year) <= 1 {
				return m, nil
			}
		}
	}

	return nil, nil
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
// It first tries to scan the movie's specific folder (derived from existing
// files or the naming convention). If no movie folder is found it falls back
// to scanning the entire library root. Matching uses multi-pass fuzzy logic
// (exact → containment → token similarity) so slightly different folder/file
// names still get picked up.
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

	// Determine the best folder to scan:
	// 1. Movie's folder derived from existing file paths
	// 2. Folder matching the naming convention "{Title} ({Year})"
	// 3. Fall back to the entire library root
	scanPath := s.resolveMovieFolder(ctx, movie, libraryPath)

	s.logger.Info("rescan: scanning folder", "movie", movie.Title, "path", scanPath)

	scanned, walkErr := walkFolder(scanPath)
	if walkErr != nil {
		s.failScan(scanID, walkErr.Error())
		return result, walkErr
	}

	s.mu.Lock()
	result.TotalFiles = len(scanned)
	s.mu.Unlock()

	for _, sf := range scanned {
		if !matchesMovie(sf.Rel.Title, sf.Rel.Year, movie.Title, movie.Year) {
			continue
		}

		quality := qualityFromParsedInfo(sf.Rel.Resolution, sf.Rel.Source, sf.Rel.IsRemux)
		if err := s.importFile(ctx, sf.Path, sf.Size, quality, sf.Rel.Source, &metadata.MovieMetadata{
			Title:  movie.Title,
			Year:   movie.Year,
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

// resolveMovieFolder determines the folder to scan for a specific movie.
// Priority: existing file → naming convention folder → library root.
func (s *Scanner) resolveMovieFolder(ctx context.Context, movie *movies.Movie, libraryPath string) string {
	// Try to derive from existing movie files
	files, err := s.movieSvc.ListMovieFiles(ctx, movie.ID)
	if err == nil && len(files) > 0 {
		folder := filepath.Dir(files[0].FilePath)
		if folder != libraryPath && folder != "." {
			if _, statErr := os.Stat(folder); statErr == nil {
				return folder
			}
		}
	}

	// Try the conventional folder name: "{Title} ({Year})"
	conventionalName := movie.Title
	if movie.Year > 0 {
		conventionalName = fmt.Sprintf("%s (%d)", movie.Title, movie.Year)
	}
	conventionalPath := filepath.Join(libraryPath, conventionalName)
	if _, statErr := os.Stat(conventionalPath); statErr == nil {
		return conventionalPath
	}

	// Try finding a folder in the library that fuzzy-matches the movie title
	entries, err := os.ReadDir(libraryPath)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			parsed := parser.Parse(entry.Name())
			if matchesMovie(parsed.Title, parsed.Year, movie.Title, movie.Year) {
				return filepath.Join(libraryPath, entry.Name())
			}
		}
	}

	// Fall back to entire library root
	return libraryPath
}

// matchesMovie uses multi-pass matching (exact, containment, token similarity)
// to determine if a parsed title+year matches a movie. Year is allowed ±1.
func matchesMovie(parsedTitle string, parsedYear int, movieTitle string, movieYear int) bool {
	if parsedTitle == "" {
		return false
	}

	yearOK := func(py, my int) bool {
		return py == 0 || my == 0 || py == my || abs(py-my) <= 1
	}

	normParsed := normalizeTitle(parsedTitle)
	normMovie := normalizeTitle(movieTitle)

	// Pass 1: exact normalized
	if normParsed == normMovie && yearOK(parsedYear, movieYear) {
		return true
	}

	// Pass 2: containment
	if (strings.Contains(normParsed, normMovie) || strings.Contains(normMovie, normParsed)) && yearOK(parsedYear, movieYear) {
		return true
	}

	// Pass 3: token similarity ≥ 80%
	if tokenSimilarity(parsedTitle, movieTitle) >= 80 && yearOK(parsedYear, movieYear) {
		return true
	}

	return false
}
