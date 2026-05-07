package nzbget

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind returns the registered downloads.Kind.
func (c *Client) Kind() downloads.Kind { return downloads.KindNZBGet }

// Protocol is always usenet.
func (c *Client) Protocol() downloads.Protocol { return downloads.ProtocolUsenet }

// listGroup is the subset of NZBGet's `listgroups` element we map
// onto downloads.Item. NZBGet ships dozens of fields per row; we
// decode only what the abstraction surfaces today.
type listGroup struct {
	NZBID            int64  `json:"NZBID"`
	NZBName          string `json:"NZBName"`
	Kind             string `json:"Kind"` // NZB | URL
	Status           string `json:"Status"`
	Category         string `json:"Category"`
	FileSizeMB       int64  `json:"FileSizeMB"`
	RemainingSizeMB  int64  `json:"RemainingSizeMB"`
	DownloadedSizeMB int64  `json:"DownloadedSizeMB"`
	DownloadRate     int64  `json:"DownloadRate"` // bytes/sec
	PostInfoText     string `json:"PostInfoText"`
	DestDir          string `json:"DestDir"`
	// MinPostTime / ActiveDownloads are present in v21+ payloads;
	// kept on the struct so we can extend ETA logic without another
	// fixture pass.
	ActiveDownloads int   `json:"ActiveDownloads"`
	PostTotalTimeSec int64 `json:"PostTotalTimeSec"`
}

// historyGroup mirrors the relevant rows of `history(false)`.
// NZBGet's history shows entries in three phases: post-processing
// (Kind=NZB still present, Status begins with PP_), completed
// (Kind=NZB, Status=SUCCESS/FAILURE/...), and URL-fetched (Kind=URL).
type historyGroup struct {
	NZBID        int64  `json:"NZBID"`
	Name         string `json:"Name"`
	Kind         string `json:"Kind"`
	Status       string `json:"Status"`
	Category     string `json:"Category"`
	FileSizeMB   int64  `json:"FileSizeMB"`
	DestDir      string `json:"DestDir"`
	ParStatus    string `json:"ParStatus"`
	UnpackStatus string `json:"UnpackStatus"`
	MoveStatus   string `json:"MoveStatus"`
	ScriptStatus string `json:"ScriptStatus"`
	// NZBGet stores the human-readable error in different fields
	// across versions; the merge picks the first non-empty one.
	DeleteStatus string `json:"DeleteStatus"`
	HealthStatus string `json:"HealthStatus"`
}

// Status returns the merged in-flight + post-processing + completed
// view from NZBGet. We call listgroups + history(false); the latter
// includes both post-processing rows and finished history entries
// (the `false` parameter means "do not hide hidden entries").
//
// When ids is empty all items are returned; otherwise the result is
// filtered to those NZBIDs (string-formatted to match the
// downloads.AddResult.ItemID we hand out from Add).
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	filter := newIDFilter(ids)

	var queue []listGroup
	if err := c.call(ctx, "listgroups", []any{0}, &queue); err != nil {
		return nil, err
	}
	var hist []historyGroup
	if err := c.call(ctx, "history", []any{false}, &hist); err != nil {
		return nil, err
	}

	out := make([]downloads.Item, 0, len(queue)+len(hist))
	for _, g := range queue {
		id := strconv.FormatInt(g.NZBID, 10)
		if !filter.match(id) {
			continue
		}
		out = append(out, queueItem(id, g))
	}
	for _, h := range hist {
		id := strconv.FormatInt(h.NZBID, 10)
		if !filter.match(id) {
			continue
		}
		out = append(out, historyItem(id, h))
	}
	if len(ids) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("%w: ids=%v", ErrNotFound, ids)
	}
	return out, nil
}

// queueItem projects a listgroups row onto downloads.Item.
func queueItem(id string, g listGroup) downloads.Item {
	totalBytes := mbToBytes(g.FileSizeMB)
	remainingBytes := mbToBytes(g.RemainingSizeMB)
	downloadedBytes := mbToBytes(g.DownloadedSizeMB)
	if downloadedBytes == 0 && totalBytes > remainingBytes {
		downloadedBytes = totalBytes - remainingBytes
	}

	progress := 0.0
	if totalBytes > 0 {
		progress = float64(downloadedBytes) / float64(totalBytes)
		if progress > 1 {
			progress = 1
		}
	}

	return downloads.Item{
		ID:              id,
		Title:           g.NZBName,
		Category:        g.Category,
		Status:          mapQueueStatus(g.Status),
		Progress:        progress,
		SizeBytes:       totalBytes,
		DownloadedBytes: downloadedBytes,
		ETA:             etaSeconds(remainingBytes, g.DownloadRate),
		DownloadRate:    g.DownloadRate,
		Message:         g.PostInfoText,
	}
}

