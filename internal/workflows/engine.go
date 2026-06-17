package workflows

import (
	"context"
	"fmt"
	"log/slog"
)

// MediaStatusUpdater abstracts movie/episode status updates.
type MediaStatusUpdater interface {
	SetMovieDownloading(ctx context.Context, movieID string) error
	SetMovieMissing(ctx context.Context, movieID string) error
	SetEpisodeDownloading(ctx context.Context, episodeID string) error
	SetEpisodeMissing(ctx context.Context, episodeID string) error
}

// Engine orchestrates workflow state transitions and retry logic.
type Engine struct {
	store  *Store
	media  MediaStatusUpdater
	logger *slog.Logger
}

// NewEngine creates a workflow engine.
func NewEngine(store *Store, media MediaStatusUpdater, logger *slog.Logger) *Engine {
	return &Engine{
		store:  store,
		media:  media,
		logger: logger.With("component", "workflow-engine"),
	}
}

// Store returns the underlying store for direct queries.
func (e *Engine) Store() *Store { return e.store }

// StartSearch creates a new workflow when an auto-search begins.
func (e *Engine) StartSearch(ctx context.Context, wfType, mediaType, qualityProfileID string, mediaIDs []string) (*Workflow, error) {
	// Check for existing active workflows for any of these media items
	for _, id := range mediaIDs {
		existing, err := e.store.FindActiveForMedia(ctx, mediaType, id)
		if err != nil {
			return nil, fmt.Errorf("check active: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("active workflow %s already exists for %s %s (state: %s)",
				existing.ID, mediaType, id, existing.State)
		}
	}

	items := make([]Item, len(mediaIDs))
	for i, id := range mediaIDs {
		items[i] = Item{MediaType: mediaType, MediaID: id}
	}

	wf := &Workflow{
		Type:             wfType,
		State:            StateSearching,
		MediaType:        mediaType,
		QualityProfileID: qualityProfileID,
		MaxRetries:       MaxRetries,
		Items:            items,
	}

	if err := e.store.Create(ctx, wf); err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	ctx = WithWorkflowID(ctx, wf.ID)
	e.logger.Info("workflow started",
		"id", wf.ID, "type", wfType, "media_type", mediaType,
		"media_ids", mediaIDs)
	return wf, nil
}

// markGrabbed transitions a workflow from searching → grabbed and records download info.
// If another active workflow already tracks the same download (e.g. two episode searches
// grabbed the same season pack), the current workflow's items are merged into the existing
// one and the current workflow is cancelled to avoid a UNIQUE constraint violation.
func (e *Engine) markGrabbed(ctx context.Context, workflowID, clientID, downloadID, title string) error {
	ctx = WithWorkflowID(ctx, workflowID)

	// Check if another workflow already tracks this download.
	existing, err := e.store.FindByDownload(ctx, clientID, downloadID)
	if err != nil {
		return fmt.Errorf("find existing download workflow: %w", err)
	}
	if existing != nil && existing.ID != workflowID {
		// Merge our items into the existing workflow and cancel this one.
		thisWf, err := e.store.Get(ctx, workflowID)
		if err != nil {
			return fmt.Errorf("get workflow for merge: %w", err)
		}
		if err := e.store.MergeItems(ctx, existing.ID, thisWf.Items); err != nil {
			return fmt.Errorf("merge items: %w", err)
		}
		// Cancel the duplicate workflow.
		e.store.Transition(ctx, workflowID, StateSearching, StateCancelled,
			"Merged into workflow "+existing.ID+" (same download)")
		e.logger.Info("workflow merged into existing",
			"merged_id", workflowID, "target_id", existing.ID, "title", title)
		return nil
	}

	ok, err := e.store.Transition(ctx, workflowID, StateSearching, StateGrabbed, "Release grabbed: "+title)
	if err != nil {
		return fmt.Errorf("transition to grabbed: %w", err)
	}
	if !ok {
		return fmt.Errorf("workflow %s not in searching state", workflowID)
	}

	if err := e.store.SetDownload(ctx, workflowID, clientID, downloadID, title); err != nil {
		return fmt.Errorf("set download: %w", err)
	}

	// Update media status to downloading
	wf, err := e.store.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	for _, item := range wf.Items {
		if item.MediaType == MediaTypeMovie {
			if err := e.media.SetMovieDownloading(ctx, item.MediaID); err != nil {
				e.logger.Warn("failed to set movie downloading", "movie_id", item.MediaID, "error", err)
			}
		} else if item.MediaType == MediaTypeEpisode {
			if err := e.media.SetEpisodeDownloading(ctx, item.MediaID); err != nil {
				e.logger.Warn("failed to set episode downloading", "episode_id", item.MediaID, "error", err)
			}
		}
	}

	e.logger.Info("workflow grabbed", "id", workflowID, "title", title)
	return nil
}

// markDownloading transitions from grabbed → downloading (download client confirmed).
func (e *Engine) markDownloading(ctx context.Context, workflowID string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	ok, err := e.store.Transition(ctx, workflowID, StateGrabbed, StateDownloading, "Download started")
	if err != nil {
		return err
	}
	if !ok {
		// May already be downloading (idempotent)
		return nil
	}
	e.logger.Info("workflow downloading", "id", workflowID)
	return nil
}

