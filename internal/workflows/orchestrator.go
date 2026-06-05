package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	commandBufferSize     = 256
	maxConcurrentImports  = 2
	progressFlushInterval = 60 * time.Second
	importRetryDelay      = 30 * time.Second
)

// importRetryStrategy classifies how an import failure should be retried.
type importRetryStrategy int

const (
	// retryImport re-dispatches the import after a delay (transient errors like path not found).
	retryImport importRetryStrategy = iota
	// retrySearch transitions back to searching state (wrong release matched).
	retrySearch
	// failPermanent marks the workflow as permanently failed (non-retryable errors).
	failPermanent
)

// ImportFunc is the function signature the orchestrator calls to run an import.
// It receives the workflow ID, download client ID, download ID, title, and category.
// It returns the imported file paths or an error.
type ImportFunc func(ctx context.Context, clientID, downloadID, title, category string) ([]string, error)

// CleanupFunc is called after a successful import to remove the download from
// the client queue and clean any remaining junk from the source folder.
// clientID and downloadID identify the torrent/nzb; importedPaths are the
// library destinations (used to locate the source folder if needed).
type CleanupFunc func(ctx context.Context, clientID, downloadID string, importedPaths []string) error

// MediaRefreshFunc is called after a successful import to refresh media
// metadata and file status in the library.
// mediaType is "movie" or "episode"; mediaIDs are the affected item IDs.
type MediaRefreshFunc func(ctx context.Context, mediaType string, mediaIDs []string) error

// ActiveDownloadInfo carries the status and path info for a single download.
type ActiveDownloadInfo struct {
	Status      string
	ContentPath string // actual on-disk path reported by the download client
	SavePath    string // client save directory; used as fallback
}

// DownloadStatusProvider allows the orchestrator to query current download state
// for startup reconciliation without importing the downloads package.
type DownloadStatusProvider interface {
	// ActiveDownloads returns a map of "clientID:downloadID" → ActiveDownloadInfo
	// for all currently active downloads across all clients.
	ActiveDownloads(ctx context.Context) (map[string]ActiveDownloadInfo, error)
}

// OrchestratorOpts configures the Orchestrator.
type OrchestratorOpts struct {
	Store          *Store
	Engine         *Engine
	Logger         *slog.Logger
	ImportFn       ImportFunc
	CleanupFn      CleanupFunc            // optional; if nil, cleanup phase is skipped
	MediaRefreshFn MediaRefreshFunc       // optional; if nil, media refresh is skipped
	DownloadStatus DownloadStatusProvider // optional, for startup reconciliation
}

// Orchestrator is the single coordinator for all workflow state transitions.
// It consumes typed commands from a buffered channel and serializes all
// mutations through a single goroutine — eliminating scattered Mark* calls.
type Orchestrator struct {
	store          *Store
	engine         *Engine
	logger         *slog.Logger
	importFn       ImportFunc
	cleanupFn      CleanupFunc
	mediaRefreshFn MediaRefreshFunc
	dlStatus       DownloadStatusProvider

	commands chan Command

	// Import concurrency limiter.
	importSem chan struct{}

	// Progress coalescing: buffered updates flushed on tick.
	progressMu  sync.Mutex
	progressBuf map[string]*CmdDownloadProgress // key: clientID:downloadID
}

// Store returns the underlying workflow store for read-only queries
// (e.g. duplicate-active-workflow checks).
func (o *Orchestrator) Store() *Store { return o.store }

// SetImportFn sets the import function after construction (for wiring order flexibility).
func (o *Orchestrator) SetImportFn(fn ImportFunc) { o.importFn = fn }

// SetCleanupFn sets the post-import cleanup function.
func (o *Orchestrator) SetCleanupFn(fn CleanupFunc) { o.cleanupFn = fn }

// SetMediaRefreshFn sets the post-import media refresh function.
func (o *Orchestrator) SetMediaRefreshFn(fn MediaRefreshFunc) { o.mediaRefreshFn = fn }

// NewOrchestrator creates a workflow orchestrator.
func NewOrchestrator(opts OrchestratorOpts) *Orchestrator {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Orchestrator{
		store:          opts.Store,
		engine:         opts.Engine,
		logger:         opts.Logger.With("component", "workflow-orchestrator"),
		importFn:       opts.ImportFn,
		cleanupFn:      opts.CleanupFn,
		mediaRefreshFn: opts.MediaRefreshFn,
		dlStatus:       opts.DownloadStatus,
		commands:       make(chan Command, commandBufferSize),
		importSem:      make(chan struct{}, maxConcurrentImports),
		progressBuf:    make(map[string]*CmdDownloadProgress),
	}
}

// Send enqueues a command for the orchestrator. Non-blocking: if the buffer
// is full, logs a warning and drops the command (callers should not block).
func (o *Orchestrator) Send(cmd Command) {
	select {
	case o.commands <- cmd:
	default:
		o.logger.Warn("orchestrator command buffer full, dropping command",
			"type", fmt.Sprintf("%T", cmd))
	}
}

// NotifyDownloadComplete satisfies downloads.MonitorOrchNotifier.
func (o *Orchestrator) NotifyDownloadComplete(clientID, downloadID, title, category, contentPath, savePath string) {
	o.Send(CmdDownloadComplete{
		ClientID:    clientID,
		DownloadID:  downloadID,
		Title:       title,
		Category:    category,
		ContentPath: contentPath,
		SavePath:    savePath,
	})
}

// NotifyDownloadProgress satisfies downloads.MonitorOrchNotifier.
func (o *Orchestrator) NotifyDownloadProgress(clientID, downloadID string, progress float64, downSpeed, upSpeed int64, ratio float64, status, contentPath, savePath string) {
	o.Send(CmdDownloadProgress{
		ClientID:    clientID,
		DownloadID:  downloadID,
		Progress:    progress,
		DownSpeed:   downSpeed,
		UpSpeed:     upSpeed,
		Ratio:       ratio,
		Status:      status,
		ContentPath: contentPath,
		SavePath:    savePath,
	})
}

