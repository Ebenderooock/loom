package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ebenderooock/loom/internal/kernel/telemetry"
	"github.com/ebenderooock/loom/internal/mediafiles"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/series"
)

// SeriesScanner orchestrates TV show discovery and episode file scanning.
type SeriesScanner struct {
	seriesSvc series.Service
	logger    *slog.Logger

	mu        sync.RWMutex
	scans     map[string]*ScanResult
	unmatched map[string][]*UnmatchedFile
}

// NewSeriesScanner creates a new SeriesScanner.
func NewSeriesScanner(seriesSvc series.Service, logger *slog.Logger) *SeriesScanner {
	return &SeriesScanner{
		seriesSvc: seriesSvc,
		logger:    logger,
		scans:     make(map[string]*ScanResult),
		unmatched: make(map[string][]*UnmatchedFile),
	}
}

// evictOldScans removes the oldest completed scans when history exceeds maxScanHistory.
// Must be called with s.mu held.
func (s *SeriesScanner) evictOldScans() {
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

const maxScanHistory = 20

var seasonDirRe = regexp.MustCompile(`(?i)(?:season|s)\s*(\d+)`)

// showFolderNameRe extracts a title and optional year from a folder name like "Breaking Bad (2008)".
var showFolderNameRe = regexp.MustCompile(`^(.+?)\s*(?:\((\d{4})\))?\s*$`)

// StartSeriesScan begins an async scan of the given root folder path.
func (s *SeriesScanner) StartSeriesScan(ctx context.Context, libraryID, rootFolder string) (string, error) {
	info, err := os.Stat(rootFolder)
	if err != nil {
		return "", fmt.Errorf("series scan: stat root folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("series scan: %s is not a directory", rootFolder)
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
	s.evictOldScans()
	s.mu.Unlock()

	go s.runSeriesScan(context.Background(), scanID, libraryID, rootFolder)

	return scanID, nil
}

// GetSeriesScanStatus returns the current state of a series scan job.
func (s *SeriesScanner) GetSeriesScanStatus(scanID string) *ScanResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scans[scanID]
}

// GetSeriesUnmatched returns all unmatched files across all series scans.
func (s *SeriesScanner) GetSeriesUnmatched() []*UnmatchedFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []*UnmatchedFile
	for _, files := range s.unmatched {
		all = append(all, files...)
	}
	return all
}

func (s *SeriesScanner) runSeriesScan(ctx context.Context, scanID, libraryID, rootFolder string) {
	scanStart := time.Now()
	s.logger.Info("starting series scan", "scanId", scanID, "libraryId", libraryID, "path", rootFolder)

	// Phase 1: Discover show folders and add new series via TMDB
	showFolders, err := discoverShowFolders(rootFolder)
	if err != nil {
		s.failSeriesScan(scanID, fmt.Sprintf("discover show folders: %v", err))
		return
	}

	s.logger.Info("discovered show folders", "scanId", scanID, "count", len(showFolders))

	existingSeries, err := s.seriesSvc.ListSeries(ctx)
	if err != nil {
		s.failSeriesScan(scanID, fmt.Sprintf("list existing series: %v", err))
		return
	}

	// Build lookup sets for existing series
	existingByTMDB := make(map[string]bool)
	existingByTitle := make(map[string]bool)
	for _, sr := range existingSeries {
		if sr.TMDBID != nil && *sr.TMDBID != "" {
			existingByTMDB[*sr.TMDBID] = true
		}
		existingByTitle[normalizeTitle(sr.Title)] = true
	}

	result := s.GetSeriesScanStatus(scanID)
	for _, sf := range showFolders {
		title, year := parseShowFolderName(sf.Name)
		if title == "" {
			continue
		}

		normTitle := normalizeTitle(title)
		if existingByTitle[normTitle] {
			s.logger.Debug("series already exists by title, skipping TMDB add", "title", title)
			continue
		}

		tmdbResults, err := s.seriesSvc.SearchTMDB(ctx, title)
		if err != nil {
			s.logger.Warn("TMDB search failed for show folder", "title", title, "err", err)
			continue
		}

		tmdbMatch := autoMatchSeries(title, year, tmdbResults)
		if tmdbMatch == nil {
			s.logger.Debug("no TMDB match for show folder", "title", title, "year", year)
			continue
		}

		tmdbID := tmdbMatch.tmdbID
		if existingByTMDB[tmdbID] {
			s.logger.Debug("series already exists by TMDB ID, skipping", "tmdbId", tmdbID)
			continue
		}

		addReq := &series.AddSeriesRequest{
			TMDBID:           tmdbID,
			LibraryID:        libraryID,
			MonitoringStatus: "all",
		}
		added, err := s.seriesSvc.AddSeries(ctx, addReq)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "already exists") {
				s.logger.Debug("series already exists, skipping", "tmdbId", tmdbID)
			} else {
				s.logger.Warn("failed to add series from TMDB", "tmdbId", tmdbID, "title", title, "err", err)
				s.mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("add series %q: %v", title, err))
				s.mu.Unlock()
			}
			continue
		}

		existingByTMDB[tmdbID] = true
		if added != nil {
			existingByTitle[normalizeTitle(added.Title)] = true
		}
		s.logger.Info("added series from TMDB", "title", title, "tmdbId", tmdbID)
	}

	// Phase 2: Walk each show folder, match files using folder-based series identity
	// (Sonarr approach: the show folder determines the series, not the filename)
	existingSeries, err = s.seriesSvc.ListSeries(ctx)
	if err != nil {
		s.failSeriesScan(scanID, fmt.Sprintf("reload series: %v", err))
		return
	}

	// Build lookup: normalizedTitle → *series.Series
	seriesByTitle := make(map[string]*series.Series)
	for _, sr := range existingSeries {
		seriesByTitle[normalizeTitle(sr.Title)] = sr
	}

	for _, sf := range showFolders {
		title, _ := parseShowFolderName(sf.Name)
		if title == "" {
			continue
		}

		matched := seriesByTitle[normalizeTitle(title)]
		if matched == nil {
			s.logger.Debug("no DB series for show folder, skipping episode scan", "folder", sf.Name)
			continue
		}

		// Walk this show folder for video files
		showFiles, walkErr := walkSeriesFolder(sf.Path)
		if walkErr != nil {
			s.logger.Warn("error walking show folder", "path", sf.Path, "err", walkErr)
			continue
		}

		s.mu.Lock()
		result.TotalFiles += len(showFiles)
		s.mu.Unlock()

		for _, ef := range showFiles {
			if err := s.processEpisodeFileForSeries(ctx, scanID, matched, ef.Path, ef.Size); err != nil {
				s.logger.Warn("failed to process episode file", "path", ef.Path, "err", err)
				s.mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", ef.Path, err))
				s.mu.Unlock()
			}
		}
	}

	now := time.Now()
	s.mu.Lock()
	result.Status = ScanStatusCompleted
	result.CompletedAt = &now
	total, matched, unmatched, imported := result.TotalFiles, result.Matched, result.Unmatched, result.Imported
	s.mu.Unlock()

	s.logger.Info("series scan completed",
		"scanId", scanID,
		"total", total,
		"matched", matched,
		"unmatched", unmatched,
		"imported", imported,
	)

	if m := telemetry.App(); m != nil {
		m.ScanTotal.WithLabelValues("series", "success").Inc()
		m.ScanDuration.WithLabelValues("series").Observe(time.Since(scanStart).Seconds())
		m.ScanFilesProcessed.WithLabelValues("series", "matched").Add(float64(matched))
		m.ScanFilesProcessed.WithLabelValues("series", "unmatched").Add(float64(unmatched))
		m.ScanFilesProcessed.WithLabelValues("series", "error").Add(float64(len(result.Errors)))
	}
}

