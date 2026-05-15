package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	commandBufferSize  = 256
	maxConcurrentImports = 2
	progressFlushInterval = 60 * time.Second
)

// ImportFunc is the function signature the orchestrator calls to run an import.
// It receives the workflow ID, download client ID, download ID, title, and category.
// It returns the imported file paths or an error.
type ImportFunc func(ctx context.Context, clientID, downloadID, title, category string) ([]string, error)

// DownloadStatusProvider allows the orchestrator to query current download state
// for startup reconciliation without importing the downloads package.
type DownloadStatusProvider interface {
	// ActiveDownloads returns a map of "clientID:downloadID" → status string
	// for all currently active downloads across all clients.
	ActiveDownloads(ctx context.Context) (map[string]string, error)
}

// OrchestratorOpts configures the Orchestrator.
type OrchestratorOpts struct {
	Store          *Store
	Engine         *Engine
	Logger         *slog.Logger
	ImportFn       ImportFunc
	DownloadStatus DownloadStatusProvider // optional, for startup reconciliation
}

// Orchestrator is the single coordinator for all workflow state transitions.
// It consumes typed commands from a buffered channel and serializes all
// mutations through a single goroutine — eliminating scattered Mark* calls.
type Orchestrator struct {
	store    *Store
	engine   *Engine
	logger   *slog.Logger
	importFn ImportFunc
	dlStatus DownloadStatusProvider

	commands chan Command

	// Import concurrency limiter.
	importSem chan struct{}

	// Progress coalescing: buffered updates flushed on tick.
	progressMu sync.Mutex
	progressBuf map[string]*CmdDownloadProgress // key: clientID:downloadID
}

// Store returns the underlying workflow store for read-only queries
// (e.g. duplicate-active-workflow checks).
func (o *Orchestrator) Store() *Store { return o.store }

// SetImportFn sets the import function after construction (for wiring order flexibility).
func (o *Orchestrator) SetImportFn(fn ImportFunc) { o.importFn = fn }

// NewOrchestrator creates a workflow orchestrator.
func NewOrchestrator(opts OrchestratorOpts) *Orchestrator {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Orchestrator{
		store:       opts.Store,
		engine:      opts.Engine,
		logger:      opts.Logger.With("component", "workflow-orchestrator"),
		importFn:    opts.ImportFn,
		dlStatus:    opts.DownloadStatus,
		commands:    make(chan Command, commandBufferSize),
		importSem:   make(chan struct{}, maxConcurrentImports),
		progressBuf: make(map[string]*CmdDownloadProgress),
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
func (o *Orchestrator) NotifyDownloadComplete(clientID, downloadID, title, category string) {
	o.Send(CmdDownloadComplete{
		ClientID:   clientID,
		DownloadID: downloadID,
		Title:      title,
		Category:   category,
	})
}

// NotifyDownloadProgress satisfies downloads.MonitorOrchNotifier.
func (o *Orchestrator) NotifyDownloadProgress(clientID, downloadID string, progress float64, downSpeed, upSpeed int64) {
	o.Send(CmdDownloadProgress{
		ClientID:   clientID,
		DownloadID: downloadID,
		Progress:   progress,
		DownSpeed:  downSpeed,
		UpSpeed:    upSpeed,
	})
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
		o.bufferProgress(c)
	case CmdDownloadComplete:
		o.handleDownloadComplete(ctx, c)
	case CmdImportResult:
		o.handleImportResult(ctx, c)
	case CmdCancel:
		o.handleCancel(ctx, c)
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
		"type":              cmd.WfType,
		"media_type":        cmd.MediaType,
		"media_ids":         cmd.MediaIDs,
		"quality_profile_id": cmd.QualityProfileID,
	})

	cmd.Reply <- SearchReply{Workflow: wf}
}