// NotifyDownloadRemoved is called when a user removes a download from the client.
func (o *Orchestrator) NotifyDownloadRemoved(clientID string, downloadIDs []string) {
	for _, dlID := range downloadIDs {
		o.Send(CmdDownloadRemoved{ClientID: clientID, DownloadID: dlID})
	}
}

// StartSearch is the synchronous API for creating a new workflow.
// Callers need the workflow ID back to later send CmdGrabbed.
func (o *Orchestrator) StartSearch(ctx context.Context, wfType, mediaType, qualityProfileID string, mediaIDs []string) (*Workflow, error) {
	reply := make(chan SearchReply, 1)
	cmd := CmdSearchStarted{
		WfType:           wfType,
		MediaType:        mediaType,
		QualityProfileID: qualityProfileID,
		MediaIDs:         mediaIDs,
		Reply:            reply,
	}

	select {
	case o.commands <- cmd:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case r := <-reply:
		return r.Workflow, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run is the main orchestrator loop. Blocks until ctx is cancelled.
func (o *Orchestrator) Run(ctx context.Context) {
	o.logger.Info("orchestrator starting")

	// Startup reconciliation: recover missed events.
	o.reconcileOnBoot(ctx)

	ticker := time.NewTicker(SchedulerTick)
	defer ticker.Stop()

	progressTicker := time.NewTicker(progressFlushInterval)
	defer progressTicker.Stop()

	pruneTicker := time.NewTicker(1 * time.Hour)
	defer pruneTicker.Stop()

	o.logger.Info("orchestrator running", "command_buffer", commandBufferSize,
		"max_imports", maxConcurrentImports)

	for {
		select {
		case <-ctx.Done():
			o.logger.Info("orchestrator stopped")
			return
		case cmd := <-o.commands:
			o.handle(ctx, cmd)
		case <-ticker.C:
			o.handleStale(ctx)
			o.checkPostDownloadWorkflows(ctx)
		case <-progressTicker.C:
			o.flushProgress(ctx)
		case <-pruneTicker.C:
			o.pruneCompleted(ctx)
		}
	}
}

// handle dispatches a command to the appropriate handler.
func (o *Orchestrator) handle(ctx context.Context, cmd Command) {
	switch c := cmd.(type) {
	case CmdSearchStarted:
		o.handleSearchStarted(ctx, c)
	case CmdGrabbed:
		o.handleGrabbed(ctx, c)
	case CmdDownloadProgress:
		o.bufferProgress(ctx, c)
	case CmdDownloadComplete:
		o.handleDownloadComplete(ctx, c)
	case CmdImportResult:
		o.handleImportResult(ctx, c)
	case CmdCancel:
		o.handleCancel(ctx, c)
	case CmdDownloadRemoved:
		o.handleDownloadRemoved(ctx, c)
	case CmdRetry:
		o.handleRetry(ctx, c)
	default:
		o.logger.Warn("unknown command type", "type", fmt.Sprintf("%T", cmd))
	}
}

// ── Command handlers ──────────────────────────────────────────────────

func (o *Orchestrator) handleSearchStarted(ctx context.Context, cmd CmdSearchStarted) {
	wf, err := o.engine.StartSearch(ctx, cmd.WfType, cmd.MediaType, cmd.QualityProfileID, cmd.MediaIDs)
	if err != nil {
		cmd.Reply <- SearchReply{Err: err}
		return
	}

	o.logEvent(ctx, wf.ID, EventSearchStarted, "Search initiated", map[string]any{
		"type":               cmd.WfType,
		"media_type":         cmd.MediaType,
		"media_ids":          cmd.MediaIDs,
		"quality_profile_id": cmd.QualityProfileID,
	})

	cmd.Reply <- SearchReply{Workflow: wf}
}

func (o *Orchestrator) handleGrabbed(ctx context.Context, cmd CmdGrabbed) {
	if err := o.engine.markGrabbed(ctx, cmd.WorkflowID, cmd.ClientID, cmd.DownloadID, cmd.Title); err != nil {
		o.logger.Error("failed to mark grabbed", "workflow_id", cmd.WorkflowID, "error", err)
		o.logEvent(ctx, cmd.WorkflowID, EventFailed, "Failed to mark grabbed: "+err.Error(), nil)
		return
	}

	o.logEvent(ctx, cmd.WorkflowID, EventGrabbed, "Release grabbed: "+cmd.Title, map[string]any{
		"client_id":   cmd.ClientID,
		"download_id": cmd.DownloadID,
		"title":       cmd.Title,
	})

	// Cache the actual on-disk path immediately at grab time so it survives
	// restarts and client-side removal. The builtin torrent client populates
	// ContentPath from t.Name() right after add; external clients may leave it empty.
	patch := map[string]any{}
	if cmd.ContentPath != "" {
		patch["content_path"] = cmd.ContentPath
	}
	if cmd.SavePath != "" {
		patch["save_path"] = cmd.SavePath
	}
	if len(patch) > 0 {
		if err := o.store.MergeMetadata(ctx, cmd.WorkflowID, patch); err != nil {
			o.logger.Warn("failed to cache content path at grab time", "workflow_id", cmd.WorkflowID, "error", err)
		}
	}

	// Persist seed policy so post_download phase knows the requirements.
	if cmd.SeedRatioLimit != nil || cmd.SeedTimeLimitMinutes != nil {
		policy := PostDownloadPolicy{
			SeedRatioLimit:       cmd.SeedRatioLimit,
			SeedTimeLimitMinutes: cmd.SeedTimeLimitMinutes,
		}
		if err := o.store.SetPostDownloadPolicy(ctx, cmd.WorkflowID, policy); err != nil {
			o.logger.Warn("failed to store seed policy", "workflow_id", cmd.WorkflowID, "error", err)
		}
	}

	// Also transition to downloading immediately — the download client has accepted it
	if err := o.engine.markDownloading(ctx, cmd.WorkflowID); err != nil {
		o.logger.Debug("grabbed → downloading transition deferred", "workflow_id", cmd.WorkflowID)
	} else {
		o.logEvent(ctx, cmd.WorkflowID, EventDownloading, "Download started", nil)
	}
}

func (o *Orchestrator) handleDownloadComplete(ctx context.Context, cmd CmdDownloadComplete) {
	// Find the workflow by download client + download ID
	wf, err := o.engine.FindByDownload(ctx, cmd.ClientID, cmd.DownloadID)
	if err != nil {
		o.logger.Error("failed to find workflow for download", "client_id", cmd.ClientID,
			"download_id", cmd.DownloadID, "error", err)
		return
	}
	if wf == nil {
		// Orphan download — no workflow tracks this. Leave it for event-bus path.
		o.logger.Debug("no workflow for completed download", "client_id", cmd.ClientID,
			"download_id", cmd.DownloadID, "title", cmd.Title)
		return
	}

	// Idempotent: if already past downloading, nothing to do.
	switch wf.State {
	case StatePostDownload, StateImporting, StateCompleted:
		o.logger.Debug("download complete received but workflow already advanced",
			"workflow_id", wf.ID, "state", wf.State)
		return
	case StateFailed, StateCancelled:
		o.logger.Debug("download complete received for terminal workflow",
			"workflow_id", wf.ID, "state", wf.State)
		return
	}

	// Ensure we're at least in downloading state
	if wf.State == StateGrabbed {
		_ = o.engine.markDownloading(ctx, wf.ID)
	}

	o.logEvent(ctx, wf.ID, EventDownloadComplete, "Download finished: "+cmd.Title, map[string]any{
		"client_id":   cmd.ClientID,
		"download_id": cmd.DownloadID,
		"title":       cmd.Title,
		"category":    cmd.Category,
	})

	// Transition to post_download (settling + seed tracking) instead of
	// importing immediately. This gives the download client time to
	// release file locks and lets torrents seed to their target ratio.
	if err := o.engine.markPostDownload(ctx, wf.ID); err != nil {
		o.logger.Error("failed to mark post_download", "workflow_id", wf.ID, "error", err)
		o.logEvent(ctx, wf.ID, EventImportFailed, "Failed to enter post-download: "+err.Error(), nil)
		return
	}

	// Stamp the post_download start time.
	policy := GetPostDownloadPolicy(wf.Metadata)
	if policy == nil {
		policy = &PostDownloadPolicy{}
	}
	policy.StartedAt = time.Now()
	if policy.SettlingDelay == 0 {
		policy.SettlingDelay = DefaultSettlingDelaySec
	}
	if err := o.store.SetPostDownloadPolicy(ctx, wf.ID, *policy); err != nil {
		o.logger.Warn("failed to stamp post_download start", "workflow_id", wf.ID, "error", err)
	}

	// Store category and content path in metadata for later import dispatch.
	patch := map[string]any{}
	if cmd.Category != "" {
		patch["category"] = cmd.Category
	}
	// Cache the download path so the import pipeline can resolve it even if
	// the item is later removed from the download client (e.g. after seeding).
	if cmd.ContentPath != "" {
		patch["content_path"] = cmd.ContentPath
	} else if cmd.SavePath != "" {
		patch["save_path"] = cmd.SavePath
	}
	if len(patch) > 0 {
		_ = o.store.MergeMetadata(ctx, wf.ID, patch)
	}

	o.logEvent(ctx, wf.ID, EventPostDownloadStart, "Post-download phase started", map[string]any{
		"settling_delay_sec":  policy.SettlingDelay,
		"seed_ratio_limit":    policy.SeedRatioLimit,
		"seed_time_limit_min": policy.SeedTimeLimitMinutes,
	})
}

func (o *Orchestrator) handleImportResult(ctx context.Context, cmd CmdImportResult) {
	// Guard: if the workflow is already in a terminal state (e.g. cancelled while
	// an import was in-flight) ignore this result entirely — no more retries.
	currentWf, err := o.store.Get(ctx, cmd.WorkflowID)
	if err != nil {
		o.logger.Error("failed to fetch workflow for import result", "workflow_id", cmd.WorkflowID, "error", err)
		return
	}
	if currentWf.IsTerminal() {
		o.logger.Debug("ignoring import result for terminal workflow",
			"workflow_id", cmd.WorkflowID, "state", currentWf.State)
		return
	}

	if cmd.Success {
		msg := "Import completed successfully"
		meta := map[string]any{"imported_paths": cmd.ImportedPaths}
		o.logEvent(ctx, cmd.WorkflowID, EventImportSuccess, msg, meta)

		// Fetch workflow to get download client/ID and media info for cleanup and refresh.
		wf, err := o.store.Get(ctx, cmd.WorkflowID)
		if err != nil {
			o.logger.Error("failed to fetch workflow for post-import", "workflow_id", cmd.WorkflowID, "error", err)
			// Best-effort: still mark completed even if we can't clean up.
			if err := o.engine.markCompleted(ctx, cmd.WorkflowID, msg); err != nil {
				o.logger.Error("failed to mark completed", "workflow_id", cmd.WorkflowID, "error", err)
			}
			return
		}

		// Transition to cleaning_up state so the user can see progress.
		if err := o.engine.markCleaningUp(ctx, cmd.WorkflowID, "Running post-import cleanup"); err != nil {
			o.logger.Warn("failed to transition to cleaning_up, completing directly",
				"workflow_id", cmd.WorkflowID, "error", err)
			if err := o.engine.markCompleted(ctx, cmd.WorkflowID, msg); err != nil {
				o.logger.Error("failed to mark completed", "workflow_id", cmd.WorkflowID, "error", err)
			}
			return
		}
		o.logEvent(ctx, cmd.WorkflowID, EventCleanupStarted, "Post-import cleanup started", nil)

		// Run cleanup and media refresh in a background goroutine so the
		// orchestrator command loop isn't blocked.
		go o.runPostImportCleanup(ctx, wf, cmd.ImportedPaths)
		return
	}

	strategy := o.classifyImportError(cmd.Error)

	o.logEvent(ctx, cmd.WorkflowID, EventImportFailed, "Import failed: "+cmd.Error, map[string]any{
		"error":          cmd.Error,
		"retry_strategy": strategy.String(),
	})

	switch strategy {
	case retryImport:
		// Look up workflow to get download info for re-dispatch.
		wf, err := o.store.Get(ctx, cmd.WorkflowID)
		if err != nil {
			o.logger.Error("failed to fetch workflow for retry", "workflow_id", cmd.WorkflowID, "error", err)
			break
		}

		// Increment retry count and check exhaustion. markFailed handles both:
		// - retries remaining: increments counter, leaves workflow in importing state
		//   (importing→importing is an invalid transition that silently no-ops).
		// - retries exhausted: transitions to failed and resets media status.
		if err := o.engine.markFailed(ctx, cmd.WorkflowID, "Import transient error: "+cmd.Error); err != nil {
			o.logger.Error("failed to record import retry", "workflow_id", cmd.WorkflowID, "error", err)
			break
		}

		// Re-fetch to see whether retries were exhausted (workflow now failed).
		updatedWf, err := o.store.Get(ctx, cmd.WorkflowID)
		if err != nil {
			o.logger.Error("failed to re-fetch workflow after retry increment", "workflow_id", cmd.WorkflowID, "error", err)
			break
		}
		if updatedWf.IsTerminal() {
			// Retries exhausted — markFailed transitioned to failed.
			return
		}

		o.logger.Info("scheduling import retry after delay",
			"workflow_id", cmd.WorkflowID, "retry", updatedWf.RetryCount,
			"max", updatedWf.MaxRetries, "delay", importRetryDelay, "error", cmd.Error)

		// Extract category from metadata if available.
		category := o.categoryFromMetadata(wf.Metadata)

		time.AfterFunc(importRetryDelay, func() {
			// Re-check workflow state: it may have been cancelled while waiting.
			latestWf, err := o.store.Get(ctx, wf.ID)
			if err != nil || latestWf.IsTerminal() || latestWf.State != StateImporting {
				o.logger.Debug("aborting scheduled import retry — workflow no longer in importing state",
					"workflow_id", wf.ID)
				return
			}
			o.logEvent(ctx, wf.ID, EventRetried, "Re-importing after delay", map[string]any{
				"retry_strategy": strategy.String(),
				"delay":          importRetryDelay.String(),
			})
			o.dispatchImport(ctx, wf.ID, wf.DownloadClientID, wf.DownloadID, wf.GrabTitle, category)
		})
		return

	case retrySearch:
		o.logger.Info("import failed with no match, falling back to re-search",
			"workflow_id", cmd.WorkflowID, "error", cmd.Error)
		if err := o.engine.markFailed(ctx, cmd.WorkflowID, "Import failed (re-search): "+cmd.Error); err != nil {
			o.logger.Error("failed to mark failed for re-search", "workflow_id", cmd.WorkflowID, "error", err)
		}
		return

	case failPermanent:
		o.logger.Warn("import failed permanently",
			"workflow_id", cmd.WorkflowID, "error", cmd.Error)
		// Force max-retry exhaustion by passing a clear permanent failure message.
		if err := o.engine.markFailed(ctx, cmd.WorkflowID, "Import failed permanently: "+cmd.Error); err != nil {
			o.logger.Error("failed to mark permanently failed", "workflow_id", cmd.WorkflowID, "error", err)
		}
		return
	}
}

// runPostImportCleanup handles the cleanup_up phase after a successful import:
//  1. Calls cleanupFn (if set) to remove the download from the client and
//     scrub any remaining junk from the source folder.
//  2. Calls mediaRefreshFn (if set) to refresh movie/series metadata and
//     file status in the library.
//  3. Marks the workflow as completed.
//
// This runs in a background goroutine so the orchestrator command loop is
// never blocked.
func (o *Orchestrator) runPostImportCleanup(ctx context.Context, wf *Workflow, importedPaths []string) {
	wfID := wf.ID

	// Step 1: remove download from client + clean folder.
	if o.cleanupFn != nil && wf.DownloadClientID != "" && wf.DownloadID != "" {
		if err := o.cleanupFn(ctx, wf.DownloadClientID, wf.DownloadID, importedPaths); err != nil {
			// Non-fatal — log and continue to completion.
			o.logger.Warn("post-import cleanup failed (non-fatal)",
				"workflow_id", wfID,
				"client_id", wf.DownloadClientID,
				"download_id", wf.DownloadID,
				"error", err,
			)
		} else {
			o.logger.Info("post-import cleanup succeeded",
				"workflow_id", wfID,
				"client_id", wf.DownloadClientID,
				"download_id", wf.DownloadID,
			)
		}
	}

	// Step 2: refresh media status.
	if o.mediaRefreshFn != nil && len(wf.Items) > 0 {
		var mediaIDs []string
		mediaType := wf.MediaType
		for _, item := range wf.Items {
			mediaIDs = append(mediaIDs, item.MediaID)
		}
		if err := o.mediaRefreshFn(ctx, mediaType, mediaIDs); err != nil {
			o.logger.Warn("post-import media refresh failed (non-fatal)",
				"workflow_id", wfID,
				"media_type", mediaType,
				"media_ids", mediaIDs,
				"error", err,
			)
		} else {
			o.logger.Info("post-import media refresh succeeded",
				"workflow_id", wfID,
				"media_type", mediaType,
				"media_ids", mediaIDs,
			)
		}
	}

	o.logEvent(ctx, wfID, EventCleanupCompleted, "Post-import cleanup completed", nil)

	if err := o.engine.markCompleted(ctx, wfID, "Import and cleanup completed"); err != nil {
		o.logger.Error("failed to mark completed after cleanup", "workflow_id", wfID, "error", err)
	}
}

// classifyImportError determines the retry strategy based on the error message.
func (o *Orchestrator) classifyImportError(errMsg string) importRetryStrategy {
	lower := strings.ToLower(errMsg)

	// Non-retryable errors — fail immediately.
	for _, s := range []string{
		"permission denied", "access denied", "unauthorized",
		"already been imported", "already imported",
		"import rejected",
	} {
		if strings.Contains(lower, s) {
			return failPermanent
		}
	}

	// Errors suggesting the wrong release was grabbed — re-search.
	for _, s := range []string{"no match found", "no files found", "unmatched", "wrong series"} {
		if strings.Contains(lower, s) {
			return retrySearch
		}
	}

	// Transient errors — retry the import after a delay.
	// This includes "path not found", timeouts, and any other unclassified error.
	return retryImport
}

// categoryFromMetadata extracts the "category" field from a workflow's metadata JSON.
func (o *Orchestrator) categoryFromMetadata(metadata string) string {
	if metadata == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return ""
	}
	if cat, ok := m["category"].(string); ok {
		return cat
	}
	return ""
}

// String returns a human-readable label for the retry strategy.
func (s importRetryStrategy) String() string {
	switch s {
	case retryImport:
		return "retry_import"
	case retrySearch:
		return "retry_search"
	case failPermanent:
		return "fail_permanent"
	default:
		return "unknown"
	}
}

func (o *Orchestrator) handleCancel(ctx context.Context, cmd CmdCancel) {
	err := o.engine.Cancel(ctx, cmd.WorkflowID)
	if err == nil {
		o.logEvent(ctx, cmd.WorkflowID, EventCancelled, "Cancelled by user", nil)
	}
	if cmd.Reply != nil {
		cmd.Reply <- err
	}
}

func (o *Orchestrator) handleRetry(ctx context.Context, cmd CmdRetry) {
	wf, err := o.engine.Store().Get(ctx, cmd.WorkflowID)
	if err != nil {
		if cmd.Reply != nil {
			cmd.Reply <- err
		}
		return
	}
	if wf.State != StateFailed {
		err = fmt.Errorf("can only retry failed workflows, current state: %s", wf.State)
		if cmd.Reply != nil {
			cmd.Reply <- err
		}
		return
	}

	// Smart retry: check download status to determine the best recovery point
	if wf.DownloadClientID != "" && wf.DownloadID != "" && o.dlStatus != nil {
		err = o.smartRetry(ctx, wf)
	} else {
		// No download info — start from scratch
		err = o.engine.Retry(ctx, wf.ID)
		if err == nil {
			o.logEvent(ctx, wf.ID, EventRetried, "Manual retry (re-search)", nil)
		}
	}

	if cmd.Reply != nil {
		cmd.Reply <- err
	}
}

// smartRetry checks the download state and picks the optimal retry point.
func (o *Orchestrator) smartRetry(ctx context.Context, wf *Workflow) error {
	downloads, err := o.dlStatus.ActiveDownloads(ctx)
	if err != nil {
		o.logger.Warn("smart retry: failed to query downloads, falling back to re-search",
			"workflow_id", wf.ID, "error", err)
		if err := o.engine.Retry(ctx, wf.ID); err != nil {
			return err
		}
		o.logEvent(ctx, wf.ID, EventRetried, "Manual retry (re-search, download query failed)", nil)
		return nil
	}

	key := wf.DownloadClientID + ":" + wf.DownloadID
	info, exists := downloads[key]

	switch {
	case !exists:
		// Download gone — re-search from scratch
		if err := o.engine.Retry(ctx, wf.ID); err != nil {
			return err
		}
		o.logEvent(ctx, wf.ID, EventRetried, "Manual retry (re-search, download not found)", nil)

	case info.Status == "completed":
		// Download complete — skip straight to import
		if err := o.engine.RecoverToImporting(ctx, wf.ID, "Manual retry (download complete, re-importing)"); err != nil {
			return err
		}
		o.logEvent(ctx, wf.ID, EventRetried, "Manual retry (re-importing)", nil)
		category := o.categoryFromMetadata(wf.Metadata)
		o.dispatchImport(ctx, wf.ID, wf.DownloadClientID, wf.DownloadID, wf.GrabTitle, category)

	case info.Status == "seeding":
		// Still seeding — go to post_download to evaluate seed requirements
		if err := o.engine.RecoverToPostDownload(ctx, wf.ID, "Manual retry (seeding, evaluating seed requirements)"); err != nil {
			return err
		}
		o.logEvent(ctx, wf.ID, EventRetried, "Manual retry (evaluating seed status)", nil)

	case info.Status == "downloading":
		// Still downloading — resume from downloading state
		if err := o.engine.RecoverToDownloading(ctx, wf.ID, "Manual retry (download still active)"); err != nil {
			return err
		}
		o.logEvent(ctx, wf.ID, EventRetried, "Manual retry (download resuming)", nil)

	default:
		// Unknown state — fall back to re-search
		o.logger.Warn("smart retry: unknown download state, falling back to re-search",
			"workflow_id", wf.ID, "dl_state", info.Status)
		if err := o.engine.Retry(ctx, wf.ID); err != nil {
			return err
		}
		o.logEvent(ctx, wf.ID, EventRetried, fmt.Sprintf("Manual retry (re-search, unknown dl state: %s)", info.Status), nil)
	}

	return nil
}

func (o *Orchestrator) handleDownloadRemoved(ctx context.Context, cmd CmdDownloadRemoved) {
	wf, err := o.engine.FindByDownload(ctx, cmd.ClientID, cmd.DownloadID)
	if err != nil {
		o.logger.Error("failed to find workflow for removed download",
			"client_id", cmd.ClientID, "download_id", cmd.DownloadID, "error", err)
		return
	}
	if wf == nil {
		return // no workflow tracks this download
	}
	if wf.IsTerminal() {
		return // already done
	}

	err = o.engine.Cancel(ctx, wf.ID)
	if err != nil {
		o.logger.Warn("failed to cancel workflow for removed download",
			"workflow_id", wf.ID, "error", err)
		return
	}
	o.logEvent(ctx, wf.ID, EventCancelled, "Download removed by user", map[string]any{
		"client_id":   cmd.ClientID,
		"download_id": cmd.DownloadID,
	})
}

// ── Progress coalescing ───────────────────────────────────────────────

func (o *Orchestrator) bufferProgress(ctx context.Context, cmd CmdDownloadProgress) {
	// For workflows in post_download, evaluate immediately instead of
	// buffering — seed/settling checks need prompt evaluation.
	wf, err := o.engine.FindByDownload(ctx, cmd.ClientID, cmd.DownloadID)
	if err == nil && wf != nil {
		if wf.State == StatePostDownload {
			_ = o.store.MergeMetadata(ctx, wf.ID, map[string]any{
				"ratio":  cmd.Ratio,
				"status": cmd.Status,
			})
			o.evaluatePostDownload(ctx, wf, cmd.Ratio, cmd.Status)
			return
		}

		// Recovery: if download is complete/seeding but workflow is still
		// in grabbed/downloading, the completion command was missed.
		if (cmd.Status == "seeding" || cmd.Status == "completed") &&
			(wf.State == StateGrabbed || wf.State == StateDownloading) {
			o.logger.Info("recovering missed completion via progress",
				"workflow_id", wf.ID, "state", wf.State, "status", cmd.Status)
			o.handleDownloadComplete(ctx, CmdDownloadComplete{
				ClientID:    cmd.ClientID,
				DownloadID:  cmd.DownloadID,
				ContentPath: cmd.ContentPath,
				SavePath:    cmd.SavePath,
			})
			return
		}
	}

	o.progressMu.Lock()
	defer o.progressMu.Unlock()
	key := cmd.ClientID + ":" + cmd.DownloadID
	o.progressBuf[key] = &cmd
}

func (o *Orchestrator) flushProgress(ctx context.Context) {
	o.progressMu.Lock()
	buf := o.progressBuf
	o.progressBuf = make(map[string]*CmdDownloadProgress, len(buf))
	o.progressMu.Unlock()

	for _, p := range buf {
		wf, err := o.engine.FindByDownload(ctx, p.ClientID, p.DownloadID)
		if err != nil || wf == nil {
			continue
		}

		// If the download is complete/seeding but the workflow is still in
		// grabbed/downloading, the completion command was missed — recover.
		if (p.Status == "seeding" || p.Status == "completed") &&
			(wf.State == StateGrabbed || wf.State == StateDownloading) {
			o.logger.Info("recovering missed download completion from progress update",
				"workflow_id", wf.ID, "state", wf.State, "status", p.Status)
			o.handleDownloadComplete(ctx, CmdDownloadComplete{
				ClientID:    p.ClientID,
				DownloadID:  p.DownloadID,
				ContentPath: p.ContentPath,
				SavePath:    p.SavePath,
			})
			continue
		}

		// Ensure we're in downloading state
		if wf.State == StateGrabbed {
			_ = o.engine.markDownloading(ctx, wf.ID)
			o.logEvent(ctx, wf.ID, EventDownloading, "Download confirmed active", nil)
		}

		// Update metadata with progress info (merge to preserve seed policy).
		// Also cache content/save paths so they survive download client removal.
		patch := map[string]any{
			"progress":   p.Progress,
			"down_speed": p.DownSpeed,
			"up_speed":   p.UpSpeed,
			"ratio":      p.Ratio,
			"status":     p.Status,
		}
		if p.ContentPath != "" {
			patch["content_path"] = p.ContentPath
		}
		if p.SavePath != "" {
			patch["save_path"] = p.SavePath
		}
		_ = o.store.MergeMetadata(ctx, wf.ID, patch)

		// Check if a post_download workflow is ready for import.
		if wf.State == StatePostDownload {
			o.evaluatePostDownload(ctx, wf, p.Ratio, p.Status)
		}
	}
}

// ── Post-download evaluation ──────────────────────────────────────────

// evaluatePostDownload checks whether a post_download workflow is ready
// to transition to importing. Called from progress flushes and periodic ticks.
func (o *Orchestrator) evaluatePostDownload(ctx context.Context, wf *Workflow, currentRatio float64, currentStatus string) {
	policy := GetPostDownloadPolicy(wf.Metadata)
	if policy == nil || policy.StartedAt.IsZero() {
		// No policy or not stamped yet — wait for next tick.
		return
	}

	now := time.Now()
	elapsed := now.Sub(policy.StartedAt)

	// 1. Settling delay must have passed.
	settleDelay := time.Duration(policy.SettlingDelay) * time.Second
	if settleDelay == 0 {
		settleDelay = time.Duration(DefaultSettlingDelaySec) * time.Second
	}
	if elapsed < settleDelay {
		return
	}

	// 2. If seeding requirements exist and the item is still seeding, check them.
	isSeeding := currentStatus == "seeding"
	hasSeedRequirements := policy.SeedRatioLimit != nil || policy.SeedTimeLimitMinutes != nil

	if isSeeding && hasSeedRequirements {
		// Check each configured requirement. A nil limit means "not configured"
		// (don't count as met). If both are configured, either being met is
		// sufficient (OR semantics: seed until ratio OR time, whichever first).
		ratioConfigured := policy.SeedRatioLimit != nil
		timeConfigured := policy.SeedTimeLimitMinutes != nil
		ratioMet := ratioConfigured && currentRatio >= *policy.SeedRatioLimit
		timeMet := timeConfigured && elapsed >= time.Duration(*policy.SeedTimeLimitMinutes)*time.Minute

		if !ratioMet && !timeMet {
			// No configured requirement has been met — keep waiting.
			return
		}

		o.logger.Info("seed requirements met",
			"workflow_id", wf.ID,
			"ratio", currentRatio,
			"ratio_limit", policy.SeedRatioLimit,
			"elapsed_min", int(elapsed.Minutes()),
			"time_limit_min", policy.SeedTimeLimitMinutes,
		)
		o.logEvent(ctx, wf.ID, EventSeedingProgress, "Seed requirements met, proceeding to import", map[string]any{
			"ratio":          currentRatio,
			"ratio_limit":    policy.SeedRatioLimit,
			"elapsed_min":    int(elapsed.Minutes()),
			"time_limit_min": policy.SeedTimeLimitMinutes,
		})
	}

	// 3. Ready to import — transition.
	o.transitionToImport(ctx, wf)
}

// checkPostDownloadWorkflows is called on each scheduler tick to evaluate
// post_download workflows that may not have received recent progress updates
// (e.g. Usenet downloads with no seeding, or torrents that finished seeding
// and the client removed them).
func (o *Orchestrator) checkPostDownloadWorkflows(ctx context.Context) {
	active, err := o.store.ListActive(ctx)
	if err != nil {
		return
	}
	for _, wf := range active {
		if wf.State != StatePostDownload {
			continue
		}

		// Read last known status from metadata.
		var ratio float64
		var status string
		if wf.Metadata != "" {
			var m map[string]any
			if json.Unmarshal([]byte(wf.Metadata), &m) == nil {
				if r, ok := m["ratio"].(float64); ok {
					ratio = r
				}
				if s, ok := m["status"].(string); ok {
					status = s
				}
			}
		}

		o.evaluatePostDownload(ctx, wf, ratio, status)
	}
}

// transitionToImport moves a workflow from post_download to importing
// and dispatches the import.
func (o *Orchestrator) transitionToImport(ctx context.Context, wf *Workflow) {
	if err := o.engine.markImporting(ctx, wf.ID); err != nil {
		o.logger.Error("failed to mark importing from post_download", "workflow_id", wf.ID, "error", err)
		return
	}

	category := o.categoryFromMetadata(wf.Metadata)

	o.logEvent(ctx, wf.ID, EventImportStarted, "Import dispatched", nil)
	o.dispatchImport(ctx, wf.ID, wf.DownloadClientID, wf.DownloadID, wf.GrabTitle, category)
}

// ── Import dispatch ───────────────────────────────────────────────────

func (o *Orchestrator) dispatchImport(ctx context.Context, workflowID, clientID, downloadID, title, category string) {
	if o.importFn == nil {
		o.logger.Warn("no import function configured, skipping import", "workflow_id", workflowID)
		return
	}

	go func() {
		// Acquire semaphore slot
		select {
		case o.importSem <- struct{}{}:
			defer func() { <-o.importSem }()
		case <-ctx.Done():
			return
		}

		// Re-check state after acquiring the slot: the workflow may have been
		// cancelled or otherwise terminated while we were waiting.
		currentWf, err := o.store.Get(ctx, workflowID)
		if err != nil || currentWf.IsTerminal() || currentWf.State != StateImporting {
			o.logger.Debug("aborting import — workflow no longer in importing state",
				"workflow_id", workflowID)
			return
		}

		o.logger.Info("import worker started", "workflow_id", workflowID, "title", title)

		paths, err := o.importFn(ctx, clientID, downloadID, title, category)

		result := CmdImportResult{
			WorkflowID:    workflowID,
			ImportedPaths: paths,
		}
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
		}

		// Send result back to orchestrator
		o.Send(result)
	}()
}

