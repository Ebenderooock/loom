package nzbget

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Pause asks NZBGet to pause the listed groups via
// editqueue("GroupPause"). Empty ids pauses the entire download
// queue via the daemon-wide `pausedownload` method.
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return c.call(ctx, "pausedownload", nil, nil)
	}
	return c.editGroups(ctx, "GroupPause", ids, false)
}

// Resume reverses Pause.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return c.call(ctx, "resumedownload", nil, nil)
	}
	return c.editGroups(ctx, "GroupResume", ids, false)
}

// Remove deletes items. NZBGet exposes two delete commands:
//
//   - GroupDelete moves the group to the history with status DELETED.
//     The data on disk is **kept** (so the operator can replay it)
//     and the row stays visible in `history(false)`.
//   - GroupFinalDelete removes the group AND its history record AND
//     unlinks any partially-downloaded files on disk.
//
// We pick GroupFinalDelete when deleteFiles is true and GroupDelete
// otherwise — the closest semantic match to the abstraction's
// "delete files" knob.
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	if len(ids) == 0 {
		return nil
	}
	command := "GroupDelete"
	if deleteFiles {
		command = "GroupFinalDelete"
	}
	return c.editGroups(ctx, command, ids, true)
}

// editGroups invokes editqueue with a Group* command. NZBGet's
// `editqueue(Command, Offset, EditText, IDs)` signature treats IDs
// as integer NZBIDs; the caller is responsible for converting the
// abstraction's stringly-typed item IDs back to ints.
//
// requireFound mirrors how lifecycle commands should behave: pause
// and resume tolerate a "no such group" because the group may have
// just finished, but a delete must surface ErrNotFound when none of
// the requested NZBIDs landed.
func (c *Client) editGroups(ctx context.Context, command string, ids []string, requireFound bool) error {
	parsed, err := parseIDs(ids)
	if err != nil {
		return err
	}
	params := []any{command, 0, "", parsed}

	var ok bool
	if err := c.call(ctx, "editqueue", params, &ok); err != nil {
		return err
	}
	if !ok && requireFound {
		return fmt.Errorf("%w: editqueue %s refused for ids=%v", ErrNotFound, command, ids)
	}
	return nil
}

// parseIDs converts the abstraction's string item IDs to the int
// list NZBGet's editqueue expects. An empty/whitespace id is
// silently dropped so callers can pass the slice straight from
// `Status` results without filtering.
func parseIDs(ids []string) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: NZBID %q is not an integer", ErrNotFound, id)
		}
		out = append(out, n)
	}
	return out, nil
}

// SetPriority is not supported for NZBGet.
func (c *Client) SetPriority(_ context.Context, _ downloads.Priority, _ ...string) error {
	return fmt.Errorf("SetPriority not supported for nzbget")
}

// SetSpeedLimit is not supported for NZBGet.
func (c *Client) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error {
	return fmt.Errorf("SetSpeedLimit not supported for nzbget")
}

// ForceStart is not supported for NZBGet.
func (c *Client) ForceStart(_ context.Context, _ ...string) error {
	return fmt.Errorf("ForceStart not supported for nzbget")
}

// Recheck is not supported for NZBGet.
func (c *Client) Recheck(_ context.Context, _ ...string) error {
	return fmt.Errorf("Recheck not supported for nzbget")
}

// Reannounce is not supported for NZBGet.
func (c *Client) Reannounce(_ context.Context, _ ...string) error {
	return fmt.Errorf("Reannounce not supported for nzbget")
}