// markPostDownload transitions from downloading → post_download (download finished, awaiting seed/settle).
func (e *Engine) markPostDownload(ctx context.Context, workflowID string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	ok, err := e.store.Transition(ctx, workflowID, StateDownloading, StatePostDownload, "Download complete, post-download phase")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s not in downloading state", workflowID)
	}
	e.logger.Info("workflow post_download", "id", workflowID)
	return nil
}

// markImporting transitions from post_download → importing.
func (e *Engine) markImporting(ctx context.Context, workflowID string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	ok, err := e.store.Transition(ctx, workflowID, StatePostDownload, StateImporting, "Post-download complete, importing")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s not in post_download state", workflowID)
	}
	e.logger.Info("workflow importing", "id", workflowID)
	return nil
}

// markCompleted transitions to completed (import successful).
func (e *Engine) markCompleted(ctx context.Context, workflowID, message string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	// Try from cleaning_up first (normal path after cleanup phase)
	ok, err := e.store.Transition(ctx, workflowID, StateCleaningUp, StateCompleted, message)
	if err != nil {
		return err
	}
	if ok {
		e.logger.Info("workflow completed", "id", workflowID)
		return nil
	}

	// Try from importing (skipped cleanup path)
	ok, err = e.store.Transition(ctx, workflowID, StateImporting, StateCompleted, message)
	if err != nil {
		return err
	}
	if ok {
		e.logger.Info("workflow completed", "id", workflowID)
		return nil
	}

	// Fallback: recover from failed state (stale detection raced with import)
	ok, err = e.store.Transition(ctx, workflowID, StateFailed, StateCompleted, message)
	if err != nil {
		return err
	}
	if ok {
		e.logger.Info("workflow completed (recovered from failed)", "id", workflowID)
		return nil
	}

	return fmt.Errorf("workflow %s not in cleaning_up, importing, or failed state", workflowID)
}

// markCleaningUp transitions from importing to cleaning_up.
func (e *Engine) markCleaningUp(ctx context.Context, workflowID, message string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	ok, err := e.store.Transition(ctx, workflowID, StateImporting, StateCleaningUp, message)
	if err != nil {
		return err
	}
	if ok {
		e.logger.Info("workflow cleaning up", "id", workflowID)
		return nil
	}
	return fmt.Errorf("workflow %s not in importing state for cleanup transition", workflowID)
}

// markFailed records failure and either retries or marks as permanently failed.
func (e *Engine) markFailed(ctx context.Context, workflowID, errMsg string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	wf, err := e.store.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	if wf.IsTerminal() {
		return nil // already done
	}

	retryCount, err := e.store.IncrementRetry(ctx, workflowID, errMsg)
	if err != nil {
		return err
	}

	if retryCount >= wf.MaxRetries {
		// Exhausted retries — permanent failure
		ok, err := e.store.Transition(ctx, workflowID, wf.State, StateFailed,
			fmt.Sprintf("Failed after %d retries: %s", retryCount, errMsg))
		if err != nil {
			return err
		}
		if ok {
			// Reset media status to missing
			e.resetMediaStatus(ctx, wf)
		}
		e.logger.Warn("workflow failed permanently", "id", workflowID, "error", errMsg)
		return nil
	}

	// Determine retry target state based on where it failed
	retryState := e.retryTargetState(wf.State)
	ok, transErr := e.store.Transition(ctx, workflowID, wf.State, retryState,
		fmt.Sprintf("Retry %d/%d: %s", retryCount, wf.MaxRetries, errMsg))
	if transErr != nil {
		return transErr
	}
	if ok {
		e.logger.Info("workflow retrying",
			"id", workflowID, "from", wf.State, "to", retryState,
			"retry", retryCount, "max", wf.MaxRetries)
	}
	return nil
}

// Cancel cancels an active workflow.
func (e *Engine) Cancel(ctx context.Context, workflowID string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	wf, err := e.store.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	if wf.IsTerminal() {
		return fmt.Errorf("workflow %s already in terminal state: %s", workflowID, wf.State)
	}

	ok, err := e.store.Transition(ctx, workflowID, wf.State, StateCancelled, "Cancelled by user")
	if err != nil {
		return err
	}
	if ok {
		e.resetMediaStatus(ctx, wf)
	}
	e.logger.Info("workflow cancelled", "id", workflowID)
	return nil
}

// Retry restarts a failed workflow from searching (no download available).
func (e *Engine) Retry(ctx context.Context, workflowID string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	wf, err := e.store.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	if wf.State != StateFailed {
		return fmt.Errorf("can only retry failed workflows, current state: %s", wf.State)
	}

	if err := e.store.ResetRetry(ctx, workflowID); err != nil {
		return fmt.Errorf("reset retry count: %w", err)
	}

	ok, err := e.store.Transition(ctx, workflowID, StateFailed, StateSearching, "Manual retry by user (re-search)")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s transition failed", workflowID)
	}
	e.logger.Info("workflow retried manually (re-search)", "id", workflowID)
	return nil
}

