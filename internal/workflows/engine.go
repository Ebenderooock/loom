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

	e.logger.Info("workflow started",
		"id", wf.ID, "type", wfType, "media_type", mediaType,
		"media_ids", mediaIDs)
	return wf, nil
}

// markGrabbed transitions a workflow from searching → grabbed and records download info.
func (e *Engine) markGrabbed(ctx context.Context, workflowID, clientID, downloadID, title string) error {
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
		}
	}

	e.logger.Info("workflow grabbed", "id", workflowID, "title", title)
	return nil
}

// markDownloading transitions from grabbed → downloading (download client confirmed).
func (e *Engine) markDownloading(ctx context.Context, workflowID string) error {
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

// markImporting transitions from downloading → importing.
func (e *Engine) markImporting(ctx context.Context, workflowID string) error {
	ok, err := e.store.Transition(ctx, workflowID, StateDownloading, StateImporting, "Download complete, importing")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s not in downloading state", workflowID)
	}
	e.logger.Info("workflow importing", "id", workflowID)
	return nil
}

// markCompleted transitions to completed (import successful).
func (e *Engine) markCompleted(ctx context.Context, workflowID, message string) error {
	// Try from importing first, but also allow from downloading (fast imports)
	ok, err := e.store.Transition(ctx, workflowID, StateImporting, StateCompleted, message)
	if err != nil {
		return err
	}
	if !ok {
		// Try from downloading directly (some imports happen immediately)
		ok, err = e.store.Transition(ctx, workflowID, StateDownloading, StateCompleted, message)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("workflow %s not in importing/downloading state", workflowID)
		}
	}
	e.logger.Info("workflow completed", "id", workflowID)
	return nil
}

// markFailed records failure and either retries or marks as permanently failed.
func (e *Engine) markFailed(ctx context.Context, workflowID, errMsg string) error {
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

// Retry restarts a failed workflow from the appropriate state.
func (e *Engine) Retry(ctx context.Context, workflowID string) error {
	wf, err := e.store.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	if wf.State != StateFailed {
		return fmt.Errorf("can only retry failed workflows, current state: %s", wf.State)
	}

	// Reset retry count and restart from searching
	retryState := StateSearching
	ok, err := e.store.Transition(ctx, workflowID, StateFailed, retryState, "Manual retry by user")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow %s transition failed", workflowID)
	}
	e.logger.Info("workflow retried manually", "id", workflowID)
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
		}
	}
}