// showFolder represents a top-level directory in the library root.
type showFolder struct {
	Name string
	Path string
}

// discoverShowFolders enumerates top-level subdirectories in the library root.
func discoverShowFolders(root string) ([]showFolder, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", root, err)
	}

	var folders []showFolder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if shouldIgnore(e.Name()) {
			continue
		}
		folders = append(folders, showFolder{
			Name: e.Name(),
			Path: filepath.Join(root, e.Name()),
		})
	}
	return folders, nil
}

// parseShowFolderName extracts a title and optional year from a folder name.
// E.g. "Breaking Bad (2008)" → ("Breaking Bad", 2008)
// E.g. "The Office" → ("The Office", 0)
func parseShowFolderName(name string) (string, int) {
	m := showFolderNameRe.FindStringSubmatch(strings.TrimSpace(name))
	if len(m) < 2 {
		return name, 0
	}
	title := strings.TrimSpace(m[1])
	year := 0
	if len(m) >= 3 && m[2] != "" {
		year, _ = strconv.Atoi(m[2])
	}
	return title, year
}

// tmdbSeriesMatch holds a parsed TMDB search result for matching.
type tmdbSeriesMatch struct {
	tmdbID string
	name   string
	year   int
}

// autoMatchSeries picks the best TMDB result by normalized title and optional year.
func autoMatchSeries(title string, year int, results []map[string]interface{}) *tmdbSeriesMatch {
	normTitle := normalizeTitle(title)

	for _, r := range results {
		name, _ := r["name"].(string)
		if name == "" {
			// Some results use "original_name"
			name, _ = r["original_name"].(string)
		}
		if normalizeTitle(name) != normTitle {
			continue
		}

		tmdbID := tmdbIDFromResult(r)
		if tmdbID == "" {
			continue
		}

		resultYear := extractYearFromResult(r)
		if year == 0 || resultYear == 0 || abs(year-resultYear) <= 1 {
			return &tmdbSeriesMatch{tmdbID: tmdbID, name: name, year: resultYear}
		}
	}

	// If no exact+year match, take the first title match
	for _, r := range results {
		name, _ := r["name"].(string)
		if name == "" {
			name, _ = r["original_name"].(string)
		}
		if normalizeTitle(name) != normTitle {
			continue
		}
		tmdbID := tmdbIDFromResult(r)
		if tmdbID == "" {
			continue
		}
		return &tmdbSeriesMatch{tmdbID: tmdbID, name: name, year: extractYearFromResult(r)}
	}
	return nil
}

