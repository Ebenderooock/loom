package imports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/parser"
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
	importSem       chan struct{} // bounds concurrent manual imports
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
		NewAlreadyImportedSpec(opts.DB),
	}
	if opts.QualityProfiles != nil {
		specs = append(specs, NewUpgradeSpec(opts.QualityProfiles))
	}

	return &ImportPipeline{
		db:              opts.DB,
		bus:             opts.Bus,
		downloadSvc:     opts.DownloadSvc,
		remotePathStore: opts.RemotePathStore,
		matcher:         matcher,
		wfEngine:        opts.WorkflowEngine,
		moviesSvc:       opts.MoviesSvc,
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
		importSem:       make(chan struct{}, 2), // max 2 concurrent manual imports
	}, nil
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

	if err := p.processImport(ctx, ev, downloadPath); err != nil {
		return nil, err
	}

	return []string{downloadPath}, nil
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
	var isOrchestrated bool
	if p.wfEngine != nil && completed.ClientID != "" && completed.DownloadID != "" {
		wf, err := p.wfEngine.FindByDownload(ctx, completed.ClientID, completed.DownloadID)
		if err != nil {
			p.logger.Warn("failed to check workflow for download", "error", err)
		}
		if wf != nil {
			isOrchestrated = true
		}
	}

	// For orphan downloads, create a trackable workflow.
	var wfID string
	if !isOrchestrated && p.wfEngine != nil {
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

	if err := p.processImport(ctx, completed, downloadPath); err != nil {
		p.logger.Error("import failed", "error", err, "title", completed.Title, "path", downloadPath)
		if wfID != "" {
			_ = p.wfEngine.FailImport(ctx, wfID, err.Error())
		}
		return nil
	}

	if wfID != "" {
		_ = p.wfEngine.CompleteImport(ctx, wfID)
	}
	return nil
}

// resolveDownloadPath determines the filesystem path of the completed download.
func (p *ImportPipeline) resolveDownloadPath(ctx context.Context, ev *downloads.DownloadCompletedEvent) (string, error) {
	if ev.ClientID == "" {
		return "", fmt.Errorf("no client_id in completion event")
	}

	candidates := make([]string, 0, 8)
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
	client, ok := p.downloadSvc.Registry().Get(ev.ClientID)
	if ok {
		if items, err := client.Status(ctx, ev.DownloadID); err == nil {
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
					addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, item.SavePath))
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
				addCandidate(p.applyRemotePathMapping(ctx, ev.ClientID, sp))
			}
		}
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
func (p *ImportPipeline) processImport(ctx context.Context, ev *downloads.DownloadCompletedEvent, downloadPath string) error {
	// 1. Verify the path exists
	info, err := os.Stat(downloadPath)
	if err != nil {
		return p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("download path not found: %w", err))
	}

	scanPath := downloadPath
	if !info.IsDir() {
		scanPath = filepath.Dir(downloadPath)
	}

	// 2. Scan for media files
	mediaFiles, err := scanMediaFiles(scanPath)
	if err != nil {
		return p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("scan media files: %w", err))
	}

	// If the download path is a file itself and it's a media file, include it
	if !info.IsDir() {
		ext := filepath.Ext(downloadPath)
		if mediaExtensions[ext] {
			mediaFiles = []string{downloadPath}
		}
	}

	if len(mediaFiles) == 0 {
		return p.recordFailure(ctx, "", "", ev.Title, downloadPath, fmt.Errorf("no media files found in %s", scanPath))
	}

	// 3. Post-download safety validation
	if p.postVal != nil {
		result, err := p.postVal.ValidateDownload(scanPath)
		if err != nil {
			p.logger.Warn("post-validation error, continuing", "error", err)
		} else if !result.Pass {
			reason := strings.Join(result.Reasons, "; ")
			p.logger.Warn("download flagged by post-validator",
				"title", ev.Title, "reasons", reason)

			if p.reviewStore != nil {
				if _, err := p.reviewStore.Create(ctx, "download", ev.DownloadID, downloadPath, reason); err != nil {
					p.logger.Error("failed to create review entry", "error", err)
				}
			}

			return p.recordStatus(ctx, "", "", downloadPath, "", StatusPendingReview, reason)
		}
	}

	// 4. Match and import each media file
	var lastErr error
	imported := 0
	for _, mediaFile := range mediaFiles {
		if err := p.importSingleFile(ctx, ev, mediaFile); err != nil {
			p.logger.Error("failed to import file", "file", mediaFile, "error", err)
			lastErr = err
			continue
		}
		imported++
	}

	if imported == 0 && lastErr != nil {
		return lastErr
	}

	p.logger.Info("import completed",
		"title", ev.Title,
		"imported", imported,
		"total", len(mediaFiles),
	)

	// Clean up source folder if it only contains junk after import.
	// Only for download-triggered imports (non-manual).
	if imported > 0 && ev.DownloadID != "" && info.IsDir() && p.cleaner != nil {
		if cleaned, err := p.cleaner.CleanFolder(scanPath); err != nil {
			p.logger.Warn("folder cleanup failed", "path", scanPath, "error", err)
		} else if cleaned {
			p.logger.Info("cleaned empty download folder", "path", scanPath)
		}
	}

	return nil
}

