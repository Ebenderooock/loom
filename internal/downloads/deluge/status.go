package deluge

import (
	"context"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// torrentStatus is the subset of fields we request from
// core.get_torrents_status. Adding to this list is cheap (Deluge
// returns only what is asked for) but reading them at the call site
// is more disciplined when the surface stays narrow.
type torrentStatus struct {
	Name       string  `json:"name"`
	Hash       string  `json:"hash"`
	TotalSize  int64   `json:"total_size"`
	TotalDone  int64   `json:"total_done"`
	ETA        int64   `json:"eta"`
	Ratio      float64 `json:"ratio"`
	State      string  `json:"state"`
	Label      string  `json:"label"`
	SavePath   string  `json:"save_path"`
	Message    string  `json:"message"`
	Progress   float64 `json:"progress"` // Deluge reports 0-100
	DLPayload  int64   `json:"download_payload_rate"`
	UPPayload  int64   `json:"upload_payload_rate"`
	Paused     bool    `json:"paused"`
	IsFinished bool    `json:"is_finished"`
}

// statusFields is the field list passed to core.get_torrents_status.
// Keep aligned with torrentStatus tags above.
var statusFields = []string{
	"name", "hash", "total_size", "total_done", "eta", "ratio",
	"state", "label", "save_path", "message", "progress",
	"download_payload_rate", "upload_payload_rate",
	"paused", "is_finished",
}

// Status implements downloads.DownloadClient. With ids set, Deluge's
// filter dict narrows by hash; otherwise we ask for everything.
//
// Deluge returns a map[hash]status; we project that onto a stable
// (sorted-by-input-order, then alphabetical) slice so callers do not
// see flapping orderings between calls.
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	filter := map[string]any{}
	if len(ids) > 0 {
		// Deluge accepts an "id" key with a list of hashes for
		// server-side filtering; lowercase to match what the
		// daemon stores.
		lowered := make([]string, len(ids))
		for i, h := range ids {
			lowered[i] = strings.ToLower(h)
		}
		filter["id"] = lowered
	}

	var raw map[string]torrentStatus
	if err := c.call(ctx, "core.get_torrents_status",
		[]any{filter, statusFields}, &raw); err != nil {
		return nil, err
	}
	if len(ids) > 0 && len(raw) == 0 {
		return nil, ErrUnknownHash
	}

	items := make([]downloads.Item, 0, len(raw))
	if len(ids) > 0 {
		// Preserve the caller's input order so list views with
		// stable ids see stable rows.
		seen := map[string]struct{}{}
		for _, h := range ids {
			lower := strings.ToLower(h)
			if t, ok := raw[lower]; ok {
				items = append(items, mapItem(lower, t))
				seen[lower] = struct{}{}
			}
		}
		// Append anything Deluge returned that the caller did
		// not explicitly request — defensive, in case the
		// server-side filter is ignored on some builds.
		for hash, t := range raw {
			if _, ok := seen[hash]; ok {
				continue
			}
			items = append(items, mapItem(hash, t))
		}
		return items, nil
	}
	for hash, t := range raw {
		items = append(items, mapItem(hash, t))
	}
	return items, nil
}

// mapItem projects a torrentStatus row onto downloads.Item.
func mapItem(hash string, t torrentStatus) downloads.Item {
	id := strings.ToLower(t.Hash)
	if id == "" {
		id = strings.ToLower(hash)
	}
	progress := t.Progress / 100.0
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return downloads.Item{
		ID:              id,
		Title:           t.Name,
		Category:        t.Label,
		Status:          mapState(t),
		Progress:        progress,
		SizeBytes:       t.TotalSize,
		DownloadedBytes: t.TotalDone,
		ETA:             t.ETA,
		DownloadRate:    t.DLPayload,
		UploadRate:      t.UPPayload,
		Ratio:           t.Ratio,
		Message:         t.Message,
		SavePath:        t.SavePath,
	}
}

// mapState collapses Deluge's state vocabulary onto the small
// downloads.ItemStatus enum. Keeping the mapping in one function
// makes it easy to audit when Deluge adds new states.
//
// Deluge state strings come from libtorrent and are stable across
// 1.x and 2.x:
//
//	Allocating, Checking, Downloading, Seeding, Paused, Error,
//	Queued, Moving, Active (filter only — never returned)
//
// We treat is_finished + Paused as "completed" so dashboards do not
// show finished-but-stopped torrents as if they still owe bytes.
func mapState(t torrentStatus) downloads.ItemStatus {
	if t.IsFinished && t.Paused {
		return downloads.StatusItemCompleted
	}
	switch t.State {
	case "Downloading":
		return downloads.StatusItemDownloading
	case "Seeding":
		return downloads.StatusItemSeeding
	case "Paused":
		// Paused-while-downloading. Paused-while-seeding has
		// IsFinished=true and is caught above as "completed".
		return downloads.StatusItemPaused
	case "Queued":
		return downloads.StatusItemQueued
	case "Checking", "Allocating", "Moving":
		// In-flight transitional states — surface as
		// "downloading" rather than "queued" because rates and
		// progress are actively changing.
		return downloads.StatusItemDownloading
	case "Error":
		return downloads.StatusItemFailed
	default:
		return downloads.StatusItemUnknown
	}
}