func tmdbIDFromResult(r map[string]interface{}) string {
	switch v := r["id"].(type) {
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	case string:
		return v
	default:
		return ""
	}
}

func extractYearFromResult(r map[string]interface{}) int {
	if dateStr, ok := r["first_air_date"].(string); ok && len(dateStr) >= 4 {
		y, _ := strconv.Atoi(dateStr[:4])
		return y
	}
	return 0
}

// processEpisodeFileForSeries matches a file to a known series using folder context.
// The series identity is already known from the show folder; we only extract season+episode.
func (s *SeriesScanner) processEpisodeFileForSeries(ctx context.Context, scanID string, matched *series.Series, path string, size int64) error {
	result := s.GetSeriesScanStatus(scanID)

	fileName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parsed := parser.Parse(fileName)

	season := parsed.Season
	episode := parsed.Episode

	// Try climbing parent directories for season (handles nested structures)
	if season < 0 {
		dir := filepath.Dir(path)
		for i := 0; i < 3; i++ {
			season = extractSeasonFromDir(dir)
			if season >= 0 {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if season < 0 || episode < 0 {
		s.addSeriesUnmatched(scanID, result, path, size, parsed, matched.Title)
		return nil
	}

	// Find episode by season + episode number
	ep, err := s.findEpisode(ctx, matched.ID, season, episode)
	if err != nil || ep == nil {
		s.addSeriesUnmatched(scanID, result, path, size, parsed, matched.Title)
		return nil
	}

	s.mu.Lock()
	result.Matched++
	s.mu.Unlock()

	if err := s.importEpisodeFile(ctx, ep, path, size, parsed); err != nil {
		return err
	}

	s.mu.Lock()
	result.Imported++
	s.mu.Unlock()

	return nil
}

func (s *SeriesScanner) importEpisodeFile(ctx context.Context, ep *series.Episode, path string, size int64, parsed *parser.Release) error {
	now := time.Now()
	f := &series.EpisodeFile{
		ID:         uuid.New().String()[:8],
		EpisodeID:  ep.ID,
		SeriesID:   ep.SeriesID,
		FilePath:   path,
		FileSize:   size,
		Quality:    qualityFromParsedInfo(parsed.Resolution, parsed.Source, parsed.IsRemux),
		Source:     parsed.Source,
		Resolution: resolutionString(parsed.Resolution),
		Codec:      parsed.Codec,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.seriesSvc.CreateEpisodeFile(ctx, f); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			s.logger.Debug("episode file already imported", "path", path)
			return nil
		}
		return fmt.Errorf("create episode file: %w", err)
	}

	// Update episode has_file = true
	ep.HasFile = true
	ep.UpdatedAt = now
	if err := s.seriesSvc.UpdateEpisode(ctx, ep); err != nil {
		s.logger.Warn("failed to update episode has_file", "episodeId", ep.ID, "err", err)
	}

	s.logger.Info("imported episode file",
		"series", ep.SeriesID,
		"season", ep.SeasonID,
		"episode", ep.EpisodeNumber,
		"file", path,
	)

	return nil
}

func (s *SeriesScanner) findSeriesByTitle(ctx context.Context, title string) (*series.Series, error) {
	all, err := s.seriesSvc.ListSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}

	normTitle := normalizeTitle(title)
	for _, sr := range all {
		if normalizeTitle(sr.Title) == normTitle {
			return sr, nil
		}
	}
	return nil, nil
}

func (s *SeriesScanner) findEpisode(ctx context.Context, seriesID string, seasonNum, episodeNum int) (*series.Episode, error) {
	episodes, err := s.seriesSvc.ListEpisodes(ctx, seriesID, &seasonNum)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}

	for _, ep := range episodes {
		if ep.EpisodeNumber == episodeNum {
			return ep, nil
		}
	}
	return nil, nil
}