// importSingleFile matches and imports a single media file.
func (p *ImportPipeline) importSingleFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, mediaFile string) error {
	// For multi-episode downloads, grab-based matching is too coarse (all files would match
	// the same episode). Try file-path-based matching first for each individual file.
	var match *MatchResult
	var err error

	// Try matching by full file path first (includes parent directory for context).
	// This is most reliable for multi-episode folders since it resolves each file individually.
	match, err = p.matcher.MatchPath(ctx, mediaFile)
	if err != nil {
		p.logger.Warn("path-based match failed", "file", mediaFile, "error", err)
	}

	// Fall back to grab-based matching if path matching didn't work.
	// This is reliable for Loom-originated single-file downloads.
	if match == nil || !match.Matched {
		match, err = p.matchByGrab(ctx, ev)
		if err != nil {
			p.logger.Warn("grab-based match failed", "error", err)
		}
	}

	// Fall back to fuzzy matching by event title as last resort.
	if match == nil || !match.Matched {
		match, err = p.matcher.Match(ctx, ev.Title)
		if err != nil {
			return p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("match: %w", err))
		}
	}
	if !match.Matched {
		return p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("no match found for %q", ev.Title))
	}

	// Build destination path with proper naming
	destFile := filepath.Join(match.DestPath, buildDestFilename(match, mediaFile))

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
		IncomingRelease: parser.Parse(filepath.Base(mediaFile)),
	}

	// Populate quality profile and existing quality for upgrade checks.
	p.enrichCandidateQuality(ctx, candidate)
	eval := p.decisions.Evaluate(ctx, candidate)
	if !eval.Approved() {
		reasons := make([]string, len(eval.Rejections))
		for i, r := range eval.Rejections {
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
		return p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
			fmt.Errorf("import rejected: %s", strings.Join(reasons, "; ")))
	}

	// Collision handling: if the destination already exists, check if it's the same file
	if destInfo, err := os.Stat(destFile); err == nil {
		srcInfo, srcErr := os.Stat(mediaFile)

		// If source is gone (already moved on a prior attempt) OR sizes match, treat as already imported.
		alreadyMoved := srcErr != nil
		sameSizeMatch := srcErr == nil && destInfo.Size() == srcInfo.Size()
		if alreadyMoved || sameSizeMatch {
			if alreadyMoved {
				p.logger.Info("source file gone and destination exists, treating prior import as successful",
					"dest", destFile, "src", mediaFile)
			} else {
				p.logger.Info("destination file already exists with same size, treating as imported",
					"dest", destFile, "src", mediaFile)
			}
			if err := p.updateLibrary(ctx, match, destFile, mediaFile); err != nil {
				p.logger.Warn("library update for existing file failed", "error", err)
			}
			if err := p.recordStatus(ctx, string(match.MediaType), match.MediaID, mediaFile, destFile, StatusImported, "already exists"); err != nil {
				p.logger.Error("failed to record import history", "error", err)
			}
			if match.MediaType == "movie" && match.MediaID != "" {
				if err := p.moviesSvc.SetMovieStatus(ctx, match.MediaID, movies.MovieStatusAvailableRightQuality); err != nil {
					p.logger.Warn("failed to update movie status after import (existing file), will be corrected on rescan",
						"movie_id", match.MediaID, "error", err)
				}
			}
			p.publishNotification(ctx, match, destFile)
			_ = p.bus.Publish(ctx, &ImportCompletedEvent{
				MediaType: match.MediaType,
				MediaID:   match.MediaID,
				Title:     match.Title,
				DestPath:  destFile,
			})
			return nil
		}

		// Destination exists with different size — upgrade scenario.
		// Recycle the old file instead of overwriting it.
		if p.recycler != nil {
			libraryRoot := filepath.Dir(match.DestPath)
			if err := p.recycler.Recycle(destFile, libraryRoot); err != nil {
				p.logger.Warn("failed to recycle existing file, will overwrite",
					"dest", destFile, "error", err)
			}
		}
	}

	// Import the file
	if err := importFile(mediaFile, destFile, p.importMode); err != nil {
		return p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
	}

	// Verify imported file
	if p.verifier != nil {
		vr := p.verifier.Verify(destFile, fileSize)
		if !vr.OK {
			p.logger.Error("import verification failed",
				"dest", destFile, "reason", vr.Reason)
			_ = os.Remove(destFile)
			return p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile,
				fmt.Errorf("verification failed: %s", vr.Reason))
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

	// Update database
	if err := p.updateLibrary(ctx, match, destFile, mediaFile); err != nil {
		p.logger.Error("library update failed after import, cleaning up",
			"error", err, "dest", destFile)
		// Try to clean up the imported file
		_ = os.Remove(destFile)
		return p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
	}

	// Record success
	if err := p.recordStatus(ctx, string(match.MediaType), match.MediaID, mediaFile, destFile, StatusImported, ""); err != nil {
		p.logger.Error("failed to record import history", "error", err)
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
		lib, err := p.matcher.libStore.Get(ctx, mv.LibraryID)
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
func (p *ImportPipeline) updateLibrary(ctx context.Context, match *MatchResult, destFile, srcFile string) error {
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
			DateAdded: time.Now(),
		}
		if err := p.matcher.moviesSvc.AddMovieFile(ctx, mf); err != nil {
			// If the file record already exists (e.g. rescan or retry), treat as success
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
				p.logger.Debug("movie file record already exists", "path", destFile)
				return nil
			}
			return err
		}
		return nil

	case MediaTypeEpisode:
		ef := &series.EpisodeFile{
			ID:        uuid.New().String(),
			EpisodeID: match.MediaID,
			SeriesID:  match.SeriesID,
			FilePath:  destFile,
			FileSize:  info.Size(),
		}
		if err := p.matcher.seriesSvc.CreateEpisodeFile(ctx, ef); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
				p.logger.Debug("episode file record already exists", "path", destFile)
				return nil
			}
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

