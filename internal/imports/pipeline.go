package imports

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/grabs"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/series"
)

// PipelineOptions configures the ImportPipeline.
type PipelineOptions struct {
	DB               *sql.DB
	Bus              eventbus.Bus
	DownloadSvc      *downloads.Service
	RemotePathStore  *downloads.RemotePathStore
	MoviesSvc        movies.Service
	SeriesSvc        series.Service
	LibStore         *libraries.Store
	GrabStore        *grabs.Store
	NotifSvc         notifications.Service
	PostVal          *safety.PostValidator
	ReviewStore      *safety.ReviewStore
	Logger           *slog.Logger
	ImportMode       ImportMode
}

// ImportPipeline subscribes to download completion events, scans files,
// matches them to library items, and imports them.
type ImportPipeline struct {
	db              *sql.DB
	bus             eventbus.Bus
	downloadSvc     *downloads.Service
	remotePathStore *downloads.RemotePathStore
	matcher         *Matcher
	grabStore       *grabs.Store
	notifSvc        notifications.Service
	postVal         *safety.PostValidator
	reviewStore     *safety.ReviewStore
	logger          *slog.Logger
	importMode      ImportMode
	decisionLog     *DecisionLogger
	unsub           func()
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
	return &ImportPipeline{
		db:              opts.DB,
		bus:             opts.Bus,
		downloadSvc:     opts.DownloadSvc,
		remotePathStore: opts.RemotePathStore,
		matcher:         NewMatcher(opts.MoviesSvc, opts.SeriesSvc, opts.LibStore),
		grabStore:       opts.GrabStore,
		notifSvc:        opts.NotifSvc,
		postVal:         opts.PostVal,
		reviewStore:     opts.ReviewStore,
		logger:          logger,
		importMode:      opts.ImportMode,
		decisionLog:     NewDecisionLogger(opts.DB, logger),
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

// handleCompleted processes a download completion event.
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

	downloadPath, err := p.resolveDownloadPath(ctx, completed)
	if err != nil {
		p.logger.Error("failed to resolve download path", "error", err, "title", completed.Title)
		p.recordFailure(ctx, "", "", completed.Title, "", err)
		return nil // don't block the event bus
	}

	if err := p.processImport(ctx, completed, downloadPath); err != nil {
		p.logger.Error("import failed", "error", err, "title", completed.Title, "path", downloadPath)
		return nil
	}
	return nil
}

// resolveDownloadPath determines the filesystem path of the completed download.
func (p *ImportPipeline) resolveDownloadPath(ctx context.Context, ev *downloads.DownloadCompletedEvent) (string, error) {
	if ev.ClientID == "" {
		return "", fmt.Errorf("no client_id in completion event")
	}

	client, ok := p.downloadSvc.Registry().Get(ev.ClientID)
	if !ok {
		return "", fmt.Errorf("download client %q not found in registry", ev.ClientID)
	}

	items, err := client.Status(ctx, ev.DownloadID)
	if err != nil {
		return "", fmt.Errorf("query download status: %w", err)
	}

	for _, item := range items {
		if item.ID == ev.DownloadID && item.SavePath != "" {
			path := filepath.Join(item.SavePath, item.Title)
			return p.applyRemotePathMapping(ctx, ev.ClientID, path), nil
		}
	}

	// Fallback: try the client definition's default save path
	def, err := p.downloadSvc.Get(ctx, ev.ClientID)
	if err != nil {
		return "", fmt.Errorf("get client definition: %w", err)
	}
	if def.SavePathDefault != "" {
		path := filepath.Join(def.SavePathDefault, ev.Title)
		return p.applyRemotePathMapping(ctx, ev.ClientID, path), nil
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
	return nil
}

// importSingleFile matches and imports a single media file.
func (p *ImportPipeline) importSingleFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, mediaFile string) error {
	// Try exact grab-based matching first (most reliable for Loom-originated downloads)
	match, err := p.matchByGrab(ctx, ev)
	if err != nil {
		p.logger.Warn("grab-based match failed, falling back to fuzzy", "error", err)
	}

	// Fall back to fuzzy matching by title
	if match == nil || !match.Matched {
		match, err = p.matcher.Match(ctx, ev.Title)
		if err != nil {
			return p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("match: %w", err))
		}
	}
	if !match.Matched {
		// Try matching by filename
		match, err = p.matcher.Match(ctx, filepath.Base(mediaFile))
		if err != nil {
			return p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("match by filename: %w", err))
		}
	}
	if !match.Matched {
		return p.recordFailure(ctx, "", "", ev.Title, mediaFile, fmt.Errorf("no match found for %q", ev.Title))
	}

	// Build destination path
	destFile := filepath.Join(match.DestPath, filepath.Base(mediaFile))

	// Import the file
	if err := importFile(mediaFile, destFile, p.importMode); err != nil {
		return p.recordFailure(ctx, string(match.MediaType), match.MediaID, ev.Title, mediaFile, err)
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

	// Publish notification
	p.publishNotification(ctx, match, destFile)

	// Publish event
	_ = p.bus.Publish(ctx, &ImportCompletedEvent{
		MediaType: match.MediaType,
		MediaID:   match.MediaID,
		Title:     match.Title,
		DestPath:  destFile,
	})

	// Clean up grab tracking now that import succeeded
	p.cleanupGrab(ctx, ev, match)

	return nil
}