func (s *SeriesScanner) addSeriesUnmatched(scanID string, result *ScanResult, path string, size int64, parsed *parser.Release, title string) {
	uf := &UnmatchedFile{
		ID:          uuid.New().String()[:8],
		ScanID:      scanID,
		FilePath:    path,
		Size:        size,
		ParsedTitle: title,
		ParsedYear:  parsed.Year,
		Quality:     qualityFromParsedInfo(parsed.Resolution, parsed.Source, parsed.IsRemux),
		Source:      parsed.Source,
	}
	s.mu.Lock()
	result.Unmatched++
	s.unmatched[scanID] = append(s.unmatched[scanID], uf)
	s.mu.Unlock()
}

func (s *SeriesScanner) failSeriesScan(scanID string, errMsg string) {
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
	s.logger.Error("series scan failed", "scanId", scanID, "error", errMsg)
	if m := telemetry.App(); m != nil {
		m.ScanTotal.WithLabelValues("series", "failed").Inc()
		if !startedAt.IsZero() {
			m.ScanDuration.WithLabelValues("series").Observe(time.Since(startedAt).Seconds())
		}
	}
}

// extractSeasonFromDir extracts a season number from a directory name
// like "Season 01", "Season 1", "S01", "s01".
func extractSeasonFromDir(dir string) int {
	if m := seasonDirRe.FindStringSubmatch(filepath.Base(dir)); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return -1
}

func resolutionString(res int) string {
	if res > 0 {
		return strconv.Itoa(res) + "p"
	}
	return ""
}

type seriesScannedFile struct {
	Path string
	Size int64
}

func walkSeriesFolder(root string) ([]seriesScannedFile, error) {
	var files []seriesScannedFile

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
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
		if info.Size() < 10*1024*1024 {
			return nil
		}

		files = append(files, seriesScannedFile{
			Path: path,
			Size: info.Size(),
		})
		return nil
	})

	return files, err
}

// RescanSeries rescans a single series folder for new/changed episode files.
func (s *SeriesScanner) RescanSeries(ctx context.Context, seriesID, libraryPath string) (*ScanResult, error) {
	sr, err := s.seriesSvc.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("get series: %w", err)
	}

	// Find the series folder within the library by matching the title to top-level dirs
	showFolders, err := discoverShowFolders(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("discover show folders: %w", err)
	}

	var showPath string
	normTarget := normalizeTitle(sr.Title)
	for _, sf := range showFolders {
		title, _ := parseShowFolderName(sf.Name)
		if normalizeTitle(title) == normTarget {
			showPath = sf.Path
			break
		}
	}

	if showPath == "" {
		return nil, fmt.Errorf("no folder found for series %q in %s", sr.Title, libraryPath)
	}

	scanID := uuid.New().String()[:8]
	result := &ScanResult{
		ID:             scanID,
		LibraryID:      sr.LibraryID,
		RootFolderPath: showPath,
		Status:         ScanStatusRunning,
		StartedAt:      time.Now(),
	}

	s.mu.Lock()
	s.scans[scanID] = result
	s.unmatched[scanID] = nil
	s.evictOldScans()
	s.mu.Unlock()

	files, walkErr := walkSeriesFolder(showPath)
	if walkErr != nil {
		s.failSeriesScan(scanID, walkErr.Error())
		return result, walkErr
	}

	s.mu.Lock()
	result.TotalFiles = len(files)
	s.mu.Unlock()

	for _, ef := range files {
		if err := s.processEpisodeFileForSeries(ctx, scanID, sr, ef.Path, ef.Size); err != nil {
			s.logger.Warn("rescan: failed to process episode file", "path", ef.Path, "err", err)
		}
	}

	now := time.Now()
	s.mu.Lock()
	result.Status = ScanStatusCompleted
	result.CompletedAt = &now
	s.mu.Unlock()

	s.logger.Info("series rescan completed", "series", sr.Title, "matched", result.Matched, "imported", result.Imported)
	return result, nil
}