// recordFailure records a failed import in history and publishes a failure event.
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

	if p.notifSvc != nil {
		msg := fmt.Sprintf("Import failed for %s: %s", title, errMsg)
		_ = p.notifSvc.Send(ctx, notifications.EventOnDownload, "Import Failed", msg, map[string]any{
			"title": title,
			"error": errMsg,
		})
	}

	return importErr
}

// recordStatus inserts a row into import_history.
func (p *ImportPipeline) recordStatus(ctx context.Context, mediaType, mediaID, sourcePath, destPath string, status ImportStatus, errMsg string) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO import_history (id, media_type, media_id, source_path, dest_path, import_mode, status, error, imported_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(),
		mediaType,
		mediaID,
		sourcePath,
		destPath,
		string(p.importMode),
		string(status),
		errMsg,
		time.Now().UTC(),
	)
	return err
}

// SubmitManualImport validates the path and runs the import in a background
// goroutine with bounded concurrency. Returns an error only if validation
// fails or the queue is full. Import results are delivered via
// notifications/events, not via the return value.
func (p *ImportPipeline) SubmitManualImport(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanMediaFiles(path)
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := filepath.Ext(path)
		if !mediaExtensions[ext] {
			return fmt.Errorf("not a media file: %s", path)
		}
		mediaFiles = []string{path}
	}
	if len(mediaFiles) == 0 {
		return fmt.Errorf("no media files found in %s", path)
	}

	// Try to acquire a slot; reject if full.
	select {
	case p.importSem <- struct{}{}:
	default:
		return fmt.Errorf("import queue full, try again later")
	}

	// Create a workflow so the import is trackable in the UI.
	var wfID string
	if p.wfEngine != nil {
		grabTitle := filepath.Base(path)
		wf, wfErr := p.wfEngine.StartImport(context.Background(), "", nil, grabTitle)
		if wfErr != nil {
			p.logger.Warn("failed to create import workflow", "path", path, "error", wfErr)
		} else {
			wfID = wf.ID
		}
	}

	go func() {
		defer func() { <-p.importSem }()
		ctx := context.Background()
		p.logger.Info("manual import starting (async)", "path", path, "files", len(mediaFiles))
		if err := p.ImportManual(ctx, path); err != nil {
			p.logger.Error("manual import failed", "path", path, "error", err)
			if wfID != "" {
				if fErr := p.wfEngine.FailImport(ctx, wfID, err.Error()); fErr != nil {
					p.logger.Error("failed to mark import workflow failed", "id", wfID, "error", fErr)
				}
			}
			return
		}
		if wfID != "" {
			if cErr := p.wfEngine.CompleteImport(ctx, wfID); cErr != nil {
				p.logger.Error("failed to mark import workflow completed", "id", wfID, "error", cErr)
			}
		}
	}()
	return nil
}

