package imports

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/alttitles"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/mediafiles"
	"github.com/ebenderooock/loom/internal/movies"
	musiclib "github.com/ebenderooock/loom/internal/music"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/workflows"
)

// PipelineOptions configures the ImportPipeline.
type PipelineOptions struct {
	DB              *sql.DB
	Bus             eventbus.Bus
	DownloadSvc     *downloads.Service
	RemotePathStore *downloads.RemotePathStore
	MoviesSvc       movies.Service
	SeriesSvc       series.Service
	MusicSvc        musiclib.Service
	LibStore        *libraries.Store
	WorkflowEngine  *workflows.Engine
	NotifSvc        notifications.Service
	PostVal         *safety.PostValidator
	ReviewStore     *safety.ReviewStore
	Logger          *slog.Logger
	ImportMode      ImportMode
	RecycleBin      *RecycleBin
	QualityProfiles QualityProfileGetter
	AltTitleStore   *alttitles.Store
	// JobWorkers bounds concurrent queued manual imports (default 2). A
	// negative value disables automatic worker startup (tests call
	// StartJobs explicitly).
	JobWorkers int
}

// ImportPipeline subscribes to download completion events, scans files,
// matches them to library items, and imports them.
type ImportPipeline struct {
	db              *sql.DB
	bus             eventbus.Bus
	downloadSvc     *downloads.Service
	remotePathStore *downloads.RemotePathStore
	matcher         *Matcher
	wfEngine        *workflows.Engine
	moviesSvc       movies.Service
	musicSvc        musiclib.Service
	libStore        *libraries.Store
	notifSvc        notifications.Service
	postVal         *safety.PostValidator
	reviewStore     *safety.ReviewStore
	logger          *slog.Logger
	importMode      ImportMode
	decisions       *DecisionMaker
	decisionLog     *DecisionLogger
	subtitles       *SubtitleService
	extras          *ExtraService
	verifier        *ImportVerifier
	recycler        *RecycleBin
	cleaner         *FolderCleaner
	unsub           func()
	jobs            *jobQueue // durable, bounded manual import queue
	scans           *scanQueue
}

