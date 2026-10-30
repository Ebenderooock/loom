package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// StallAction defines what to do when a download stalls.
type StallAction string

const (
	StallActionPause              StallAction = "pause"
	StallActionRemove             StallAction = "remove"
	StallActionRemoveAndBlocklist StallAction = "remove_and_blocklist"
	StallActionRetry              StallAction = "retry"
)

// StallHandlerOptions wires a StallHandler.
type StallHandlerOptions struct {
	Registry   *Registry
	Blocklist  *BlocklistStore
	Bus        eventbus.Bus
	Logger     *slog.Logger
	Clock      Clock
	Action     StallAction
	MaxRetries int
}

// StallHandler executes the configured action when a download stalls.
//
// Every action except pause abandons the release: it is removed from the
// client and added to the blocklist so automatic search never grabs it again.
type StallHandler struct {
	registry   *Registry
	blocklist  *BlocklistStore
	bus        eventbus.Bus
	logger     *slog.Logger
	clock      Clock
	action     StallAction
	maxRetries int

	mu sync.Mutex
	// retryCount tracks per-title retry attempts in memory (retry action).
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

// Handle processes a stalled or failed item. It returns true when the
// download was abandoned (removed and blocklisted), meaning the owning
// workflow should be failed and a replacement searched for. It returns false
// for the pause action, which leaves the download in place.
func (h *StallHandler) Handle(ctx context.Context, item Item, reason string) bool {
	h.logger.WarnContext(ctx, "stall detected",
		"item_id", item.ID,
		"client_id", item.ClientID,
		"title", item.Title,
		"reason", reason,
		"action", string(h.action),
	)

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
		return false
	case StallActionRemove, StallActionRemoveAndBlocklist:
		h.doRemoveAndBlocklist(ctx, item, reason)
		return true
	case StallActionRetry:
		h.doRetry(ctx, item, reason)
		return true
	default:
		h.logger.ErrorContext(ctx, "unknown stall action, abandoning download", "action", string(h.action))
		h.doRemoveAndBlocklist(ctx, item, reason)
		return true
	}
}

func (h *StallHandler) client(item Item) (DownloadClient, bool) {
	if h.registry == nil {
		return nil, false
	}
	return h.registry.Get(item.ClientID)
}

func (h *StallHandler) doPause(ctx context.Context, item Item) {
	c, ok := h.client(item)
	if !ok {
		h.logger.WarnContext(ctx, "cannot pause stalled download: client not found",
			"item_id", item.ID, "client_id", item.ClientID)
		return
	}
	if err := c.Pause(ctx, item.ID); err != nil {
		h.logger.WarnContext(ctx, "could not pause stalled download",
			"item_id", item.ID, "client_id", item.ClientID, "error", err)
		return
	}
	h.logger.DebugContext(ctx, "paused stalled download", "item_id", item.ID)
}

func (h *StallHandler) doRemove(ctx context.Context, item Item, deleteFiles bool) {
	c, ok := h.client(item)
	if !ok {
		h.logger.WarnContext(ctx, "cannot remove stalled download: client not found",
			"item_id", item.ID, "client_id", item.ClientID)
		return
	}
	if err := c.Remove(ctx, []string{item.ID}, deleteFiles); err != nil {
		h.logger.WarnContext(ctx, "could not remove stalled download",
			"item_id", item.ID, "client_id", item.ClientID, "error", err)
		return
	}
	h.logger.DebugContext(ctx, "removed stalled download", "item_id", item.ID, "delete_files", deleteFiles)
}

func (h *StallHandler) doRemoveAndBlocklist(ctx context.Context, item Item, reason string) {
	h.doRemove(ctx, item, true)
	h.addToBlocklist(ctx, item, reason)
}

// addToBlocklist records the release so it is never grabbed again. Torrents
// are keyed by infohash (stable across indexers); other downloads by a hash
// of the release title.
func (h *StallHandler) addToBlocklist(ctx context.Context, item Item, reason string) {
	if h.blocklist == nil {
		return
	}
	hash := ReleaseHash(item.Title, item.ID)
	entry := BlocklistEntry{
		ID:          fmt.Sprintf("bl-%s", hash[:12]),
		Title:       item.Title,
		ReleaseHash: hash,
		Reason:      reason,
		CreatedAt:   h.clock.Now(),
	}
	if err := h.blocklist.Add(ctx, entry); err != nil {
		h.logger.ErrorContext(ctx, "failed to add to blocklist", "item_id", item.ID, "error", err)
		return
	}
	h.logger.InfoContext(ctx, "blocklisted stalled release", "item_id", item.ID, "title", item.Title)
}

func (h *StallHandler) doRetry(ctx context.Context, item Item, reason string) {
	h.mu.Lock()
	h.retryCount[item.Title]++
	attempt := h.retryCount[item.Title]
	if attempt > h.maxRetries {
		delete(h.retryCount, item.Title)
	}
	h.mu.Unlock()

	// The stalled release itself is never retried — it is blocklisted and a
	// different release is searched for.
	h.doRemoveAndBlocklist(ctx, item, reason)

	if attempt > h.maxRetries {
		h.logger.WarnContext(ctx, "max retries exceeded for stalled download",
			"item_id", item.ID, "retries", attempt-1)
		return
	}
	if h.bus != nil {
		_ = h.bus.Publish(ctx, &DownloadRetryEvent{
			Title:     item.Title,
			Category:  item.Category,
			Reason:    reason,
			Attempt:   attempt,
			RetriedAt: h.clock.Now(),
		})
	}
}
