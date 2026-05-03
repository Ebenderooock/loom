package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// torrentInfo mirrors the subset of /api/v2/torrents/info we consume.
// Documented at:
//
//	https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#get-torrent-list
type torrentInfo struct {
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	Size         int64   `json:"size"`
	TotalSize    int64   `json:"total_size"`
	Downloaded   int64   `json:"downloaded"`
	Progress     float64 `json:"progress"`
	State        string  `json:"state"`
	Category     string  `json:"category"`
	SavePath     string  `json:"save_path"`
	ETA          int64   `json:"eta"`
	DLSpeed      int64   `json:"dlspeed"`
	UpSpeed      int64   `json:"upspeed"`
	Ratio        float64 `json:"ratio"`
	AmountLeft   int64   `json:"amount_left"`
	CompletionOn int64   `json:"completion_on"`
}

// Status implements downloads.DownloadClient. An empty ids slice
// returns every torrent the server knows about; otherwise the
// `hashes` filter is applied server-side.
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	params := url.Values{}
	if len(ids) > 0 {
		params.Set("hashes", strings.Join(ids, "|"))
	}
	body, err := c.get(ctx, "torrents/info", params)
	if err != nil {
		return nil, err
	}
	var raw []torrentInfo
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("qbittorrent: parsing torrents/info: %w", err)
	}
	if len(ids) > 0 && len(raw) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownHash, strings.Join(ids, ","))
	}
	out := make([]downloads.Item, 0, len(raw))
	for _, t := range raw {
		out = append(out, mapItem(t))
	}
	return out, nil
}

// mapItem projects a /torrents/info row onto downloads.Item. Size
// prefers `total_size` when reported (qBittorrent fills it for
// running torrents) and falls back to `size` otherwise.
func mapItem(t torrentInfo) downloads.Item {
	size := t.TotalSize
	if size == 0 {
		size = t.Size
	}
	return downloads.Item{
		ID:              strings.ToLower(t.Hash),
		Title:           t.Name,
		Category:        t.Category,
		Status:          mapState(t.State),
		Progress:        t.Progress,
		SizeBytes:       size,
		DownloadedBytes: t.Downloaded,
		ETA:             t.ETA,
		DownloadRate:    t.DLSpeed,
		UploadRate:      t.UpSpeed,
		Ratio:           t.Ratio,
		Message:         "",
	}
}

// mapState collapses qBittorrent's rich state vocabulary onto the
// small downloads.ItemStatus enum. Keeping the mapping in one
// function makes it easy to audit when qBittorrent adds new states.
//
// State strings come from:
//
//	https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#get-torrent-list
func mapState(state string) downloads.ItemStatus {
	switch state {
	case "downloading", "metaDL", "forcedDL", "checkingDL", "allocating":
		return downloads.StatusItemDownloading
	case "stalledDL":
		// Still considered "in progress" — operator knows nothing
		// is actually moving via DownloadRate=0.
		return downloads.StatusItemDownloading
	case "queuedDL", "queuedUP":
		return downloads.StatusItemQueued
	case "uploading", "forcedUP", "stalledUP", "checkingUP":
		return downloads.StatusItemSeeding
	case "pausedDL":
		return downloads.StatusItemPaused
	case "pausedUP":
		// Paused-while-seeding maps to "completed" rather than
		// "paused": the download itself is finished and the
		// operator has chosen to stop seeding. Status dashboards
		// care about whether bytes are still owed.
		return downloads.StatusItemCompleted
	case "error", "missingFiles":
		return downloads.StatusItemFailed
	case "moving", "checkingResumeData":
		return downloads.StatusItemDownloading
	default:
		return downloads.StatusItemUnknown
	}
}