// NewPipeline creates and wires an ImportPipeline. Call Start to
// subscribe to the event bus.
func NewPipeline(opts PipelineOptions) (*ImportPipeline, error) {
	if opts.DB == nil {
		return nil, fmt.Errorf("imports: db required")
	}
	if opts.Bus == nil {
		return nil, fmt.Errorf("imports: event bus required")
	}
	if opts.DownloadSvc == nil {
		return nil, fmt.Errorf("imports: download service required")
	}
	if opts.MoviesSvc == nil {
		return nil, fmt.Errorf("imports: movies service required")
	}
	if opts.SeriesSvc == nil {
		return nil, fmt.Errorf("imports: series service required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ImportMode == "" {
		opts.ImportMode = ImportModeMove
	}

	logger := opts.Logger.With("module", "imports")
	matcher := NewMatcher(opts.MoviesSvc, opts.SeriesSvc, opts.LibStore)

	// Wire alternative-title matching if a store is provided.
	if opts.AltTitleStore != nil {
		matcher.SetAltTitleMatcher(NewAltTitleMatcher(opts.AltTitleStore, opts.MoviesSvc, opts.SeriesSvc))
	}

	// Build the import spec chain, including the upgrade spec when profiles are available.
	specs := []ImportSpec{
		&SampleSpec{},
		&FreeSpaceSpec{},
		&UnpackingSpec{},
		&DangerousFileSpec{},
		NewUpgradeSpec(opts.QualityProfiles),
	}

	p := &ImportPipeline{
		db:              opts.DB,
		bus:             opts.Bus,
		downloadSvc:     opts.DownloadSvc,
		remotePathStore: opts.RemotePathStore,
		matcher:         matcher,
		wfEngine:        opts.WorkflowEngine,
		moviesSvc:       opts.MoviesSvc,
		musicSvc:        opts.MusicSvc,
		libStore:        opts.LibStore,
		notifSvc:        opts.NotifSvc,
		postVal:         opts.PostVal,
		reviewStore:     opts.ReviewStore,
		logger:          logger,
		importMode:      opts.ImportMode,
		decisions:       NewDecisionMaker(specs...),
		decisionLog:     NewDecisionLogger(opts.DB, logger),
		subtitles:       &SubtitleService{},
		extras:          &ExtraService{},
		verifier:        &ImportVerifier{},
		recycler:        opts.RecycleBin,
		cleaner:         &FolderCleaner{},
		jobs:            newJobQueue(opts.JobWorkers),
		scans:           &scanQueue{tasks: make(map[string]*scanTask)},
	}
	// Queue workers run independently of the event-bus subscription so
	// queued and retrying imports resume after a restart. Stop halts them.
	if opts.JobWorkers >= 0 {
		p.StartJobs()
	}
	return p, nil
}

// Start subscribes to download completion events.
func (p *ImportPipeline) Start() {
	p.unsub = p.bus.Subscribe(downloads.TopicDownloadCompleted, p.handleCompleted)
	p.logger.Info("import pipeline started", "import_mode", p.importMode)
}

// Stop unsubscribes from the event bus.
func (p *ImportPipeline) Stop() {
	if p.unsub != nil {
		p.unsub()
	}
	p.stopScans()
	p.stopJobs()
}

// RunImport performs the import for a specific download, bypassing the event bus.
// This is the entry point used by the workflow orchestrator.
// It returns the list of imported file paths on success.
func (p *ImportPipeline) RunImport(ctx context.Context, clientID, downloadID, title, category string) ([]string, error) {
	ev := &downloads.DownloadCompletedEvent{
		DownloadID: downloadID,
		ClientID:   clientID,
		Title:      title,
		Category:   category,
	}

	p.logger.Info("orchestrator-triggered import starting",
		"download_id", downloadID, "client_id", clientID, "title", title)

	downloadPath, err := p.resolveDownloadPath(ctx, ev)
	if err != nil {
		p.recordFailure(ctx, "", "", title, "", err)
		return nil, fmt.Errorf("resolve download path: %w", err)
	}

	return p.processImport(ctx, ev, downloadPath)
}

// handleCompleted processes a download completion event.
// For orphan downloads (not tracked by a workflow), this is the only import path.
// For orchestrator-tracked downloads, state management is handled by the orchestrator;
// this path only runs the actual file import logic.
func (p *ImportPipeline) handleCompleted(ctx context.Context, ev eventbus.Event) error {
	completed, ok := ev.(*downloads.DownloadCompletedEvent)
	if !ok {
		return nil
	}

	p.logger.Info("download completed, starting import",
		"download_id", completed.DownloadID,
		"client_id", completed.ClientID,
		"title", completed.Title,
	)

	// Check if an orchestrator workflow already owns this download.
	if p.wfEngine != nil && completed.ClientID != "" && completed.DownloadID != "" {
		wf, err := p.wfEngine.FindByDownload(ctx, completed.ClientID, completed.DownloadID)
		if err != nil {
			return fmt.Errorf("check import ownership: %w", err)
		}
		if wf != nil {
			return nil // The orchestrator owns dispatch, retries, and cleanup.
		}
	}

	// For orphan downloads, create a trackable workflow.
	var wfID string
	if p.wfEngine != nil {
		wf, wfErr := p.wfEngine.StartImport(ctx, "", nil, completed.Title)
		if wfErr != nil {
			p.logger.Warn("failed to create orphan import workflow", "title", completed.Title, "error", wfErr)
		} else {
			wfID = wf.ID
		}
	}

	downloadPath, err := p.resolveDownloadPath(ctx, completed)
	if err != nil {
		p.logger.Error("failed to resolve download path", "error", err, "title", completed.Title)
		p.recordFailure(ctx, "", "", completed.Title, "", err)
		if wfID != "" {
			_ = p.wfEngine.FailImport(ctx, wfID, err.Error())
		}
		return nil // don't block the event bus
	}

	paths, err := p.processImport(ctx, completed, downloadPath)
	if err != nil {
		p.logger.Error("import failed", "error", err, "title", completed.Title, "path", downloadPath)
		if wfID != "" {
			_ = p.wfEngine.FailImport(ctx, wfID, err.Error())
		}
		return nil
	}

	if wfID != "" {
		if err := p.wfEngine.CompleteImport(ctx, wfID); err != nil {
			return err
		}
	}
	if err := p.FinalizeImports(ctx, paths); err != nil {
		p.logger.Warn("import succeeded but source cleanup failed", "error", err)
	}
	return nil
}

// resolveDownloadPath determines the filesystem path of the completed download.
func (p *ImportPipeline) resolveDownloadPath(ctx context.Context, ev *downloads.DownloadCompletedEvent) (string, error) {
	if ev.ClientID == "" {
		return "", fmt.Errorf("no client_id in completion event")
	}

	candidates := make([]string, 0, 8)
	var fallbackPaths []string
	seen := make(map[string]struct{}, 8)
	var unresolvedPath string
	addCandidate := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if isUnresolvedContentPath(path) {
			if unresolvedPath == "" {
				unresolvedPath = path
			}
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}
	resolveFirst := func() (string, bool) {
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return path, true
			}
		}
		return "", false
	}

	var statusErr error
	var liveItemTitle string
	registry := p.downloadSvc.Registry()
	client, ok := registry.Get(ev.ClientID)
	if ok {
		if items, err := registry.ClientStatus(ctx, client, ev.DownloadID); err == nil {
			for _, item := range items {
				if item.ID != ev.DownloadID {
					continue
				}
				liveItemTitle = item.Title
				// Prefer ContentPath (actual on-disk location set by the
				// download client) over SavePath+Title heuristics.
				addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, item.ContentPath))
				if item.SavePath != "" {
					addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(item.SavePath, item.Title)))
					addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(item.SavePath, ev.Title)))
					// The SavePath itself is the torrent's storage directory
					// (e.g. Rain's {download_dir}/{torrent_id}); scanning it
					// directly resolves the content regardless of the internal
					// folder/file name.
					fallbackPaths = append(fallbackPaths, p.applyRemotePathMapping(ctx, ev.ClientID, item.SavePath))
				}
			}
		} else {
			statusErr = err
		}
	}
	if path, ok := resolveFirst(); ok {
		return path, nil
	}

	// Item is no longer in the client. Try the ContentPath/SavePath cached in the event
	// (these were captured when the item completed and survive client-side removal).
	if ev.ContentPath != "" {
		addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, ev.ContentPath))
	}
	if ev.SavePath != "" {
		addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(ev.SavePath, ev.Title)))
		fallbackPaths = append(fallbackPaths, p.applyRemotePathMapping(ctx, ev.ClientID, ev.SavePath))
	}
	if path, ok := resolveFirst(); ok {
		return path, nil
	}

	// The item is no longer present in the download client (e.g. removed after
	// seeding). Try the path we cached in workflow metadata when the download
	// first completed — this survives client-side removal.
	var metadataDownloadTitle string
	if p.wfEngine != nil {
		if wf, err := p.wfEngine.FindByDownload(ctx, ev.ClientID, ev.DownloadID); err == nil && wf != nil {
			if cached := metadataString(wf.Metadata, "content_path"); cached != "" {
				addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, cached))
			}
			metadataDownloadTitle = metadataString(wf.Metadata, "download_title")
			if sp := metadataString(wf.Metadata, "save_path"); sp != "" {
				if metadataDownloadTitle != "" {
					addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(sp, metadataDownloadTitle)))
				}
				if liveItemTitle != "" {
					addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(sp, liveItemTitle)))
				}
				addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(sp, ev.Title)))
				// The cached save_path is the torrent's storage directory;
				// scan it directly as a last resort.
				fallbackPaths = append(fallbackPaths, p.applyRemotePathMapping(ctx, ev.ClientID, sp))
			}
		}
	}
	if path, ok := resolveFirst(); ok {
		return path, nil
	}
	for _, path := range fallbackPaths {
		addCandidate(path)
	}
	if path, ok := resolveFirst(); ok {
		return path, nil
	}

	// Fallback: try the client definition's default save path
	def, err := p.downloadSvc.Get(ctx, ev.ClientID)
	if err != nil {
		if statusErr != nil {
			return "", fmt.Errorf("query download status: %w", statusErr)
		}
		return "", fmt.Errorf("get client definition: %w", err)
	}
	if def.SavePathDefault != "" {
		if metadataDownloadTitle != "" {
			addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(def.SavePathDefault, metadataDownloadTitle)))
		}
		if liveItemTitle != "" {
			addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(def.SavePathDefault, liveItemTitle)))
		}
		addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, filepath.Join(def.SavePathDefault, ev.Title)))
		if path, ok := resolveFirst(); ok {
			return path, nil
		}
	}
	if unresolvedPath != "" {
		return "", fmt.Errorf("download metadata not resolved yet: unresolved content path %q", unresolvedPath)
	}
	if statusErr != nil {
		return "", fmt.Errorf("query download status: %w", statusErr)
	}

	return "", fmt.Errorf("could not determine download path for %q", ev.Title)
}

// applyRemotePathMapping translates a remote path reported by the download
// client into a local path using configured mappings. Returns the path
// unchanged if no mapping applies.
func (p *ImportPipeline) applyRemotePathMapping(ctx context.Context, clientID, path string) string {
	if p.remotePathStore == nil {
		return path
	}
	return p.remotePathStore.MapPath(ctx, clientID, path)
}

func isUnresolvedContentPath(path string) bool {
	p := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	return strings.Contains(p, "/infohash:")
}