// RecoverToImporting transitions a failed workflow directly to importing.
// Used when the download is already complete and only the import needs to run.
func (e *Engine) RecoverToImporting(ctx context.Context, workflowID, reason string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	if err := e.store.ResetRetry(ctx, workflowID); err != nil {
		return fmt.Errorf("reset retry count: %w", err)
	}
	ok, err := e.store.Transition(ctx, workflowID, StateFailed, StateImporting, reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s not in failed state", workflowID)
	}
	e.logger.Info("workflow recovered to importing", "id", workflowID, "reason", reason)
	return nil
}

// RecoverToPostDownload transitions a failed workflow to post_download.
// Used when the download is seeding and seed requirements need re-evaluation.
func (e *Engine) RecoverToPostDownload(ctx context.Context, workflowID, reason string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	if err := e.store.ResetRetry(ctx, workflowID); err != nil {
		return fmt.Errorf("reset retry count: %w", err)
	}
	ok, err := e.store.Transition(ctx, workflowID, StateFailed, StatePostDownload, reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s not in failed state", workflowID)
	}
	e.logger.Info("workflow recovered to post_download", "id", workflowID, "reason", reason)
	return nil
}

// RecoverToDownloading transitions a failed workflow to downloading.
// Used when the download is still active.
func (e *Engine) RecoverToDownloading(ctx context.Context, workflowID, reason string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	if err := e.store.ResetRetry(ctx, workflowID); err != nil {
		return fmt.Errorf("reset retry count: %w", err)
	}
	ok, err := e.store.Transition(ctx, workflowID, StateFailed, StateDownloading, reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s not in failed state", workflowID)
	}
	e.logger.Info("workflow recovered to downloading", "id", workflowID, "reason", reason)
	return nil
}

// StartImport creates a workflow for a manual import operation.
// The workflow starts directly in "importing" state, skipping search/download phases.
func (e *Engine) StartImport(ctx context.Context, mediaType string, mediaIDs []string, grabTitle string) (*Workflow, error) {
	items := make([]Item, len(mediaIDs))
	for i, id := range mediaIDs {
		items[i] = Item{MediaType: mediaType, MediaID: id}
	}

	wf := &Workflow{
		Type:       TypeManualImport,
		State:      StateImporting,
		MediaType:  mediaType,
		GrabTitle:  grabTitle,
		MaxRetries: 0,
		Items:      items,
	}

	if err := e.store.Create(ctx, wf); err != nil {
		return nil, fmt.Errorf("create import workflow: %w", err)
	}

	ctx = WithWorkflowID(ctx, wf.ID)
	e.logger.Info("import workflow started",
		"id", wf.ID, "media_type", mediaType, "grab_title", grabTitle)
	return wf, nil
}

// CompleteImport transitions an import workflow to completed.
func (e *Engine) CompleteImport(ctx context.Context, workflowID string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	ok, err := e.store.Transition(ctx, workflowID, StateImporting, StateCompleted, "Import completed")
	if err != nil {
		return err
	}
	if !ok {
		e.logger.Warn("import workflow not in importing state", "id", workflowID)
	}
	return nil
}

// FailImport transitions an import workflow to failed.
func (e *Engine) FailImport(ctx context.Context, workflowID, reason string) error {
	ctx = WithWorkflowID(ctx, workflowID)
	ok, err := e.store.Transition(ctx, workflowID, StateImporting, StateFailed, reason)
	if err != nil {
		return err
	}
	if !ok {
		e.logger.Warn("import workflow not in importing state", "id", workflowID)
	}
	return nil
}

// FindByDownload finds a workflow by download client + download ID.
func (e *Engine) FindByDownload(ctx context.Context, clientID, downloadID string) (*Workflow, error) {
	return e.store.FindByDownload(ctx, clientID, downloadID)
}

// retryTargetState determines where to restart based on where the failure occurred.
func (e *Engine) retryTargetState(failedState string) string {
	switch failedState {
	case StateSearching, StateGrabbed:
		return StateSearching // re-search from scratch
	case StateDownloading:
		return StateSearching // bad release, search again
	case StateImporting:
		return StateImporting // retry import (files may still be there)
	default:
		return StateSearching
	}
}

// resetMediaStatus sets all workflow media items back to missing.
func (e *Engine) resetMediaStatus(ctx context.Context, wf *Workflow) {
	for _, item := range wf.Items {
		if item.MediaType == MediaTypeMovie {
			if err := e.media.SetMovieMissing(ctx, item.MediaID); err != nil {
				e.logger.Warn("failed to reset movie status", "movie_id", item.MediaID, "error", err)
			}
		} else if item.MediaType == MediaTypeEpisode {
			if err := e.media.SetEpisodeMissing(ctx, item.MediaID); err != nil {
				e.logger.Warn("failed to reset episode status", "episode_id", item.MediaID, "error", err)
			}
		}
	}
}
