package sabnzbd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Pause asks SABnzbd to pause one or more queued items. Empty ids
// pauses the entire queue (mode=pause without a value), matching
// the interface contract.
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return c.simpleQueueOp(ctx, "pause", "")
	}
	return c.forEachID(ids, func(id string) error {
		return c.simpleQueueOp(ctx, "pause", id)
	})
}

// Resume reverses Pause.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return c.simpleQueueOp(ctx, "resume", "")
	}
	return c.forEachID(ids, func(id string) error {
		return c.simpleQueueOp(ctx, "resume", id)
	})
}

// Remove deletes items. SAB needs different `mode` values for the
// queue and the history, so we try the queue first and fall back to
// the history for any id that wasn't there. deleteFiles is honoured
// by passing del_files=1 on the history delete; the queue delete
// also implicitly removes the partial files it has buffered.
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	if len(ids) == 0 {
		return nil
	}
	missing := make([]string, 0, len(ids))
	for _, id := range ids {
		removed, err := c.queueDelete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			missing = append(missing, id)
		}
	}
	for _, id := range missing {
		if err := c.historyDelete(ctx, id, deleteFiles); err != nil {
			return err
		}
	}
	return nil
}

// queueDelete tries to remove id from the active queue. The bool
// return is true when SAB confirmed the deletion. Items that have
// already moved to history return false with no error.
func (c *Client) queueDelete(ctx context.Context, id string) (bool, error) {
	form := url.Values{}
	form.Set("name", "delete")
	form.Set("value", id)
	target := c.endpoint("queue", form)
	var resp struct {
		Status bool     `json:"status"`
		NzoIDs []string `json:"nzo_ids"`
	}
	if err := c.getJSON(ctx, target, &resp); err != nil {
		return false, err
	}
	if !resp.Status {
		return false, nil
	}
	for _, n := range resp.NzoIDs {
		if n == id {
			return true, nil
		}
	}
	// SAB sometimes returns status:true with empty nzo_ids when the
	// id was already gone; treat that as "not in queue, try history".
	return len(resp.NzoIDs) > 0, nil
}

// historyDelete removes id from the history table. del_files=1
// instructs SAB to also unlink any files it stored on disk.
func (c *Client) historyDelete(ctx context.Context, id string, deleteFiles bool) error {
	form := url.Values{}
	form.Set("name", "delete")
	form.Set("value", id)
	if deleteFiles {
		form.Set("del_files", "1")
	}
	target := c.endpoint("history", form)
	var resp struct {
		Status bool `json:"status"`
	}
	if err := c.getJSON(ctx, target, &resp); err != nil {
		return err
	}
	if !resp.Status {
		return fmt.Errorf("%w: history delete refused for nzo_id %q", ErrNotFound, id)
	}
	return nil
}

// simpleQueueOp posts mode=queue&name=<op>&value=<id>. value is
// optional; when empty SAB applies the op to the entire queue.
func (c *Client) simpleQueueOp(ctx context.Context, op, value string) error {
	form := url.Values{}
	form.Set("name", op)
	if value != "" {
		form.Set("value", value)
	}
	target := c.endpoint("queue", form)
	var resp struct {
		Status bool `json:"status"`
	}
	if err := c.getJSON(ctx, target, &resp); err != nil {
		return err
	}
	if !resp.Status {
		return fmt.Errorf("%w: queue %s refused for value %q", ErrServer, op, value)
	}
	return nil
}

// forEachID runs fn against every id in turn, returning the first
// error. Used by Pause/Resume so a single failing id surfaces with
// context rather than being silently skipped.
func (c *Client) forEachID(ids []string, fn func(string) error) error {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return nil
}

// SetPriority is not supported for SABnzbd.
func (c *Client) SetPriority(_ context.Context, _ downloads.Priority, _ ...string) error {
	return fmt.Errorf("SetPriority not supported for sabnzbd")
}

// SetSpeedLimit is not supported for SABnzbd.
func (c *Client) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error {
	return fmt.Errorf("SetSpeedLimit not supported for sabnzbd")
}

// ForceStart is not supported for SABnzbd.
func (c *Client) ForceStart(_ context.Context, _ ...string) error {
	return fmt.Errorf("ForceStart not supported for sabnzbd")
}

// Recheck is not supported for SABnzbd.
func (c *Client) Recheck(_ context.Context, _ ...string) error {
	return fmt.Errorf("Recheck not supported for sabnzbd")
}

// Reannounce is not supported for SABnzbd.
func (c *Client) Reannounce(_ context.Context, _ ...string) error {
	return fmt.Errorf("Reannounce not supported for sabnzbd")
}