// processImport runs the full import pipeline for a single download.
func (p *ImportPipeline) processImport(ctx context.Context, ev *downloads.DownloadCompletedEvent, downloadPath string) ([]string, error) {
	// 1. Verify the path exists
	info, err := os.Stat(downloadPath)
	if err != nil {
		return nil, p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("download path not found: %w", err))
	}
	scanPath := downloadPath
	if !info.IsDir() {
		scanPath = filepath.Dir(downloadPath)
	}

	// A single-file download must never scan unrelated siblings.
	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanImportFiles(ctx, downloadPath)
		if err != nil {
			return nil, p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("scan media files: %w", err))
		}
	} else if ext := strings.ToLower(filepath.Ext(downloadPath)); mediaExtensions[ext] || mediafiles.IsAudio(ext) {
		mediaFiles = []string{downloadPath}
	}

	if len(mediaFiles) == 0 {
		return nil, p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("no media files found in %s", downloadPath))
	}

	// 3. Post-download safety validation
	if p.postVal != nil {
		result, err := p.postVal.ValidateDownload(downloadPath)
		if err != nil {
			return nil, p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("post-validation: %w", err))
		} else if !result.Pass {
			reason := strings.Join(result.Reasons, "; ")
			p.logger.Warn("download flagged by post-validator",
				"title", ev.Title, "reasons", reason)

			if p.reviewStore != nil {
				if _, err := p.reviewStore.Create(ctx, "download", ev.DownloadID, downloadPath, reason); err != nil {
					p.logger.Error("failed to create review entry", "error", err)
				}
			}

			if err := p.recordStatus(ctx, "", "", downloadPath, "", StatusPendingReview, reason); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("import pending review: %s", reason)
		}
	}

	if strings.EqualFold(strings.TrimSpace(ev.Category), "music") {
		return p.processMusicImport(ctx, ev, downloadPath, info)
	}

	// 4. Match and import each media file
	primaryFiles := countPrimaryMediaFiles(mediaFiles)
	var failures []error
	var paths []string
	for _, mediaFile := range mediaFiles {
		dest, err := p.importDownloadFileAt(ctx, ev, mediaFile, primaryFiles, scanPath)
		if err != nil {
			if errors.Is(err, errImportSkipped) {
				continue
			}
			p.logger.Error("failed to import file", "file", mediaFile, "error", err)
			failures = append(failures, err)
			continue
		}
		paths = append(paths, dest)
	}

	if len(failures) > 0 {
		return paths, fmt.Errorf("import incomplete (%d of %d files failed): %w", len(failures), len(mediaFiles), errors.Join(failures...))
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no importable media files found in %s", downloadPath)
	}

	p.logger.Info("import completed",
		"title", ev.Title,
		"imported", len(paths),
		"total", len(mediaFiles),
	)

	// Clean up source folder if it only contains junk after import.
	// Only for download-triggered imports (non-manual).
	if ev.DownloadID != "" && info.IsDir() && p.cleaner != nil {
		if cleaned, err := p.cleaner.CleanFolder(scanPath); err != nil {
			p.logger.Warn("folder cleanup failed", "path", scanPath, "error", err)
		} else if cleaned {
			p.logger.Info("cleaned empty download folder", "path", scanPath)
		}
	}

	return paths, nil
}

var errImportSkipped = errors.New("import skipped")

// importSingleFile matches and imports a single media file that is the only
// media file of its download (or a manual import of one file).
func (p *ImportPipeline) importSingleFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, mediaFile string) (string, error) {
	return p.importDownloadFile(ctx, ev, mediaFile, 1)
}

// importDownloadFile matches and imports one media file out of a download that
// contains fileCount media files. Grab linkage identifies the item that was
// searched for, not the item each file contains: a season pack grabbed for
// one episode must still be matched file by file, so grab-based matching is
// only trusted for single-file downloads (or movies).
func (p *ImportPipeline) importDownloadFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, mediaFile string, fileCount int) (string, error) {
	root := ""
	if ev.DownloadID != "" {
		root = filepath.Dir(mediaFile)
	}
	return p.importDownloadFileAt(ctx, ev, mediaFile, fileCount, root)
}

func (p *ImportPipeline) importDownloadFileAt(ctx context.Context, ev *downloads.DownloadCompletedEvent, mediaFile string, fileCount int, root string) (string, error) {
	if mediafiles.IsAudio(strings.ToLower(filepath.Ext(mediaFile))) {
		return p.importMusicFile(ctx, ev, mediaFile)
	}
	if dest, err := p.completedImport(ctx, mediaFile); err != nil || dest != "" {
		return dest, err
	}
	var match *MatchResult
	var err error
	singleFile := fileCount <= 1
	_, _, contextualEpisode := bareEpisode(mediaFile)
	preferGrab := !contextualEpisode && singleFile && p.shouldPreferGrabMatch(ctx, ev)

	if preferGrab {
		match, err = p.matchByGrab(ctx, ev)
		if err != nil {
			p.logger.Warn("grab-based match failed", "error", err)
		}
	}

	// Try matching by full file path first (includes parent directory for context).
	// This is most reliable for multi-episode folders since it resolves each file individually.
	if match == nil || !match.Matched {
		match, err = p.matcher.MatchDownloadPath(ctx, mediaFile, root, ev.Title, ev.DownloadID)
		if err != nil {
			if contextualEpisode {
				return "", p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("match: %w", err))
			}
			p.logger.Warn("path-based match failed", "file", mediaFile, "error", err)
		}
	}

	// Fall back to grab-based matching if path matching didn't work. For
	// multi-file downloads an episode grab would map every file onto the same
	// episode, so only a movie grab is acceptable there.
	if !contextualEpisode && !preferGrab && (match == nil || !match.Matched) {
		grab, gErr := p.matchByGrab(ctx, ev)
		if gErr != nil {
			p.logger.Warn("grab-based match failed", "error", gErr)
		}
		if grab != nil && grab.Matched && (singleFile || grab.MediaType == MediaTypeMovie) {
			match = grab
		}
	}

	// Fall back to fuzzy matching by event title as last resort. A
	// multi-file download title (e.g. a season pack) cannot identify a
	// specific episode, so only accept a movie match from it.
	if !contextualEpisode && (match == nil || !match.Matched) {
		match, err = p.matcher.Match(ctx, ev.Title)
		if err != nil {
			return "", p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("match: %w", err))
		}
		if match.Matched && !singleFile && match.MediaType == MediaTypeEpisode {
			match = &MatchResult{Matched: false}
		}
	}
	if !match.Matched {
		return "", p.recordFailure(ctx, "", "", ev.Title, mediaFile,
			fmt.Errorf("%w: no match found for %q", ErrNoMatch, filepath.Base(mediaFile)))
	}
	return p.importMatched(ctx, ev, match, mediaFile, buildDestFilename(match, mediaFile))
}

// countPrimaryMediaFiles counts media files that are not obvious samples, so a
// movie with a bundled sample still counts as a single-file download.
func countPrimaryMediaFiles(files []string) int {
	n := 0
	for _, f := range files {
		if !strings.Contains(strings.ToLower(filepath.Base(f)), "sample") {
			n++
		}
	}
	return n
}

// ErrNoMatch reports that a file could not be linked to a library item.
var ErrNoMatch = errors.New("no library match")

// importMatched imports mediaFile for an already resolved match.
func (p *ImportPipeline) importMatched(ctx context.Context, ev *downloads.DownloadCompletedEvent, match *MatchResult, mediaFile, destName string) (string, error) {
	return p.importMatchedWith(ctx, ev, match, mediaFile, destName, false)
}

// importRelease fills missing quality tags only from a release identifying this
// item. Explicit file evidence wins, including a lower resolution.
func importRelease(file, title string, match *MatchResult) *parser.Release {
	incoming := parser.Parse(filepath.Base(file))
	if match == nil || !match.Matched {
		return incoming
	}
	contextTitle := reBroadcastRange.ReplaceAllString(title, "")
	contextTitle = reSeriesPack.ReplaceAllString(contextTitle, "")
	identity := parseReleaseName(contextTitle)
	contextTitle = reContextSeason.ReplaceAllString(identity.Title, " ")
	if normalise(cleanTitle(contextTitle)) != normalise(match.Title) {
		return incoming
	}
	switch match.MediaType {
	case MediaTypeMovie:
		if identity.IsSeries || !movieYearsMatch(identity.Year, match.Year) {
			return incoming
		}
	case MediaTypeEpisode:
		if (identity.Season >= 0 && identity.Season != match.Season) ||
			(identity.Episode >= 0 && identity.Episode != match.Episode) {
			return incoming
		}
	default:
		return incoming
	}
	release := parser.Parse(title)
	if incoming.Resolution == 0 {
		incoming.Resolution = release.Resolution
	}
	if incoming.Source == "" {
		incoming.Source = release.Source
		incoming.IsRemux = release.IsRemux
	}
	return incoming
}