// matchByGrab attempts to match using exact grab linkage data recorded
// when the download was initiated. This is the most reliable path for
// Loom-originated downloads since it avoids fuzzy title matching.
func (p *ImportPipeline) matchByGrab(ctx context.Context, ev *downloads.DownloadCompletedEvent) (*MatchResult, error) {
	if p.grabStore == nil || ev.ClientID == "" || ev.DownloadID == "" {
		return nil, nil
	}

	gm, err := p.grabStore.LookupByDownload(ctx, ev.ClientID, ev.DownloadID)
	if err != nil {
		return nil, err
	}
	if gm == nil {
		return nil, nil
	}

	// Episode-based match
	if len(gm.EpisodeIDs) > 0 {
		ep, err := p.matcher.seriesSvc.GetEpisode(ctx, gm.EpisodeIDs[0])
		if err != nil {
			return nil, fmt.Errorf("get episode from grab: %w", err)
		}
		s, err := p.matcher.seriesSvc.GetSeries(ctx, ep.SeriesID)
		if err != nil {
			return nil, fmt.Errorf("get series from grab: %w", err)
		}
		lib, err := p.matcher.libStore.Get(ctx, s.LibraryID)
		if err != nil {
			return nil, fmt.Errorf("get library from grab: %w", err)
		}
		// Look up the season to get the season number
		seasons, err := p.matcher.seriesSvc.ListSeasons(ctx, ep.SeriesID)
		if err != nil {
			return nil, fmt.Errorf("list seasons from grab: %w", err)
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
		p.logger.Info("matched via grab linkage",
			"media_type", "episode", "series", s.Title,
			"season", seasonNum, "episode", ep.EpisodeNumber)
		return &MatchResult{
			Matched:   true,
			MediaType: MediaTypeEpisode,
			MediaID:   ep.ID,
			Title:     s.Title,
			Year:      s.Year,
			Season:    seasonNum,
			Episode:   ep.EpisodeNumber,
			DestPath:  destDir,
		}, nil
	}

	// Movie-based match
	if len(gm.MovieIDs) > 0 {
		mv, err := p.matcher.moviesSvc.GetMovie(ctx, gm.MovieIDs[0])
		if err != nil {
			return nil, fmt.Errorf("get movie from grab: %w", err)
		}
		lib, err := p.matcher.libStore.Get(ctx, mv.LibraryID)
		if err != nil {
			return nil, fmt.Errorf("get library from grab: %w", err)
		}
		destDir := filepath.Join(lib.Path, sanitizeDirName(fmt.Sprintf("%s (%d)", mv.Title, mv.Year)))
		p.logger.Info("matched via grab linkage",
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

// cleanupGrab removes the grab tracking entry after a successful import.
// Uses download-level removal (clientID + downloadID) when available,
// falling back to per-media removal.
func (p *ImportPipeline) cleanupGrab(ctx context.Context, ev *downloads.DownloadCompletedEvent, match *MatchResult) {
	if p.grabStore == nil {
		return
	}

	// Try download-level cleanup first (most precise)
	if ev.ClientID != "" && ev.DownloadID != "" {
		if err := p.grabStore.RemoveByDownload(ctx, ev.ClientID, ev.DownloadID); err != nil {
			p.logger.Warn("grab cleanup by download failed", "error", err)
		} else {
			p.logger.Debug("grab cleaned up by download",
				"client_id", ev.ClientID, "download_id", ev.DownloadID)
			return
		}
	}

	// Fallback: per-media cleanup
	switch match.MediaType {
	case MediaTypeEpisode:
		if err := p.grabStore.RemoveByEpisode(ctx, match.MediaID); err != nil {
			p.logger.Warn("grab cleanup by episode failed", "error", err)
		}
	case MediaTypeMovie:
		if err := p.grabStore.RemoveByMovie(ctx, match.MediaID); err != nil {
			p.logger.Warn("grab cleanup by movie failed", "error", err)
		}
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
		return p.matcher.moviesSvc.AddMovieFile(ctx, mf)

	case MediaTypeEpisode:
		ef := &series.EpisodeFile{
			ID:        uuid.New().String(),
			EpisodeID: match.MediaID,
			FilePath:  destFile,
			FileSize:  info.Size(),
		}
		return p.matcher.seriesSvc.CreateEpisodeFile(ctx, ef)

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

	match, err := p.matcher.Match(ctx, filepath.Base(filePath))
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
