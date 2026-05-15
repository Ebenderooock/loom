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
func (o *Orchestrator) NotifyDownloadProgress(clientID, downloadID string, progress float64, downSpeed, upSpeed int64, ratio float64, status string) {
	o.Send(CmdDownloadProgress{
		ClientID:   clientID,
		DownloadID: downloadID,
		Progress:   progress,
		DownSpeed:  downSpeed,
		UpSpeed:    upSpeed,
		Ratio:      ratio,
		Status:     status,
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

	// Store category in metadata for later import dispatch.
	if cmd.Category != "" {
		_ = o.store.MergeMetadata(ctx, wf.ID, map[string]any{"category": cmd.Category})
	}

	o.logEvent(ctx, wf.ID, EventPostDownloadStart, "Post-download phase started", map[string]any{
		"settling_delay_sec":     policy.SettlingDelay,
		"seed_ratio_limit":      policy.SeedRatioLimit,
		"seed_time_limit_min":   policy.SeedTimeLimitMinutes,
	})
}

func (o *Orchestrator) handleImportResult(ctx context.Context, cmd CmdImportResult) {
	if cmd.Success {
		msg := "Import completed successfully"
		meta := map[string]any{"imported_paths": cmd.ImportedPaths}
		o.logEvent(ctx, cmd.WorkflowID, EventImportSuccess, msg, meta)

		if err := o.engine.markCompleted(ctx, cmd.WorkflowID, msg); err != nil {
			o.logger.Error("failed to mark completed", "workflow_id", cmd.WorkflowID, "error", err)
		}
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

		o.logger.Info("scheduling import retry after delay",
			"workflow_id", cmd.WorkflowID, "delay", importRetryDelay, "error", cmd.Error)

		// Extract category from metadata if available.
		category := o.categoryFromMetadata(wf.Metadata)

		time.AfterFunc(importRetryDelay, func() {
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

// classifyImportError determines the retry strategy based on the error message.
func (o *Orchestrator) classifyImportError(errMsg string) importRetryStrategy {
	lower := strings.ToLower(errMsg)

	// Non-retryable errors — fail immediately.
	for _, s := range []string{"permission denied", "access denied", "unauthorized"} {
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
	err := o.engine.Retry(ctx, cmd.WorkflowID)
	if err == nil {
		o.logEvent(ctx, cmd.WorkflowID, EventRetried, "Manual retry", nil)
	}
	if cmd.Reply != nil {
		cmd.Reply <- err
	}
}

// ── Progress coalescing ───────────────────────────────────────────────

func (o *Orchestrator) bufferProgress(ctx context.Context, cmd CmdDownloadProgress) {
	// For workflows in post_download, evaluate immediately instead of
	// buffering — seed/settling checks need prompt evaluation.
	wf, err := o.engine.FindByDownload(ctx, cmd.ClientID, cmd.DownloadID)
	if err == nil && wf != nil && wf.State == StatePostDownload {
		_ = o.store.MergeMetadata(ctx, wf.ID, map[string]any{
			"ratio":  cmd.Ratio,
			"status": cmd.Status,
		})
		o.evaluatePostDownload(ctx, wf, cmd.Ratio, cmd.Status)
		return
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

		// Ensure we're in downloading state
		if wf.State == StateGrabbed {
			_ = o.engine.markDownloading(ctx, wf.ID)
			o.logEvent(ctx, wf.ID, EventDownloading, "Download confirmed active", nil)
		}

		// Update metadata with progress info (merge to preserve seed policy)
		_ = o.store.MergeMetadata(ctx, wf.ID, map[string]any{
			"progress":   p.Progress,
			"down_speed": p.DownSpeed,
			"up_speed":   p.UpSpeed,
			"ratio":      p.Ratio,
			"status":     p.Status,
		})

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
		case !exists && wf.State == StatePostDownload:
			// Download removed after seeding — ready to import
			o.logger.Info("reconcile: post_download item no longer in client, importing",
				"workflow_id", wf.ID)
			o.transitionToImport(ctx, wf)
			reconciled++
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
			} else if wf.State == StatePostDownload {
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
				o.evaluatePostDownload(ctx, wf, ratio, dlState)
				reconciled++
			}
		case exists && dlState == "downloading" && wf.State == StateGrabbed:
			_ = o.engine.markDownloading(ctx, wf.ID)
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