// importMatchedWith imports mediaFile for a resolved match. userMatched
// skips release-year verification because the user chose the item.
func (p *ImportPipeline) importMatchedWith(ctx context.Context, ev *downloads.DownloadCompletedEvent, match *MatchResult, mediaFile, destName string, userMatched bool) (string, error) {
	if !userMatched {
		if err := verifyMovieImportYear(match, ev.Title, mediaFile); err != nil {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
		}
	}

	// Build destination path with proper naming
	destFile := filepath.Join(match.DestPath, destName)
	unlock := lockImportDestination(destFile)
	defer unlock()

	// Run import decision engine (spec-based pre-checks)
	srcInfo, statErr := os.Stat(mediaFile)
	var fileSize int64
	if statErr == nil {
		fileSize = srcInfo.Size()
	}
	candidate := &ImportCandidate{
		SourcePath:      mediaFile,
		DestPath:        destFile,
		FileSize:        fileSize,
		Match:           match,
		ImportMode:      p.importMode,
		IsManual:        ev.DownloadID == "",
		IncomingRelease: importRelease(mediaFile, ev.Title, match),
	}

	// Populate quality profile and existing quality for upgrade checks.
	if err := p.enrichCandidateQuality(ctx, candidate); err != nil {
		return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, fmt.Errorf("load import quality: %w", err))
	}
	incomingQuality := qualityprofiles.QualityNameFromRelease(candidate.IncomingRelease)
	destInfo, destErr := os.Lstat(destFile)
	var owned bool
	if destErr == nil && destInfo.Mode().IsRegular() {
		var quality string
		var err error
		owned, quality, err = p.matchedDestination(ctx, match, destFile)
		if err != nil {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, fmt.Errorf("check destination ownership: %w", err))
		}
		if owned {
			if quality == "" {
				quality = releaseQuality(destFile)
			}
			candidate.ExistingQuality = quality
		}
	}
	eval := p.decisions.Evaluate(ctx, candidate)
	if !eval.Approved() {
		sampleOnly := true
		reasons := make([]string, len(eval.Rejections))
		for i, r := range eval.Rejections {
			sampleOnly = sampleOnly && r.Reason == RejectionSample
			reasons[i] = r.Message
			// Log each rejection to the decision log
			_ = p.decisionLog.Log(ctx, ImportDecision{
				SourcePath: mediaFile,
				DestPath:   destFile,
				MediaType:  string(match.MediaType),
				MediaID:    match.MediaID,
				Action:     "rejected",
				Reason:     string(r.Reason) + ": " + r.Message,
				FileSize:   fileSize,
			})
		}
		if sampleOnly {
			return "", fmt.Errorf("%w: %s", errImportSkipped, strings.Join(reasons, "; "))
		}
		return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
			fmt.Errorf("import rejected: %s", strings.Join(reasons, "; ")))
	}

	// A crash can publish a complete copy before history is recorded.
	// Resume only after proving identity or checking the full content.
	registered := false
	if destErr == nil {
		if destInfo.Mode()&os.ModeSymlink != 0 || !destInfo.Mode().IsRegular() {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
				fmt.Errorf("import destination is not a regular file: %s", destFile))
		}
		srcInfo, srcErr := os.Lstat(mediaFile)
		if srcErr != nil {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
				fmt.Errorf("inspect import source: %w", srcErr))
		}
		same, err := p.sameContent(ctx, mediaFile, destFile, srcInfo, destInfo)
		if err != nil {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
		}
		if same {
			if p.verifier != nil {
				if vr := p.verifier.Verify(destFile, fileSize); !vr.OK {
					return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
						fmt.Errorf("verification failed (source preserved at %s): %s", mediaFile, vr.Reason))
				}
			}
			p.logger.Info("source already present at import destination",
				"dest", destFile, "src", mediaFile)
			var err error
			if owned {
				err = p.updateReplacementRecord(ctx, match, destFile, fileSize, incomingQuality)
			} else {
				err = p.updateLibrary(ctx, match, destFile, mediaFile, incomingQuality)
			}
			if err != nil {
				return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
			}
			registered = true
		} else if !owned {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
				fmt.Errorf("%w: %s", ErrDestinationExists, destFile))
		} else {
			if err := p.replaceMatched(ctx, match, mediaFile, destFile, destInfo, fileSize, incomingQuality); err != nil {
				return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
			}
			registered = true
		}
	} else if !os.IsNotExist(destErr) {
		return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
			fmt.Errorf("inspect import destination: %w", destErr))
	}

	// Stage moves without consuming the source until verification and registration succeed.
	mode := p.importMode
	if mode == ImportModeMove {
		mode = ImportModeHardlink
	}
	if !registered {
		if err := importFile(mediaFile, destFile, mode); err != nil {
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
		}

		// Verify imported file
		if p.verifier != nil {
			vr := p.verifier.Verify(destFile, fileSize)
			if !vr.OK {
				p.logger.Error("import verification failed",
					"dest", destFile, "reason", vr.Reason)
				if err := os.Remove(destFile); err != nil {
					p.logger.Error("failed to remove unverified destination", "dest", destFile, "error", err)
				}
				return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
					fmt.Errorf("verification failed (source preserved at %s): %s", mediaFile, vr.Reason))
			}
		}

		// Register before consuming any source media or sidecar files.
		if err := p.updateLibrary(ctx, match, destFile, mediaFile, incomingQuality); err != nil {
			p.logger.Error("library update failed after import, preserving source",
				"error", err, "dest", destFile)
			if removeErr := os.Remove(destFile); removeErr != nil {
				p.logger.Error("failed to remove staged destination", "dest", destFile, "error", removeErr)
			}
			return "", p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
				fmt.Errorf("library update failed (source preserved at %s): %w", mediaFile, err))
		}
	}

	// Import associated subtitle files (failures are non-fatal)
	if subs, err := p.subtitles.FindSubtitles(mediaFile); err != nil {
		p.logger.Warn("failed to scan for subtitles", "source", mediaFile, "error", err)
	} else {
		for _, sub := range subs {
			if err := p.subtitles.ImportSubtitle(sub, destFile, p.importMode); err != nil {
				p.logger.Warn("failed to import subtitle", "subtitle", sub.Path, "error", err)
			} else {
				p.logger.Info("imported subtitle", "subtitle", sub.Path, "language", sub.Language)
			}
		}
	}

	// Import associated extra files (failures are non-fatal)
	if extras, err := p.extras.FindExtras(mediaFile); err != nil {
		p.logger.Warn("failed to scan for extras", "source", mediaFile, "error", err)
	} else {
		for _, extra := range extras {
			if err := p.extras.ImportExtra(extra, destFile, p.importMode); err != nil {
				p.logger.Warn("failed to import extra", "extra", extra, "error", err)
			} else {
				p.logger.Info("imported extra", "extra", extra)
			}
		}
	}

	// Record success
	if err := p.recordStatus(ctx, string(match.MediaType), match.MediaID, mediaFile, destFile, StatusImported, ""); err != nil {
		return "", fmt.Errorf("record import history (source preserved at %s): %w", mediaFile, err)
	}
	if p.importMode == ImportModeMove && ev.DownloadID == "" {
		if err := p.FinalizeImports(ctx, []string{destFile}); err != nil {
			return "", fmt.Errorf("import registered but source cleanup failed: %w", err)
		}
	}

	// Update movie status to available
	if match.MediaType == "movie" && match.MediaID != "" {
		if err := p.moviesSvc.SetMovieStatus(ctx, match.MediaID, movies.MovieStatusAvailableRightQuality); err != nil {
			// Non-fatal: file is already in the library. Status will be corrected on next rescan.
			p.logger.Warn("failed to update movie status after import, will be corrected on rescan",
				"movie_id", match.MediaID, "error", err)
		}
	}

	// Publish notification
	p.publishNotification(ctx, match, destFile)

	// Publish event
	_ = p.bus.Publish(ctx, &ImportCompletedEvent{
		MediaType: match.MediaType,
		MediaID:   match.MediaID,
		Title:     match.Title,
		DestPath:  destFile,
	})

	return destFile, nil
}