// historyItem projects a history(false) row onto downloads.Item.
func historyItem(id string, h historyGroup) downloads.Item {
	st := mapHistoryStatus(h.Status)
	progress := 1.0
	if st == downloads.StatusItemFailed {
		progress = 0
	}
	size := mbToBytes(h.FileSizeMB)
	return downloads.Item{
		ID:              id,
		Title:           h.Name,
		Category:        h.Category,
		Status:          st,
		Progress:        progress,
		SizeBytes:       size,
		DownloadedBytes: size,
		Message:         historyMessage(h),
	}
}

// historyMessage picks the first non-empty diagnostic field NZBGet
// supplies on a history row. Kept separate so the case ordering is
// reviewable in one place.
func historyMessage(h historyGroup) string {
	for _, m := range []string{h.DeleteStatus, h.HealthStatus, h.ParStatus, h.UnpackStatus, h.MoveStatus, h.ScriptStatus} {
		switch m {
		case "", "NONE", "SUCCESS":
			continue
		}
		return m
	}
	return ""
}

// mapQueueStatus is the canonical NZBGet `listgroups.Status` →
// downloads.ItemStatus table. NZBGet's documented status set lives
// in their daemon source under `nzbget/daemon/queue/QueueState.cpp`;
// the strings below are stable across NZBGet 21 and 22.
//
// Kept in one switch so adding a new case is a one-line review.
func mapQueueStatus(state string) downloads.ItemStatus {
	switch state {
	case "QUEUED":
		return downloads.StatusItemQueued
	case "PAUSED":
		return downloads.StatusItemPaused
	case "DOWNLOADING", "FETCHING":
		return downloads.StatusItemDownloading
	case "PP_QUEUED",
		"LOADING_PARS",
		"VERIFYING_SOURCES",
		"REPAIRING",
		"UNPACKING",
		"MOVING",
		"EXECUTING_SCRIPT",
		"COPYING",
		"RENAMING",
		"VERIFYING_REPAIRED":
		return downloads.StatusItemDownloading
	case "PP_FINISHED":
		return downloads.StatusItemCompleted
	case "DELETED":
		// NZBGet flags items for deletion that have not yet been
		// purged; treat them as failed so dashboards do not retry.
		return downloads.StatusItemFailed
	default:
		return downloads.StatusItemUnknown
	}
}

// mapHistoryStatus translates the smaller history vocabulary.
// NZBGet stamps SUCCESS / FAILURE / HEALTH / WARNING on completed
// rows; PP_FINISHED appears briefly while an item is moving from
// post-processing into the final history bucket.
func mapHistoryStatus(state string) downloads.ItemStatus {
	switch state {
	case "SUCCESS":
		return downloads.StatusItemCompleted
	case "FAILURE", "HEALTH":
		return downloads.StatusItemFailed
	case "DELETED":
		return downloads.StatusItemFailed
	case "PP_FINISHED":
		return downloads.StatusItemCompleted
	case "WARNING":
		return downloads.StatusItemCompleted
	default:
		return downloads.StatusItemUnknown
	}
}

// mbToBytes converts NZBGet's binary-megabyte fields to bytes.
// NZBGet reports MB as integers (not strings), so the conversion
// is a straight multiplication. A 1 GiB file shows as 1024 MB.
func mbToBytes(mb int64) int64 {
	if mb < 0 {
		return 0
	}
	return mb * 1024 * 1024
}

// etaSeconds mirrors NZBGet's own ETA calculation: remaining bytes
// divided by current download rate. Returns 0 when the rate is
// zero (paused / queued) or when there is nothing left to fetch.
func etaSeconds(remainingBytes, rateBytesPerSec int64) int64 {
	if rateBytesPerSec <= 0 || remainingBytes <= 0 {
		return 0
	}
	return remainingBytes / rateBytesPerSec
}

// idFilter does the optional ids slice membership test in O(1).
type idFilter struct {
	all bool
	set map[string]struct{}
}

func newIDFilter(ids []string) idFilter {
	if len(ids) == 0 {
		return idFilter{all: true}
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return idFilter{set: set}
}

func (f idFilter) match(id string) bool {
	if f.all {
		return true
	}
	_, ok := f.set[id]
	return ok
}
