package sabnzbd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// Kind returns the registered downloads.Kind.
func (c *Client) Kind() downloads.Kind { return downloads.KindSABnzbd }

// Protocol is always usenet.
func (c *Client) Protocol() downloads.Protocol { return downloads.ProtocolUsenet }

// queueResponse mirrors the JSON SAB returns under mode=queue. Only
// the fields we map onto downloads.Item are decoded; SAB ships a
// large surface and we deliberately ignore the rest.
type queueResponse struct {
	Queue struct {
		Slots []queueSlot `json:"slots"`
	} `json:"queue"`
}

type queueSlot struct {
	NzoID    string `json:"nzo_id"`
	Filename string `json:"filename"`
	Cat      string `json:"cat"`
	Status   string `json:"status"`
	// SAB serialises sizes as megabyte strings (e.g. "1024.5"). The
	// units field is "MB" for queue, "KB" for some history rows; we
	// always treat the *_total / *_left numbers as MB to keep the
	// math uniform — SAB's own UI does the same.
	Mb        string `json:"mb"`
	MbLeft    string `json:"mbleft"`
	Percentage string `json:"percentage"`
	TimeLeft  string `json:"timeleft"`
	Priority  string `json:"priority"`
	Script    string `json:"script"`
}

// historyResponse mirrors mode=history. SAB returns *_total in
// bytes here, not MB — completed jobs report final byte counts.
type historyResponse struct {
	History struct {
		Slots []historySlot `json:"slots"`
	} `json:"history"`
}

type historySlot struct {
	NzoID    string `json:"nzo_id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Bytes    int64  `json:"bytes"`
	Storage  string `json:"storage"`
	FailMsg  string `json:"fail_message"`
}

// Status returns the current state of items in SAB. It merges the
// queue (in-flight) with the history (completed/failed). When ids
// is empty all items are returned. Empty queue + empty history is
// not an error.
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	filter := newIDFilter(ids)

	queue, err := c.fetchQueue(ctx)
	if err != nil {
		return nil, err
	}
	hist, err := c.fetchHistory(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]downloads.Item, 0, len(queue.Queue.Slots)+len(hist.History.Slots))
	for _, s := range queue.Queue.Slots {
		if !filter.match(s.NzoID) {
			continue
		}
		out = append(out, queueItem(s))
	}
	for _, s := range hist.History.Slots {
		if !filter.match(s.NzoID) {
			continue
		}
		out = append(out, historyItem(s))
	}
	if len(ids) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("%w: ids=%v", ErrNotFound, ids)
	}
	return out, nil
}

func (c *Client) fetchQueue(ctx context.Context) (queueResponse, error) {
	var resp queueResponse
	if err := c.getJSON(ctx, c.endpoint("queue", nil), &resp); err != nil {
		return queueResponse{}, err
	}
	return resp, nil
}

func (c *Client) fetchHistory(ctx context.Context) (historyResponse, error) {
	var resp historyResponse
	if err := c.getJSON(ctx, c.endpoint("history", nil), &resp); err != nil {
		return historyResponse{}, err
	}
	return resp, nil
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

// queueItem projects a queue slot onto a downloads.Item, including
// the SAB->downloads status mapping.
func queueItem(s queueSlot) downloads.Item {
	total := mbToBytes(s.Mb)
	left := mbToBytes(s.MbLeft)
	downloaded := total - left
	if downloaded < 0 {
		downloaded = 0
	}
	return downloads.Item{
		ID:              s.NzoID,
		Title:           s.Filename,
		Category:        s.Cat,
		Status:          mapQueueStatus(s.Status),
		Progress:        parsePercent(s.Percentage),
		SizeBytes:       total,
		DownloadedBytes: downloaded,
		ETA:             parseTimeLeft(s.TimeLeft),
	}
}

// historyItem projects a completed/failed history slot.
func historyItem(s historySlot) downloads.Item {
	st := mapHistoryStatus(s.Status)
	progress := 1.0
	if st == downloads.StatusItemFailed {
		progress = 0
	}
	return downloads.Item{
		ID:              s.NzoID,
		Title:           s.Name,
		Category:        s.Category,
		Status:          st,
		Progress:        progress,
		SizeBytes:       s.Bytes,
		DownloadedBytes: s.Bytes,
		Message:         s.FailMsg,
	}
}

// mapQueueStatus is the canonical SAB-queue → downloads.ItemStatus
// table. SAB documents these strings in interfaces/Plush/templates.
// Kept in one switch so adding a new case is a one-line review.
func mapQueueStatus(sab string) downloads.ItemStatus {
	switch sab {
	case "Downloading", "Fetching", "Checking", "Extracting", "Repairing", "Verifying", "Moving", "Running":
		return downloads.StatusItemDownloading
	case "Paused":
		return downloads.StatusItemPaused
	case "Queued", "Grabbing", "QuickCheck":
		return downloads.StatusItemQueued
	case "Completed":
		return downloads.StatusItemCompleted
	case "Failed":
		return downloads.StatusItemFailed
	default:
		return downloads.StatusItemUnknown
	}
}

// mapHistoryStatus translates the smaller history vocabulary.
func mapHistoryStatus(sab string) downloads.ItemStatus {
	switch sab {
	case "Completed":
		return downloads.StatusItemCompleted
	case "Failed":
		return downloads.StatusItemFailed
	case "Queued":
		return downloads.StatusItemQueued
	case "Extracting", "Repairing", "Verifying", "Running", "Moving":
		return downloads.StatusItemDownloading
	default:
		return downloads.StatusItemUnknown
	}
}

// mbToBytes parses a SAB megabyte string to bytes. SAB uses
// decimal megabytes (1 MB = 1_000_000 B) in the queue payload
// regardless of locale; an unparseable string yields zero.
func mbToBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1024 * 1024)
}

// parsePercent reads the "percentage" string. SAB returns an
// integer 0..100 as a string; we normalise to 0.0..1.0.
func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return v / 100.0
}

// parseTimeLeft turns SAB's "HH:MM:SS" or "MM:SS" string into
// seconds. Unparseable values become zero.
func parseTimeLeft(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0:00:00" {
		return 0
	}
	parts := strings.Split(s, ":")
	var h, m, sec int64
	switch len(parts) {
	case 3:
		h, _ = strconv.ParseInt(parts[0], 10, 64)
		m, _ = strconv.ParseInt(parts[1], 10, 64)
		sec, _ = strconv.ParseInt(parts[2], 10, 64)
	case 2:
		m, _ = strconv.ParseInt(parts[0], 10, 64)
		sec, _ = strconv.ParseInt(parts[1], 10, 64)
	case 1:
		sec, _ = strconv.ParseInt(parts[0], 10, 64)
	default:
		return 0
	}
	return h*3600 + m*60 + sec
}
