package transmission

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// torrentAddArgs is the `arguments` body for torrent-add. We only set
// the fields we actually use; Transmission ignores zero-valued ones.
type torrentAddArgs struct {
	Filename    string   `json:"filename,omitempty"`
	Metainfo    string   `json:"metainfo,omitempty"`
	DownloadDir string   `json:"download-dir,omitempty"`
	Paused      bool     `json:"paused,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// torrentAddResult is the relevant slice of the reply. Transmission
// fills exactly one of `torrent-added` (new) or `torrent-duplicate`
// (already present) on success — both shapes carry the hashString
// we hand back as the per-client item id.
type torrentAddResult struct {
	Added struct {
		ID         int64  `json:"id"`
		HashString string `json:"hashString"`
		Name       string `json:"name"`
	} `json:"torrent-added"`
	Duplicate struct {
		ID         int64  `json:"id"`
		HashString string `json:"hashString"`
		Name       string `json:"name"`
	} `json:"torrent-duplicate"`
}

// Add submits a magnet, .torrent URL, or raw .torrent payload to the
// Transmission daemon via the torrent-add RPC. Precedence on the
// AddRequest follows downloads.AddRequest's contract: RawBytes wins,
// then Magnet, then TorrentURL.
//
// The returned ItemID is the torrent's hashString — it is stable
// across daemon restarts (unlike the integer torrent id) and matches
// the format every other Loom torrent kind uses.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	args, err := buildAddArgs(req)
	if err != nil {
		return downloads.AddResult{}, err
	}

	var result torrentAddResult
	if err := c.call(ctx, "torrent-add", args, &result); err != nil {
		return downloads.AddResult{}, err
	}

	hash := strings.ToLower(firstNonEmpty(result.Added.HashString, result.Duplicate.HashString))
	if hash == "" {
		return downloads.AddResult{}, fmt.Errorf("%w: torrent-add returned no hashString", ErrServer)
	}
	return downloads.AddResult{
		ClientID: c.id,
		ItemID:   hash,
	}, nil
}

// buildAddArgs translates an AddRequest into the torrent-add argument
// shape. Raw bytes are base64-encoded into `metainfo`; magnets and
// fetchable URLs share the `filename` slot (Transmission accepts
// both). The category is mapped onto Transmission's labels because
// the protocol has no first-class category field — see
// docs/downloads-transmission.md for the rationale.
func buildAddArgs(req downloads.AddRequest) (torrentAddArgs, error) {
	args := torrentAddArgs{DownloadDir: req.SavePath}

	switch {
	case len(req.RawBytes) > 0:
		args.Metainfo = base64.StdEncoding.EncodeToString(req.RawBytes)
	case req.Magnet != "":
		args.Filename = req.Magnet
	case req.TorrentURL != "":
		args.Filename = req.TorrentURL
	default:
		return torrentAddArgs{}, ErrMissingPayload
	}

	// Transmission 3.x+ supports labels as the closest analogue to
	// Loom's category concept. We always start paused=false (the
	// daemon's default) unless a tag explicitly requests pause —
	// callers that need a paused-on-add behaviour can use the
	// "paused" tag.
	labels := make([]string, 0, len(req.Tags)+1)
	if req.Category != "" {
		labels = append(labels, req.Category)
	}
	for _, t := range req.Tags {
		switch t {
		case "paused":
			args.Paused = true
		default:
			labels = append(labels, t)
		}
	}
	if len(labels) > 0 {
		args.Labels = labels
	}
	return args, nil
}