// SubmitManualMatch validates the request and runs the exact-match import
// in a background goroutine with bounded concurrency.
func (p *ImportPipeline) SubmitManualMatch(path string, mediaType MediaType, mediaID string) error {
	// Validate path exists before queuing
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	select {
	case p.importSem <- struct{}{}:
	default:
		return fmt.Errorf("import queue full, try again later")
	}

	// Create a workflow so the import is trackable in the UI.
	var wfID string
	if p.wfEngine != nil {
		grabTitle := filepath.Base(path)
		wf, wfErr := p.wfEngine.StartImport(context.Background(), string(mediaType), []string{mediaID}, grabTitle)
		if wfErr != nil {
			p.logger.Warn("failed to create import workflow", "path", path, "error", wfErr)
		} else {
			wfID = wf.ID
		}
	}

	go func() {
		defer func() { <-p.importSem }()
		ctx := context.Background()
		p.logger.Info("manual match import starting (async)",
			"path", path, "media_type", mediaType, "media_id", mediaID)
		if _, err := p.ImportManualMatch(ctx, path, mediaType, mediaID); err != nil {
			p.logger.Error("manual match import failed",
				"path", path, "media_type", mediaType, "media_id", mediaID, "error", err)
			if wfID != "" {
				if fErr := p.wfEngine.FailImport(ctx, wfID, err.Error()); fErr != nil {
					p.logger.Error("failed to mark import workflow failed", "id", wfID, "error", fErr)
				}
			}
			return
		}
		if wfID != "" {
			if cErr := p.wfEngine.CompleteImport(ctx, wfID); cErr != nil {
				p.logger.Error("failed to mark import workflow completed", "id", wfID, "error", cErr)
			}
		}
	}()
	return nil
}