func (o *Orchestrator) handleGrabbed(ctx context.Context, cmd CmdGrabbed) {
	if err := o.engine.MarkGrabbed(ctx, cmd.WorkflowID, cmd.ClientID, cmd.DownloadID, cmd.Title); err != nil {
		o.logger.Error("failed to mark grabbed", "workflow_id", cmd.WorkflowID, "error", err)
		o.logEvent(ctx, cmd.WorkflowID, EventFailed, "Failed to mark grabbed: "+err.Error(), nil)
		return
	}

	o.logEvent(ctx, cmd.WorkflowID, EventGrabbed, "Release grabbed: "+cmd.Title, map[string]any{
		"client_id":   cmd.ClientID,
		"download_id": cmd.DownloadID,
		"title":       cmd.Title,
	})

	// Also transition to downloading immediately — the download client has accepted it
	if err := o.engine.MarkDownloading(ctx, cmd.WorkflowID); err != nil {
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

	// Ensure we're at least in downloading state
	if wf.State == StateGrabbed {
		_ = o.engine.MarkDownloading(ctx, wf.ID)
	}

	o.logEvent(ctx, wf.ID, EventDownloadComplete, "Download finished: "+cmd.Title, map[string]any{
		"client_id":   cmd.ClientID,
		"download_id": cmd.DownloadID,
		"title":       cmd.Title,
		"category":    cmd.Category,
	})

	// Transition to importing
	if err := o.engine.MarkImporting(ctx, wf.ID); err != nil {
		o.logger.Error("failed to mark importing", "workflow_id", wf.ID, "error", err)
		o.logEvent(ctx, wf.ID, EventImportFailed, "Failed to start import: "+err.Error(), nil)
		return
	}

	o.logEvent(ctx, wf.ID, EventImportStarted, "Import dispatched", nil)

	// Dispatch import asynchronously via bounded worker pool
	o.dispatchImport(ctx, wf.ID, cmd.ClientID, cmd.DownloadID, cmd.Title, cmd.Category)
}

func (o *Orchestrator) handleImportResult(ctx context.Context, cmd CmdImportResult) {
	if cmd.Success {
		msg := "Import completed successfully"
		meta := map[string]any{"imported_paths": cmd.ImportedPaths}
		o.logEvent(ctx, cmd.WorkflowID, EventImportSuccess, msg, meta)

		if err := o.engine.MarkCompleted(ctx, cmd.WorkflowID, msg); err != nil {
			o.logger.Error("failed to mark completed", "workflow_id", cmd.WorkflowID, "error", err)
		}
	} else {
		o.logEvent(ctx, cmd.WorkflowID, EventImportFailed, "Import failed: "+cmd.Error, map[string]any{
			"error": cmd.Error,
		})

		if err := o.engine.MarkFailed(ctx, cmd.WorkflowID, "Import failed: "+cmd.Error); err != nil {
			o.logger.Error("failed to mark failed", "workflow_id", cmd.WorkflowID, "error", err)
		}
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
	err := o.engine.Retry(ctx, cmd.WorkflowID)
	if err == nil {
		o.logEvent(ctx, cmd.WorkflowID, EventRetried, "Manual retry", nil)
	}
	if cmd.Reply != nil {
		cmd.Reply <- err
	}
}

// ── Progress coalescing ───────────────────────────────────────────────

func (o *Orchestrator) bufferProgress(cmd CmdDownloadProgress) {
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

		// Ensure we're in downloading state
		if wf.State == StateGrabbed {
			_ = o.engine.MarkDownloading(ctx, wf.ID)
			o.logEvent(ctx, wf.ID, EventDownloading, "Download confirmed active", nil)
		}

		// Update metadata with progress info
		meta := MetadataFromMap(map[string]any{
			"progress":   p.Progress,
			"down_speed": p.DownSpeed,
			"up_speed":   p.UpSpeed,
		})
		_ = o.store.SetMetadata(ctx, wf.ID, meta)
	}
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

	for _, wf := range stale {
		age := time.Since(wf.UpdatedAt)
		o.logger.Warn("stale workflow detected",
			"id", wf.ID, "state", wf.State, "age", age.Round(time.Second))

		o.logEvent(ctx, wf.ID, EventStaleDetected,
			fmt.Sprintf("Stale in %s state for %s", wf.State, age.Round(time.Second)),
			map[string]any{"state": wf.State, "age_seconds": int(age.Seconds())})

		err := o.engine.MarkFailed(ctx, wf.ID,
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

	active, err := o.store.ListActive(ctx)
	if err != nil {
		o.logger.Error("reconcile: failed to list active workflows", "error", err)
		return
	}

	if len(active) == 0 {
		o.logger.Debug("reconcile: no active workflows")
		return
	}

	downloads, err := o.dlStatus.ActiveDownloads(ctx)
	if err != nil {
		o.logger.Error("reconcile: failed to query download state", "error", err)
		return
	}

	reconciled := 0
	for _, wf := range active {
		if wf.DownloadClientID == "" || wf.DownloadID == "" {
			continue
		}

		key := wf.DownloadClientID + ":" + wf.DownloadID
		dlState, exists := downloads[key]

		switch {
		case !exists && wf.State == StateDownloading:
			// Download disappeared — might have completed while we were down
			o.logger.Warn("reconcile: download not found, may have completed",
				"workflow_id", wf.ID, "state", wf.State)
			o.logEvent(ctx, wf.ID, EventStaleDetected,
				"Download not found during startup reconciliation", nil)
		case exists && (dlState == "completed" || dlState == "seeding"):
			// Download completed while we were down
			if wf.State == StateGrabbed || wf.State == StateDownloading {
				o.logger.Info("reconcile: recovering completed download",
					"workflow_id", wf.ID, "state", wf.State)
				o.handleDownloadComplete(ctx, CmdDownloadComplete{
					ClientID:   wf.DownloadClientID,
					DownloadID: wf.DownloadID,
					Title:      wf.GrabTitle,
				})
				reconciled++
			}
		case exists && dlState == "downloading" && wf.State == StateGrabbed:
			_ = o.engine.MarkDownloading(ctx, wf.ID)
			o.logEvent(ctx, wf.ID, EventDownloading, "Reconciled: download active", nil)
			reconciled++
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
