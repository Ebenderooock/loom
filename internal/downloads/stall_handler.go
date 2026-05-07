package downloads

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// StallAction defines what to do when a download stalls.
type StallAction string

const (
	StallActionPause             StallAction = "pause"
	StallActionRemove            StallAction = "remove"
	StallActionRemoveAndBlocklist StallAction = "remove_and_blocklist"
	StallActionRetry             StallAction = "retry"
)

// StallHandlerOptions wires a StallHandler.
type StallHandlerOptions struct {
	Registry       *Registry
	Blocklist      *BlocklistStore
	Bus            eventbus.Bus
	Logger         *slog.Logger
	Clock          Clock
	Action         StallAction
	MaxRetries     int
}

// StallHandler executes the configured action when a download stalls.
type StallHandler struct {
	registry   *Registry
	blocklist  *BlocklistStore
	bus        eventbus.Bus
	logger     *slog.Logger
	clock      Clock
	action     StallAction
	maxRetries int

	// retryCount tracks per-item retry attempts in memory.
	retryCount map[string]int
}

// NewStallHandler returns a wired handler.
func NewStallHandler(opts StallHandlerOptions) *StallHandler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.Action == "" {
		opts.Action = StallActionRemove
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	return &StallHandler{
		registry:   opts.Registry,
		blocklist:  opts.Blocklist,
		bus:        opts.Bus,
		logger:     opts.Logger.With("module", "downloads/stall-handler"),
		clock:      opts.Clock,
		action:     opts.Action,
		maxRetries: opts.MaxRetries,
		retryCount: make(map[string]int),
	}
}

// Handle processes a stalled or failed item.
func (h *StallHandler) Handle(ctx context.Context, item Item, reason string) {
	h.logger.Warn("stall detected",
		"item_id", item.ID,
		"title", item.Title,
		"reason", reason,
		"action", string(h.action),
	)

	// Emit event for notifications.
	if h.bus != nil {
		_ = h.bus.Publish(ctx, &DownloadStalledEvent{
			DownloadID: item.ID,
			Title:      item.Title,
			Reason:     reason,
			Action:     string(h.action),
			StalledAt:  h.clock.Now(),
		})
	}

	switch h.action {
	case StallActionPause:
		h.doPause(ctx, item)
	case StallActionRemove:
		h.doRemove(ctx, item, true)
	case StallActionRemoveAndBlocklist:
		h.doRemoveAndBlocklist(ctx, item, reason)
	case StallActionRetry:
		h.doRetry(ctx, item, reason)
	default:
		h.logger.Error("unknown stall action", "action", string(h.action))
	}
}

func (h *StallHandler) doPause(ctx context.Context, item Item) {
	for _, c := range h.registry.List() {
		if err := c.Pause(ctx, item.ID); err == nil {
			h.logger.Info("paused stalled download", "item_id", item.ID)
			return
		}
	}
	h.logger.Warn("could not pause stalled download on any client", "item_id", item.ID)
}

func (h *StallHandler) doRemove(ctx context.Context, item Item, deleteFiles bool) {
	for _, c := range h.registry.List() {
		if err := c.Remove(ctx, []string{item.ID}, deleteFiles); err == nil {
			h.logger.Info("removed stalled download", "item_id", item.ID, "delete_files", deleteFiles)
			return
		}
	}
	h.logger.Warn("could not remove stalled download on any client", "item_id", item.ID)
}

func (h *StallHandler) doRemoveAndBlocklist(ctx context.Context, item Item, reason string) {
	h.doRemove(ctx, item, true)
	if h.blocklist != nil {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(item.Title)))
		entry := BlocklistEntry{
			ID:          fmt.Sprintf("bl-%s", hash[:12]),
			Title:       item.Title,
			ReleaseHash: hash,
			Reason:      reason,
			CreatedAt:   h.clock.Now(),
		}
		if err := h.blocklist.Add(ctx, entry); err != nil {
			h.logger.Error("failed to add to blocklist", "item_id", item.ID, "err", err)
		} else {
			h.logger.Info("added to blocklist", "item_id", item.ID, "title", item.Title)
		}
	}
}

func (h *StallHandler) doRetry(ctx context.Context, item Item, reason string) {
	h.retryCount[item.ID]++
	if h.retryCount[item.ID] > h.maxRetries {
		h.logger.Warn("max retries exceeded, removing and blocklisting",
			"item_id", item.ID, "retries", h.retryCount[item.ID]-1)
		h.doRemoveAndBlocklist(ctx, item, reason+"; max retries exceeded")
		delete(h.retryCount, item.ID)
		return
	}

	h.logger.Info("retrying stalled download",
		"item_id", item.ID,
		"attempt", h.retryCount[item.ID],
		"max_retries", h.maxRetries,
	)
	h.doRemove(ctx, item, true)

	// Publish a re-search event so upstream can find a replacement.
	if h.bus != nil {
		_ = h.bus.Publish(ctx, &DownloadRetryEvent{
			Title:     item.Title,
			Category:  item.Category,
			Reason:    reason,
			Attempt:   h.retryCount[item.ID],
			RetriedAt: h.clock.Now(),
		})
	}
}