// SubmitReimport validates the request and runs the reimport in a background
// goroutine with bounded concurrency.
func (p *ImportPipeline) SubmitReimport(mediaType MediaType, mediaID, sourcePath string, opts ReimportOptions) error {
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	select {
	case p.importSem <- struct{}{}:
	default:
		return fmt.Errorf("import queue full, try again later")
	}

	// Create a workflow so the reimport is trackable in the UI.
	var wfID string
	if p.wfEngine != nil {
		grabTitle := filepath.Base(sourcePath)
		wf, wfErr := p.wfEngine.StartImport(context.Background(), string(mediaType), []string{mediaID}, grabTitle)
		if wfErr != nil {
			p.logger.Warn("failed to create reimport workflow", "source_path", sourcePath, "error", wfErr)
		} else {
			wfID = wf.ID
		}
	}

	go func() {
		defer func() { <-p.importSem }()
		ctx := context.Background()
		p.logger.Info("reimport starting (async)",
			"media_type", mediaType, "media_id", mediaID, "source_path", sourcePath)
		if _, err := p.ReimportFile(ctx, mediaType, mediaID, sourcePath, opts); err != nil {
			p.logger.Error("reimport failed",
				"media_type", mediaType, "media_id", mediaID, "source_path", sourcePath, "error", err)
			if wfID != "" {
				if fErr := p.wfEngine.FailImport(ctx, wfID, err.Error()); fErr != nil {
					p.logger.Error("failed to mark reimport workflow failed", "id", wfID, "error", fErr)
				}
			}
			return
		}
		if wfID != "" {
			if cErr := p.wfEngine.CompleteImport(ctx, wfID); cErr != nil {
				p.logger.Error("failed to mark reimport workflow completed", "id", wfID, "error", cErr)
			}
		}
	}()
	return nil
}