// ── Periodic tasks ────────────────────────────────────────────────────

func (o *Orchestrator) handleStale(ctx context.Context) {
	stale, err := o.store.StaleWorkflows(ctx)
	if err != nil {
		o.logger.Error("stale check failed", "error", err)
		return
	}

	// Query current download states for recovery attempts.
	var dlStates map[string]ActiveDownloadInfo
	if o.dlStatus != nil {
		dlStates, _ = o.dlStatus.ActiveDownloads(ctx)
	}

	for _, wf := range stale {
		age := time.Since(wf.UpdatedAt)

		// For downloading/grabbed workflows, attempt recovery before failing.
		if wf.State == StateDownloading || wf.State == StateGrabbed {
			if dlStates != nil && wf.DownloadClientID != "" && wf.DownloadID != "" {
				key := wf.DownloadClientID + ":" + wf.DownloadID
				if info, exists := dlStates[key]; exists {
					if info.Status == "completed" || info.Status == "seeding" {
						o.logger.Info("stale recovery: download is actually complete, recovering",
							"workflow_id", wf.ID, "state", wf.State, "dl_state", info.Status)
						o.logEvent(ctx, wf.ID, EventStaleDetected,
							fmt.Sprintf("Stale in %s but download is %s — recovering", wf.State, info.Status), nil)
						o.handleDownloadComplete(ctx, CmdDownloadComplete{
							ClientID:    wf.DownloadClientID,
							DownloadID:  wf.DownloadID,
							Title:       wf.GrabTitle,
							ContentPath: info.ContentPath,
							SavePath:    info.SavePath,
						})
						continue
					}
					if info.Status == "downloading" {
						// Still downloading — touch updated_at to reset stale timer
						_ = o.store.MergeMetadata(ctx, wf.ID, map[string]any{"stale_check": "still_downloading"})
						o.logger.Debug("stale check: download still active, resetting timer",
							"workflow_id", wf.ID)
						continue
					}
				}
			}

			// For downloading state, only fail if the item is gone AND silent
			// for longer than the dedicated downloading threshold. This avoids
			// false-positives for large files on slow connections where progress
			// events may be temporarily absent (client restart, network blip).
			if wf.State == StateDownloading {
				if age < DownloadingStaleThreshold {
					// Still within tolerance — reset the timer so this workflow
					// doesn't immediately re-appear as stale next tick.
					_ = o.store.MergeMetadata(ctx, wf.ID, map[string]any{"stale_check": "within_threshold"})
					o.logger.Debug("stale check: downloading within threshold, extending",
						"workflow_id", wf.ID, "age", age.Round(time.Minute))
					continue
				}
				// Beyond threshold AND item not found in any client — genuinely stale.
				o.logger.Warn("stale workflow: download silent beyond threshold",
					"id", wf.ID, "age", age.Round(time.Minute))
			}
		}

		o.logger.Warn("stale workflow detected",
			"id", wf.ID, "state", wf.State, "age", age.Round(time.Second))

		o.logEvent(ctx, wf.ID, EventStaleDetected,
			fmt.Sprintf("Stale in %s state for %s", wf.State, age.Round(time.Second)),
			map[string]any{"state": wf.State, "age_seconds": int(age.Seconds())})

		err := o.engine.markFailed(ctx, wf.ID,
			fmt.Sprintf("Stale in %s state for %s", wf.State, age.Round(time.Second)))
		if err != nil {
			o.logger.Error("failed to handle stale workflow", "id", wf.ID, "error", err)
		}
	}
}

