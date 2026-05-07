package transmission

import (
	"context"
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// torrentGetArgs is the `arguments` body for torrent-get. Fields
// requested here are the minimum needed to populate downloads.Item;
// extending the projection later is one append away.
type torrentGetArgs struct {
	Fields []string `json:"fields"`
	IDs    any      `json:"ids,omitempty"`
}

// torrentGetReply mirrors the response envelope. Transmission keys
// fields by the names requested above.
type torrentGetReply struct {
	Torrents []torrentRow `json:"torrents"`
}

// torrentRow is the subset of /torrent-get fields Loom consumes.
// Field names match the Transmission RPC spec verbatim so the JSON
// tag set is the spec itself.
type torrentRow struct {
	ID            int64    `json:"id"`
	HashString    string   `json:"hashString"`
	Name          string   `json:"name"`
	Status        int      `json:"status"`
	TotalSize     int64    `json:"totalSize"`
	LeftUntilDone int64    `json:"leftUntilDone"`
	ETA           int64    `json:"eta"`
	UploadRatio   float64  `json:"uploadRatio"`
	DownloadDir   string   `json:"downloadDir"`
	ErrorString   string   `json:"errorString"`
	PercentDone   float64  `json:"percentDone"`
	Labels        []string `json:"labels"`
	RateDownload  int64    `json:"rateDownload"`
	RateUpload    int64    `json:"rateUpload"`
}

// statusFields is the static fields list we ask torrent-get for. Kept
// at package scope so tests and Status() share the same projection.
var statusFields = []string{
	"id",
	"hashString",
	"name",
	"status",
	"totalSize",
	"leftUntilDone",
	"eta",
	"uploadRatio",
	"downloadDir",
	"errorString",
	"percentDone",
	"labels",
	"rateDownload",
	"rateUpload",
}

// Status implements downloads.DownloadClient. An empty ids slice
// returns every torrent the daemon knows about; otherwise the daemon
// applies its `ids` filter (which accepts both integer ids and
// hashStrings, in any mix).
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	args := torrentGetArgs{Fields: statusFields}
	if len(ids) > 0 {
		args.IDs = ids
	}
	var reply torrentGetReply
	if err := c.call(ctx, "torrent-get", args, &reply); err != nil {
		return nil, err
	}
	if len(ids) > 0 && len(reply.Torrents) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTorrent, strings.Join(ids, ","))
	}
	out := make([]downloads.Item, 0, len(reply.Torrents))
	for _, t := range reply.Torrents {
		out = append(out, mapTorrent(t))
	}
	return out, nil
}

// mapTorrent projects a torrent-get row onto downloads.Item. The
// downloaded-bytes calculation is `totalSize - leftUntilDone` because
// Transmission does not expose a single cumulative-downloaded field
// in the modern RPC vocabulary.
func mapTorrent(t torrentRow) downloads.Item {
	downloaded := t.TotalSize - t.LeftUntilDone
	if downloaded < 0 {
		downloaded = 0
	}
	category := ""
	if len(t.Labels) > 0 {
		// First label wins as the "category" in Loom's
		// vocabulary. Subsequent labels remain visible via
		// Categories() / health rows.
		category = t.Labels[0]
	}
	eta := t.ETA
	if eta < 0 {
		// Transmission uses negative sentinels (-1 "unknown",
		// -2 "no peers"); collapse them to zero so dashboards do
		// not render bogus negative durations.
		eta = 0
	}
	return downloads.Item{
		ID:              strings.ToLower(t.HashString),
		Title:           t.Name,
		Category:        category,
		Status:          mapStatus(t.Status),
		Progress:        t.PercentDone,
		SizeBytes:       t.TotalSize,
		DownloadedBytes: downloaded,
		ETA:             eta,
		DownloadRate:    t.RateDownload,
		UploadRate:      t.RateUpload,
		Ratio:           t.UploadRatio,
		Message:         t.ErrorString,
	}
}

// mapStatus collapses Transmission's seven status integers onto the
// small downloads.ItemStatus enum. Keeping the mapping in one switch
// makes it easy to audit when Transmission adds a new state.
//
// Status values come from libtransmission/transmission.h:
//
//	0 TR_STATUS_STOPPED        — paused
//	1 TR_STATUS_CHECK_WAIT     — queued (waiting to verify)
//	2 TR_STATUS_CHECK          — verifying local data (still active)
//	3 TR_STATUS_DOWNLOAD_WAIT  — queued (waiting to download)
//	4 TR_STATUS_DOWNLOAD       — downloading
//	5 TR_STATUS_SEED_WAIT      — queued (waiting to seed)
//	6 TR_STATUS_SEED           — seeding
func mapStatus(s int) downloads.ItemStatus {
	switch s {
	case 0:
		return downloads.StatusItemPaused
	case 1, 3, 5:
		return downloads.StatusItemQueued
	case 2:
		// Verification keeps the torrent "active" from Loom's
		// perspective — surfacing it as `downloading` matches
		// how qBittorrent's `checkingDL` is mapped.
		return downloads.StatusItemDownloading
	case 4:
		return downloads.StatusItemDownloading
	case 6:
		return downloads.StatusItemSeeding
	default:
		return downloads.StatusItemUnknown
	}
}