// ImportManual triggers an import for an arbitrary filesystem path.
func (p *ImportPipeline) ImportManual(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanMediaFiles(path)
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := filepath.Ext(path)
		if !mediaExtensions[ext] {
			return fmt.Errorf("not a media file: %s", path)
		}
		mediaFiles = []string{path}
	}

	if len(mediaFiles) == 0 {
		return fmt.Errorf("no media files found in %s", path)
	}

	fakeEvent := &downloads.DownloadCompletedEvent{
		Title:       filepath.Base(path),
		CompletedAt: time.Now(),
	}

	var lastErr error
	for _, mf := range mediaFiles {
		if err := p.importSingleFile(ctx, fakeEvent, mf); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// ImportManualMatch imports a file or folder and directly links it to a
// specific media item, bypassing fuzzy matching. The user explicitly
// provides the media_type and media_id, so no guessing is needed.
func (p *ImportPipeline) ImportManualMatch(ctx context.Context, path string, mediaType MediaType, mediaID string) (*ImportRecord, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanMediaFiles(path)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := filepath.Ext(path)
		if !mediaExtensions[ext] {
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

	// For directory-based imports in move mode, rename the source folder to
	// the destination folder path, preserving subtitles, NFOs, images, etc.
	if info.IsDir() && p.importMode == ImportModeMove && path != match.DestPath {
		if err := importFolder(path, match.DestPath); err != nil {
			p.logger.Warn("folder rename failed, falling back to per-file import",
				"src", path, "dest", match.DestPath, "error", err)
			// Ensure destination directory exists for per-file fallback
			if err := os.MkdirAll(match.DestPath, 0755); err != nil {
				return nil, fmt.Errorf("create destination directory: %w", err)
			}
		} else {
			p.logger.Info("renamed source folder to library folder",
				"src", path, "dest", match.DestPath)
			// Update media file paths to reflect the renamed folder
			for i, mf := range mediaFiles {
				rel, _ := filepath.Rel(path, mf)
				mediaFiles[i] = filepath.Join(match.DestPath, rel)
			}
		}
	} else {
		// Ensure destination directory exists
		if err := os.MkdirAll(match.DestPath, 0755); err != nil {
			return nil, fmt.Errorf("create destination directory: %w", err)
		}
	}

	var lastRecord *ImportRecord
	for _, mf := range mediaFiles {
		destFile := filepath.Join(match.DestPath, filepath.Base(mf))

		// If source and dest are the same (folder was already renamed), skip the move
		if mf != destFile {
			if err := importFile(mf, destFile, p.importMode); err != nil {
				_ = p.recordFailure(ctx, string(match.MediaType), match.MediaID, filepath.Base(mf), mf, err)
				continue
			}
		}

		if err := p.updateLibrary(ctx, match, destFile, mf); err != nil {
			p.logger.Error("library update failed after manual match import", "error", err)
			_ = os.Remove(destFile)
			_ = p.recordFailure(ctx, string(match.MediaType), match.MediaID, filepath.Base(mf), mf, err)
			continue
		}

		_ = p.recordStatus(ctx, string(match.MediaType), match.MediaID, mf, destFile, StatusImported, "")
		p.publishNotification(ctx, match, destFile)

		// Update movie status
		if match.MediaType == MediaTypeMovie && match.MediaID != "" {
			_ = p.moviesSvc.SetMovieStatus(ctx, match.MediaID, movies.MovieStatusAvailableRightQuality)
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
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanMediaFiles(path)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := filepath.Ext(path)
		if !mediaExtensions[ext] {
			return nil, fmt.Errorf("not a media file: %s", path)
		}
		mediaFiles = []string{path}
	}

	var previews []ImportPreview
	for _, mf := range mediaFiles {
		preview := p.previewSingleFile(ctx, mf)
		previews = append(previews, preview)
	}
	return previews, nil
}

func (p *ImportPipeline) previewSingleFile(ctx context.Context, filePath string) ImportPreview {
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

	destFile := filepath.Join(match.DestPath, filepath.Base(filePath))
	if _, err := os.Stat(destFile); err == nil {
		existing := ParseFileQuality(destFile)
		incoming := ParseFileQuality(filePath)
		if qualityScore(incoming) > qualityScore(existing) {
			preview.Action = "upgrade"
			preview.Reason = fmt.Sprintf("incoming quality score %d > existing %d", qualityScore(incoming), qualityScore(existing))
		} else {
			preview.Action = "skip"
			preview.Reason = fmt.Sprintf("existing quality score %d >= incoming %d", qualityScore(existing), qualityScore(incoming))
		}
	} else {
		preview.Action = "import"
		preview.Reason = "no existing file at destination"
	}

	return preview
}

// enrichCandidateQuality populates QualityProfileID and ExistingQuality
// on the candidate so the UpgradeSpec can make informed decisions.
func (p *ImportPipeline) enrichCandidateQuality(ctx context.Context, c *ImportCandidate) {
	if c.Match == nil || !c.Match.Matched {
		return
	}

	switch c.Match.MediaType {
	case MediaTypeMovie:
		movie, err := p.moviesSvc.GetMovie(ctx, c.Match.MediaID)
		if err != nil {
			return
		}
		c.QualityProfileID = movie.QualityProfileID

		files, err := p.moviesSvc.ListMovieFiles(ctx, movie.ID)
		if err != nil || len(files) == 0 {
			return
		}
		// Use the quality of the first (usually only) existing file.
		c.ExistingQuality = files[0].Quality

	case MediaTypeEpisode:
		ep, err := p.matcher.seriesSvc.GetEpisode(ctx, c.Match.MediaID)
		if err != nil {
			return
		}
		show, err := p.matcher.seriesSvc.GetSeries(ctx, ep.SeriesID)
		if err != nil {
			return
		}
		c.QualityProfileID = show.QualityProfileID
		// Episode files don't have a list method, but HasFile indicates
		// whether a file exists. Without file quality info, leave
		// ExistingQuality empty so the upgrade spec allows the import.
	}
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