// completedImport makes retries of copy/hardlink imports idempotent without
// mistaking a reused source pathname or missing destination for an import.
func (p *ImportPipeline) completedImport(ctx context.Context, source string) (string, error) {
	var id, dest string
	err := p.db.QueryRowContext(ctx, `SELECT id, dest_path FROM import_history
		WHERE source_path = ? AND status = ? ORDER BY imported_at DESC LIMIT 1`,
		source, string(StatusImported)).Scan(&id, &dest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check previous import: %w", err)
	}
	srcInfo, err := os.Lstat(source)
	if err != nil {
		return "", fmt.Errorf("inspect previous import source: %w", err)
	}
	destInfo, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect previous import destination: %w", err)
	}
	if !srcInfo.Mode().IsRegular() || !destInfo.Mode().IsRegular() ||
		srcInfo.Size() == 0 || srcInfo.Size() != destInfo.Size() {
		return "", nil
	}
	same, err := p.sameContent(ctx, source, dest, srcInfo, destInfo)
	if err != nil {
		return "", err
	}
	if !same {
		return "", nil
	}
	if _, err := p.db.ExecContext(ctx, `UPDATE import_history
		SET source_size = ?, source_mtime = ?, dest_size = ?, dest_mtime = ? WHERE id = ?`,
		srcInfo.Size(), srcInfo.ModTime().UnixNano(), destInfo.Size(), destInfo.ModTime().UnixNano(), id); err != nil {
		return "", fmt.Errorf("save verified import metadata: %w", err)
	}
	return dest, nil
}

// sameContent checks identity or full SHA-256 content. Metadata-only scans
// never call this; destructive decisions must not reuse metadata-keyed results.
func (p *ImportPipeline) sameContent(ctx context.Context, source, dest string, srcInfo, destInfo os.FileInfo) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !srcInfo.Mode().IsRegular() || !destInfo.Mode().IsRegular() || srcInfo.Size() != destInfo.Size() {
		return false, nil
	}
	if os.SameFile(srcInfo, destInfo) {
		return true, nil
	}
	digest := func(path string) ([sha256.Size]byte, error) {
		f, err := os.Open(path) //nolint:gosec // Both import paths are validated regular files before hashing.
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, err := io.Copy(h, contextReader{ctx: ctx, reader: f}); err != nil {
			return [sha256.Size]byte{}, err
		}
		var sum [sha256.Size]byte
		copy(sum[:], h.Sum(nil))
		return sum, nil
	}
	srcHash, err := digest(source)
	if err != nil {
		return false, fmt.Errorf("verify previous import source: %w", err)
	}
	destHash, err := digest(dest)
	if err != nil {
		return false, fmt.Errorf("verify previous import destination: %w", err)
	}
	same := srcHash == destHash
	return same, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

// FinalizeImports consumes move sources only after the workflow has durably
// entered cleanup. Until then, a restart can safely repeat the import.
func (p *ImportPipeline) FinalizeImports(ctx context.Context, paths []string) error {
	for _, dest := range paths {
		var source, mode string
		if err := p.db.QueryRowContext(ctx, `SELECT source_path, import_mode FROM import_history
			WHERE dest_path = ? AND status = ? ORDER BY imported_at DESC LIMIT 1`,
			dest, string(StatusImported)).Scan(&source, &mode); err != nil {
			return fmt.Errorf("find import source for cleanup: %w", err)
		}
		if mode != string(ImportModeMove) || filepath.Clean(source) == filepath.Clean(dest) {
			continue
		}
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect import source for cleanup: %w", err)
		}
		verifiedDest, err := p.completedImport(ctx, source)
		if err != nil {
			return err
		}
		if verifiedDest != dest {
			return fmt.Errorf("source or destination changed after import; retaining %s", source)
		}
		if err := os.Remove(source); err != nil {
			return fmt.Errorf("remove registered import source %q: %w", source, err)
		}
	}
	return nil
}

// shouldPreferGrabMatch returns true when the download is tracked by exactly one
// workflow item. In that case, grab-based linkage is more authoritative than
// filename parsing for season/episode resolution.
func (p *ImportPipeline) shouldPreferGrabMatch(ctx context.Context, ev *downloads.DownloadCompletedEvent) bool {
	if p.wfEngine == nil || ev == nil || ev.ClientID == "" || ev.DownloadID == "" {
		return false
	}
	wf, err := p.wfEngine.FindByDownload(ctx, ev.ClientID, ev.DownloadID)
	if err != nil || wf == nil {
		return false
	}
	items, err := p.wfEngine.Store().GetItems(ctx, wf.ID)
	if err != nil {
		return false
	}
	return len(items) == 1
}

func verifyMovieImportYear(match *MatchResult, releaseTitle, mediaFile string) error {
	if match.MediaType != MediaTypeMovie || match.Year <= 0 {
		return nil
	}
	for _, name := range []string{releaseTitle, filepath.Base(mediaFile), filepath.Base(filepath.Dir(mediaFile))} {
		year := parser.Parse(name).Year
		if !movieYearsMatch(year, match.Year) {
			return fmt.Errorf("import rejected: release year %d in %q does not match %s (%d)",
				year, name, match.Title, match.Year)
		}
	}
	return nil
}