func (o *Orchestrator) pruneCompleted(ctx context.Context) {
	pruned, err := o.store.PruneCompleted(ctx, CompletedTTL)
	if err != nil {
		o.logger.Error("prune failed", "error", err)
		return
	}
	if pruned > 0 {
		o.logger.Info("pruned completed workflows", "count", pruned)
	}
}

// ── Startup reconciliation ────────────────────────────────────────────

func (o *Orchestrator) reconcileOnBoot(ctx context.Context) {
	if o.dlStatus == nil {
		o.logger.Debug("no download status provider, skipping startup reconciliation")
		return
	}

	o.logger.Info("running startup reconciliation")

	downloads, err := o.dlStatus.ActiveDownloads(ctx)
	if err != nil {
		o.logger.Error("reconcile: failed to query download state", "error", err)
		return
	}

	// Phase 1: Reconcile active (non-terminal) workflows
	active, err := o.store.ListActive(ctx)
	if err != nil {
		o.logger.Error("reconcile: failed to list active workflows", "error", err)
		return
	}

	reconciled := 0
	for _, wf := range active {
		if wf.DownloadClientID == "" || wf.DownloadID == "" {
			continue
		}

		key := wf.DownloadClientID + ":" + wf.DownloadID
		info, exists := downloads[key]

		switch {
		case wf.State == StateImporting:
			// Import goroutine was killed on restart — re-dispatch
			o.logger.Info("reconcile: re-dispatching import for interrupted workflow",
				"workflow_id", wf.ID)
			category := o.categoryFromMetadata(wf.Metadata)
			o.logEvent(ctx, wf.ID, EventImportStarted, "Re-dispatching import after restart", nil)
			o.dispatchImport(ctx, wf.ID, wf.DownloadClientID, wf.DownloadID, wf.GrabTitle, category)
			reconciled++

		case wf.State == StateCleaningUp:
			// Cleanup goroutine was killed on restart — re-run it.
			o.logger.Info("reconcile: re-running post-import cleanup for interrupted workflow",
				"workflow_id", wf.ID)
			o.logEvent(ctx, wf.ID, EventCleanupStarted, "Re-running cleanup after restart", nil)
			go o.runPostImportCleanup(ctx, wf, nil)
			reconciled++

		case !exists && wf.State == StateDownloading:
			// Download disappeared while we were down — treat it as completed
			// and attempt to import. The download may have finished and been
			// removed from the client (e.g. client auto-removed after seeding),
			// or the import path is cached in metadata from a prior run.
			o.logger.Info("reconcile: downloading workflow missing from client, treating as completed",
				"workflow_id", wf.ID)
			o.logEvent(ctx, wf.ID, EventDownloadComplete,
				"Download not found at startup — treating as completed", nil)
			o.handleDownloadComplete(ctx, CmdDownloadComplete{
				ClientID:   wf.DownloadClientID,
				DownloadID: wf.DownloadID,
				Title:      wf.GrabTitle,
			})
			reconciled++
		case !exists && wf.State == StatePostDownload:
			// Download removed after seeding — ready to import
			o.logger.Info("reconcile: post_download item no longer in client, importing",
				"workflow_id", wf.ID)
			o.transitionToImport(ctx, wf)
			reconciled++
		case exists && (info.Status == "completed" || info.Status == "seeding"):
			// Download completed while we were down
			switch wf.State {
			case StateGrabbed, StateDownloading:
				o.logger.Info("reconcile: recovering completed download",
					"workflow_id", wf.ID, "state", wf.State)
				o.handleDownloadComplete(ctx, CmdDownloadComplete{
					ClientID:    wf.DownloadClientID,
					DownloadID:  wf.DownloadID,
					Title:       wf.GrabTitle,
					ContentPath: info.ContentPath,
					SavePath:    info.SavePath,
				})
				reconciled++
			case StatePostDownload:
				// Re-evaluate seed requirements with current data
				var ratio float64
				if wf.Metadata != "" {
					var m map[string]any
					if json.Unmarshal([]byte(wf.Metadata), &m) == nil {
						if r, ok := m["ratio"].(float64); ok {
							ratio = r
						}
					}
				}
				o.evaluatePostDownload(ctx, wf, ratio, info.Status)
				reconciled++
			}
		case exists && info.Status == "downloading" && wf.State == StateGrabbed:
			_ = o.engine.markDownloading(ctx, wf.ID)
			o.logEvent(ctx, wf.ID, EventDownloading, "Reconciled: download active", nil)
			reconciled++
		}
	}

	// Phase 2: Recover recently-failed workflows that have completed downloads
	failed, err := o.store.ListRecentlyFailed(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		o.logger.Error("reconcile: failed to list recently failed workflows", "error", err)
	} else {
		for _, wf := range failed {
			if wf.DownloadClientID == "" || wf.DownloadID == "" {
				continue
			}
			key := wf.DownloadClientID + ":" + wf.DownloadID
			info, exists := downloads[key]
			if !exists {
				continue
			}

			switch info.Status {
			case "completed":
				o.logger.Info("reconcile: recovering failed workflow with completed download",
					"workflow_id", wf.ID)
				if err := o.engine.RecoverToImporting(ctx, wf.ID, "Boot recovery: download complete, re-importing"); err != nil {
					o.logger.Error("reconcile: failed to recover to importing", "workflow_id", wf.ID, "error", err)
					continue
				}
				o.logEvent(ctx, wf.ID, EventRetried, "Boot recovery: re-importing", nil)
				category := o.categoryFromMetadata(wf.Metadata)
				o.dispatchImport(ctx, wf.ID, wf.DownloadClientID, wf.DownloadID, wf.GrabTitle, category)
				reconciled++

			case "seeding":
				o.logger.Info("reconcile: recovering failed workflow with seeding download",
					"workflow_id", wf.ID)
				if err := o.engine.RecoverToPostDownload(ctx, wf.ID, "Boot recovery: download seeding, evaluating"); err != nil {
					o.logger.Error("reconcile: failed to recover to post_download", "workflow_id", wf.ID, "error", err)
					continue
				}
				o.logEvent(ctx, wf.ID, EventRetried, "Boot recovery: evaluating seed status", nil)
				reconciled++
			}
		}
	}

	if reconciled > 0 {
		o.logger.Info("startup reconciliation complete", "reconciled", reconciled)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

func (o *Orchestrator) logEvent(ctx context.Context, workflowID, eventType, message string, meta map[string]any) {
	var metadata string
	if meta != nil {
		b, _ := json.Marshal(meta)
		metadata = string(b)
	}
	if err := o.store.LogEvent(ctx, workflowID, eventType, message, metadata); err != nil {
		o.logger.Error("failed to log workflow event",
			"workflow_id", workflowID, "event_type", eventType, "error", err)
	}
}