// matchByGrab attempts to match using workflow linkage data recorded
// when the download was initiated. This is the most reliable path for
// Loom-originated downloads since it avoids fuzzy title matching.
func (p *ImportPipeline) matchByGrab(ctx context.Context, ev *downloads.DownloadCompletedEvent) (*MatchResult, error) {
	if p.wfEngine == nil || ev.ClientID == "" || ev.DownloadID == "" {
		return nil, nil
	}

	wf, err := p.wfEngine.Store().FindByDownload(ctx, ev.ClientID, ev.DownloadID)
	if err != nil {
		return nil, err
	}
	if wf == nil {
		return nil, nil
	}

	// Get linked media items from the workflow
	items, err := p.wfEngine.Store().GetItems(ctx, wf.ID)
	if err != nil {
		return nil, fmt.Errorf("get workflow items: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Episode-based match
	if items[0].MediaType == workflows.MediaTypeEpisode {
		ep, err := p.matcher.seriesSvc.GetEpisode(ctx, items[0].MediaID)
		if err != nil {
			return nil, fmt.Errorf("get episode from workflow: %w", err)
		}
		s, err := p.matcher.seriesSvc.GetSeries(ctx, ep.SeriesID)
		if err != nil {
			return nil, fmt.Errorf("get series from workflow: %w", err)
		}
		lib, err := p.matcher.libStore.Get(ctx, s.LibraryID)
		if err != nil {
			return nil, fmt.Errorf("get library from workflow: %w", err)
		}
		seasons, err := p.matcher.seriesSvc.ListSeasons(ctx, ep.SeriesID)
		if err != nil {
			return nil, fmt.Errorf("list seasons from workflow: %w", err)
		}
		seasonNum := 1
		for _, sn := range seasons {
			if sn.ID == ep.SeasonID {
				seasonNum = sn.SeasonNumber
				break
			}
		}
		destDir := filepath.Join(
			lib.Path,
			sanitizeDirName(s.Title),
			fmt.Sprintf("Season %02d", seasonNum),
		)
		p.logger.Info("matched via workflow linkage",
			"media_type", "episode", "series", s.Title,
			"season", seasonNum, "episode", ep.EpisodeNumber)
		return &MatchResult{
			Matched:   true,
			MediaType: MediaTypeEpisode,
			MediaID:   ep.ID,
			SeriesID:  ep.SeriesID,
			Title:     s.Title,
			Year:      s.Year,
			Season:    seasonNum,
			Episode:   ep.EpisodeNumber,
			DestPath:  destDir,
		}, nil
	}

	// Movie-based match
	if items[0].MediaType == workflows.MediaTypeMovie {
		mv, err := p.matcher.moviesSvc.GetMovie(ctx, items[0].MediaID)
		if err != nil {
			return nil, fmt.Errorf("get movie from workflow: %w", err)
		}
		if p.matcher.libStore == nil {
			return nil, fmt.Errorf("get library from workflow: library store not configured")
		}
		libraryID := strings.TrimSpace(mv.LibraryID)
		if libraryID == "" {
			libs, listErr := p.matcher.libStore.List(ctx)
			if listErr != nil {
				return nil, fmt.Errorf("get library from workflow: movie %q has no library assigned and listing libraries failed: %w", mv.Title, listErr)
			}
			movieLibs := make([]libraries.Library, 0, len(libs))
			for _, lib := range libs {
				if strings.EqualFold(lib.MediaType, "movie") {
					movieLibs = append(movieLibs, lib)
				}
			}
			switch len(movieLibs) {
			case 0:
				return nil, fmt.Errorf("get library from workflow: movie %q has no library assigned and no movie library is configured", mv.Title)
			case 1:
				libraryID = movieLibs[0].ID
				mv.LibraryID = libraryID
				if updateErr := p.matcher.moviesSvc.UpdateMovie(ctx, mv); updateErr != nil {
					p.logger.Warn("failed to persist inferred library for movie",
						"movie_id", mv.ID, "library_id", libraryID, "error", updateErr)
				}
				p.logger.Info("inferred missing movie library from single configured movie library",
					"movie_id", mv.ID, "library_id", libraryID)
			default:
				return nil, fmt.Errorf("get library from workflow: movie %q has no library assigned and multiple movie libraries are configured", mv.Title)
			}
		}
		lib, err := p.matcher.libStore.Get(ctx, libraryID)
		if err != nil {
			return nil, fmt.Errorf("get library from workflow: %w", err)
		}
		destDir := filepath.Join(lib.Path, sanitizeDirName(fmt.Sprintf("%s (%d)", mv.Title, mv.Year)))
		p.logger.Info("matched via workflow linkage",
			"media_type", "movie", "title", mv.Title)
		return &MatchResult{
			Matched:   true,
			MediaType: MediaTypeMovie,
			MediaID:   mv.ID,
			Title:     mv.Title,
			Year:      mv.Year,
			DestPath:  destDir,
		}, nil
	}

	return nil, nil
}

// buildDestFilename generates a clean library filename from the match result.
// Movies: "Movie Title (2024).ext"
// Episodes: "Series Title - S01E02.ext"
func buildDestFilename(match *MatchResult, sourceFile string) string {
	ext := filepath.Ext(sourceFile)

	switch match.MediaType {
	case MediaTypeMovie:
		name := sanitizeDirName(match.Title)
		if match.Year > 0 {
			return fmt.Sprintf("%s (%d)%s", name, match.Year, ext)
		}
		return name + ext

	case MediaTypeEpisode:
		name := sanitizeDirName(match.Title)
		return fmt.Sprintf("%s - S%02dE%02d%s", name, match.Season, match.Episode, ext)

	default:
		return filepath.Base(sourceFile)
	}
}

// updateLibrary adds a file record to the appropriate service.
func (p *ImportPipeline) updateLibrary(ctx context.Context, match *MatchResult, destFile, srcFile, quality string) error {
	info, err := os.Stat(destFile)
	if err != nil {
		// File was just imported; stat the source as fallback
		info, err = os.Stat(srcFile)
		if err != nil {
			return fmt.Errorf("stat imported file: %w", err)
		}
	}

	switch match.MediaType {
	case MediaTypeMovie:
		mf := &movies.MovieFile{
			ID:        uuid.New().String(),
			MovieID:   match.MediaID,
			FilePath:  destFile,
			Size:      info.Size(),
			Quality:   quality,
			DateAdded: time.Now(),
		}
		if err := p.matcher.moviesSvc.AddMovieFile(ctx, mf); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
				return p.updateReplacementRecord(ctx, match, destFile, info.Size(), mf.Quality)
			}
			return err
		}
		return nil

	case MediaTypeEpisode:
		var count int
		if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files WHERE episode_id = ? AND file_path = ?`, match.MediaID, destFile).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return p.updateReplacementRecord(ctx, match, destFile, info.Size(), quality)
		}
		ef := &series.EpisodeFile{
			ID:        uuid.New().String(),
			EpisodeID: match.MediaID,
			SeriesID:  match.SeriesID,
			FilePath:  destFile,
			FileSize:  info.Size(),
			Quality:   quality,
		}
		if err := p.matcher.seriesSvc.CreateEpisodeFile(ctx, ef); err != nil {
			return err
		}
		return nil

	default:
		return fmt.Errorf("unknown media type: %s", match.MediaType)
	}
}

// publishNotification sends a notification about a completed import.
func (p *ImportPipeline) publishNotification(ctx context.Context, match *MatchResult, destFile string) {
	if p.notifSvc == nil {
		return
	}
	title := fmt.Sprintf("Imported: %s", match.Title)
	msg := fmt.Sprintf("File imported to %s", destFile)
	if err := p.notifSvc.Send(ctx, notifications.EventOnDownload, title, msg, map[string]any{
		"media_type": string(match.MediaType),
		"media_id":   match.MediaID,
		"dest_path":  destFile,
	}); err != nil {
		p.logger.Warn("notification send failed", "error", err)
	}
}

// recordFailure records a failed import in history and publishes a failure
// event. The notification dispatcher turns that event into the user alert, so
// no direct notification is sent here.
func (p *ImportPipeline) recordFailure(ctx context.Context, mediaType, mediaID, title, sourcePath string, importErr error) error {
	errMsg := ""
	if importErr != nil {
		errMsg = importErr.Error()
	}
	if err := p.recordStatus(ctx, mediaType, mediaID, sourcePath, "", StatusFailed, errMsg); err != nil {
		p.logger.Error("failed to record import failure", "error", err)
	}
	_ = p.bus.Publish(ctx, &ImportFailedEvent{
		Title:      title,
		SourcePath: sourcePath,
		Error:      errMsg,
	})

	return importErr
}

// recordStatus stores metadata for inexpensive historical scan hints.
// Content verification remains separate from these size/mtime snapshots.
func (p *ImportPipeline) recordStatus(ctx context.Context, mediaType, mediaID, sourcePath, destPath string, status ImportStatus, errMsg string) error {
	var size, mtime int64
	if sourcePath != "" {
		if info, err := os.Lstat(sourcePath); err == nil && info.Mode().IsRegular() {
			size, mtime = info.Size(), info.ModTime().UnixNano()
		}
	}
	var destSize, destMtime int64
	if status == StatusImported && destPath != "" {
		info, err := os.Lstat(destPath)
		if err != nil {
			return fmt.Errorf("record import destination metadata: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("record import destination metadata: not a regular file: %s", destPath)
		}
		destSize, destMtime = info.Size(), info.ModTime().UnixNano()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO import_history (id, media_type, media_id, source_path, dest_path, import_mode, status, error, imported_at, source_size, source_mtime, dest_size, dest_mtime)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(),
		mediaType,
		mediaID,
		sourcePath,
		destPath,
		string(p.importMode),
		string(status),
		errMsg,
		time.Now().UTC(),
		size,
		mtime,
		destSize,
		destMtime,
	)
	return err
}

// SubmitManualImport queues a manual import of a file or folder. Every
// media file is tracked and retried independently; the request is never
// rejected for capacity.
func (p *ImportPipeline) SubmitManualImport(path string) error {
	_, err := p.EnqueueImport(context.Background(), JobRequest{Kind: JobKindManual, Path: path})
	return err
}

// SubmitManualMatch queues an import linked to an explicit media item.
func (p *ImportPipeline) SubmitManualMatch(path string, mediaType MediaType, mediaID string) error {
	if mediaType == MediaTypeMusic {
		if err := checkRegularSource(path); err != nil {
			return err
		}
		if !mediafiles.IsAudio(strings.ToLower(filepath.Ext(path))) {
			return fmt.Errorf("not an audio file: %s", path)
		}
	}
	_, err := p.EnqueueImport(context.Background(), JobRequest{
		Kind: JobKindManualMatch, Path: path, MediaType: mediaType, MediaID: mediaID,
	})
	return err
}

// SubmitReimport queues a forced reimport of one source file.
func (p *ImportPipeline) SubmitReimport(mediaType MediaType, mediaID, sourcePath string, opts ReimportOptions) error {
	_, err := p.EnqueueImport(context.Background(), JobRequest{
		Kind: JobKindReimport, Path: sourcePath, MediaType: mediaType, MediaID: mediaID,
		ConflictPolicy: opts.ConflictPolicy,
	})
	return err
}

// ImportManual triggers an import for an arbitrary filesystem path.
func (p *ImportPipeline) ImportManual(ctx context.Context, path string) error {
	mediaFiles, err := scanImportFiles(ctx, path)
	if err != nil {
		return err
	}

	if len(mediaFiles) == 0 {
		return fmt.Errorf("no media files found in %s", path)
	}

	fakeEvent := &downloads.DownloadCompletedEvent{
		Title:       filepath.Base(path),
		CompletedAt: time.Now(),
	}

	primary := countPrimaryMediaFiles(mediaFiles)
	var errs []error
	imported := 0
	for _, mf := range mediaFiles {
		if _, err := p.importDownloadFile(ctx, fakeEvent, mf, primary); err != nil {
			if errors.Is(err, errImportSkipped) {
				continue
			}
			errs = append(errs, err)
			continue
		}
		imported++
	}
	if len(errs) > 0 {
		return fmt.Errorf("import incomplete (%d of %d files failed): %w", len(errs), len(mediaFiles), errors.Join(errs...))
	}
	if imported == 0 {
		return fmt.Errorf("no importable media files found in %s", path)
	}
	return nil
}

// ImportManualMatch imports a file or folder and directly links it to a
// specific media item, bypassing fuzzy matching. The user explicitly
// provides the media_type and media_id, so no guessing is needed.
func (p *ImportPipeline) ImportManualMatch(ctx context.Context, path string, mediaType MediaType, mediaID string) (*ImportRecord, error) {
	if mediaType == MediaTypeMusic {
		return p.importMusicMatch(ctx, path, mediaID)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing symlink import path: %s", path)
	}

	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanMediaFiles(path)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := filepath.Ext(path)
		if !mediaExtensions[strings.ToLower(ext)] {
			return nil, fmt.Errorf("not a media file: %s", path)
		}
		mediaFiles = []string{path}
	}
	if len(mediaFiles) == 0 {
		return nil, fmt.Errorf("no media files found in %s", path)
	}

	// Resolve destination using exact match (no fuzzy matching)
	match, err := p.matcher.MatchExact(ctx, mediaType, mediaID)
	if err != nil {
		return nil, fmt.Errorf("resolve media: %w", err)
	}
	if !info.IsDir() {
		dest, err := p.importMatchedWith(ctx, &downloads.DownloadCompletedEvent{Title: filepath.Base(path)}, match, path, filepath.Base(path), true)
		if err != nil {
			return nil, err
		}
		return &ImportRecord{ID: uuid.New().String(), MediaType: match.MediaType, MediaID: match.MediaID,
			SourcePath: path, DestPath: dest, ImportMode: p.importMode, Status: StatusImported, ImportedAt: time.Now().UTC()}, nil
	}
	moveFolder := p.importMode == ImportModeMove && filepath.Clean(path) != filepath.Clean(match.DestPath)
	if moveFolder {
		sourceAbs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		destAbs, err := filepath.Abs(match.DestPath)
		if err != nil {
			return nil, err
		}
		for _, pair := range [][2]string{{sourceAbs, destAbs}, {destAbs, sourceAbs}} {
			rel, err := filepath.Rel(pair[0], pair[1])
			if err != nil {
				return nil, err
			}
			if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
				return nil, fmt.Errorf("source and destination folders must not contain one another")
			}
		}
		if err := validateImportTree(path); err != nil {
			return nil, err
		}
	}

	var lastRecord *ImportRecord
	var importErrors []error
	for _, mf := range mediaFiles {
		rel, err := filepath.Rel(path, mf)
		if err != nil {
			importErrors = append(importErrors, err)
			continue
		}
		destFile, err := p.importMatchedWith(ctx, &downloads.DownloadCompletedEvent{Title: filepath.Base(mf)}, match, mf, rel, true)
		if err != nil {
			importErrors = append(importErrors, err)
			continue
		}

		lastRecord = &ImportRecord{
			ID:         uuid.New().String(),
			MediaType:  match.MediaType,
			MediaID:    match.MediaID,
			SourcePath: mf,
			DestPath:   destFile,
			ImportMode: p.importMode,
			Status:     StatusImported,
			ImportedAt: time.Now().UTC(),
		}
	}

	if len(importErrors) > 0 {
		return lastRecord, errors.Join(importErrors...)
	}
	if moveFolder {
		if err := moveRemainingContents(path, match.DestPath); err != nil {
			return lastRecord, fmt.Errorf("move remaining folder contents: %w", err)
		}
	}
	if lastRecord == nil {
		return nil, fmt.Errorf("all files failed to import from %s", path)
	}
	return lastRecord, nil
}

// ListHistory returns import history records.
func (p *ImportPipeline) ListHistory(ctx context.Context, limit, offset int) ([]*ImportRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, media_type, media_id, source_path, dest_path, import_mode, status, error, imported_at
		 FROM import_history
		 ORDER BY imported_at DESC
		 LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query import history: %w", err)
	}
	defer rows.Close()

	var records []*ImportRecord
	for rows.Next() {
		var r ImportRecord
		if err := rows.Scan(
			&r.ID, &r.MediaType, &r.MediaID, &r.SourcePath, &r.DestPath,
			&r.ImportMode, &r.Status, &r.Error, &r.ImportedAt,
		); err != nil {
			return nil, fmt.Errorf("scan import record: %w", err)
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

// ImportPreview describes what would happen if an import were triggered.
type ImportPreview struct {
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
	MediaType string `json:"media_type,omitempty"`
	MediaID   string `json:"media_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Quality   string `json:"quality,omitempty"`
}

// PreviewImport is a dry-run mode that returns what would happen without
// actually importing. POST /api/v1/imports/preview uses this.
func (p *ImportPipeline) PreviewImport(ctx context.Context, path string) ([]ImportPreview, error) {
	mediaFiles, err := scanImportFiles(ctx, path)
	if err != nil {
		return nil, err
	}

	var previews []ImportPreview
	for _, mf := range mediaFiles {
		preview := p.previewSingleFile(ctx, mf)
		previews = append(previews, preview)
	}
	return previews, nil
}

func (p *ImportPipeline) previewSingleFile(ctx context.Context, filePath string) ImportPreview {
	if mediafiles.IsAudio(strings.ToLower(filepath.Ext(filePath))) {
		scan := p.scanMusicFile(ctx, filePath)
		reason := "no unique album and track match; manual matching is required"
		if scan.MatchedMediaID != "" {
			reason = "matched using audio tags and album track metadata"
		}
		return ImportPreview{
			FilePath: scan.FilePath, FileSize: scan.FileSize, MediaType: scan.MediaType,
			MediaID: scan.MatchedMediaID, Title: scan.MatchedMedia, Quality: scan.Quality,
			Action: scan.SuggestedAction, Reason: reason,
		}
	}
	info, _ := os.Stat(filePath)
	var fileSize int64
	if info != nil {
		fileSize = info.Size()
	}

	quality := ParseFileQuality(filePath)
	qualityStr := ""
	if quality.Resolution > 0 {
		qualityStr = fmt.Sprintf("%dp", quality.Resolution)
	}

	preview := ImportPreview{
		FilePath: filePath,
		FileSize: fileSize,
		Quality:  qualityStr,
		Action:   "skip",
		Reason:   "no match found",
	}

	match, err := p.matcher.MatchPath(ctx, filePath)
	if err != nil {
		preview.Action = "error"
		preview.Reason = err.Error()
		return preview
	}

	if !match.Matched {
		return preview
	}

	preview.MediaType = string(match.MediaType)
	preview.MediaID = match.MediaID
	preview.Title = match.Title

	destFile := filepath.Join(match.DestPath, buildDestFilename(match, filePath))
	candidate := &ImportCandidate{SourcePath: filePath, DestPath: destFile, FileSize: fileSize,
		Match: match, ImportMode: p.importMode, IncomingRelease: parser.Parse(filepath.Base(filePath))}
	if err := p.enrichCandidateQuality(ctx, candidate); err != nil {
		preview.Action, preview.Reason = "error", err.Error()
		return preview
	}
	if destInfo, err := os.Lstat(destFile); err == nil {
		if !destInfo.Mode().IsRegular() {
			preview.Action, preview.Reason = "error", "destination is not a regular file"
			return preview
		}
		owned, existingQuality, err := p.matchedDestination(ctx, match, destFile)
		if err != nil {
			preview.Action, preview.Reason = "error", err.Error()
			return preview
		}
		if !owned {
			preview.Action, preview.Reason = "skip", "destination is not registered to this item"
			return preview
		}
		if existingQuality == "" {
			existingQuality = releaseQuality(destFile)
		}
		candidate.ExistingQuality = existingQuality
		preview.Action, preview.Reason = "upgrade", "replace existing matched file if quality is accepted"
	} else if !os.IsNotExist(err) {
		preview.Action, preview.Reason = "error", err.Error()
		return preview
	} else {
		preview.Action, preview.Reason = "import", "no existing file at destination"
	}
	if evaluation := p.decisions.Evaluate(ctx, candidate); !evaluation.Approved() {
		preview.Action, preview.Reason = "skip", evaluation.Rejections[0].Message
	}

	return preview
}

// enrichCandidateQuality populates QualityProfileID and ExistingQuality
// on the candidate so the UpgradeSpec can make informed decisions.
func (p *ImportPipeline) enrichCandidateQuality(ctx context.Context, c *ImportCandidate) error {
	if c.Match == nil || !c.Match.Matched {
		return nil
	}

	switch c.Match.MediaType {
	case MediaTypeMovie:
		movie, err := p.moviesSvc.GetMovie(ctx, c.Match.MediaID)
		if err != nil {
			return err
		}
		if movie == nil {
			return fmt.Errorf("movie %q not found", c.Match.MediaID)
		}
		c.QualityProfileID = movie.QualityProfileID

		files, err := p.moviesSvc.ListMovieFiles(ctx, movie.ID)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return nil
		}
		// Use the quality of the first (usually only) existing file.
		c.ExistingQuality = files[0].Quality
		if c.ExistingQuality == "" {
			c.ExistingQuality = releaseQuality(files[0].FilePath)
		}

	case MediaTypeEpisode:
		ep, err := p.matcher.seriesSvc.GetEpisode(ctx, c.Match.MediaID)
		if err != nil {
			return err
		}
		if ep == nil {
			return fmt.Errorf("episode %q not found", c.Match.MediaID)
		}
		show, err := p.matcher.seriesSvc.GetSeries(ctx, ep.SeriesID)
		if err != nil {
			return err
		}
		if show == nil {
			return fmt.Errorf("series %q not found", ep.SeriesID)
		}
		c.QualityProfileID = show.QualityProfileID
		var quality, path string
		err = p.db.QueryRowContext(ctx, `SELECT COALESCE(quality, ''), file_path FROM episode_files WHERE episode_id = ? ORDER BY created_at DESC LIMIT 1`, ep.ID).Scan(&quality, &path)
		if err == nil {
			c.ExistingQuality = quality
			if c.ExistingQuality == "" {
				c.ExistingQuality = releaseQuality(path)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// metadataString extracts a string field from a JSON metadata blob.
// Returns "" if the blob is empty, not valid JSON, or the field is absent.
func metadataString(metadata, key string) string {
	if metadata == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
