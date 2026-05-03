package deluge

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// Add submits a magnet, .torrent URL, or raw .torrent payload to
// the running Deluge daemon via the Web UI's JSON-RPC. Deluge has
// three distinct add methods on the daemon — we route to the right
// one based on which AddRequest field is populated. Each returns
// the torrent's infohash directly, which we use as the item id.
//
// Precedence on the AddRequest follows downloads.AddRequest's
// contract: RawBytes wins, then Magnet, then TorrentURL.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	options := buildAddOptions(req)

	var (
		method string
		params []any
	)
	switch {
	case len(req.RawBytes) > 0:
		method = "core.add_torrent_file"
		params = []any{
			filenameFor(req),
			base64.StdEncoding.EncodeToString(req.RawBytes),
			options,
		}
	case req.Magnet != "":
		method = "core.add_torrent_magnet"
		params = []any{req.Magnet, options}
	case req.TorrentURL != "":
		method = "core.add_torrent_url"
		// Third param is a headers dict; we never set custom
		// headers, so an empty map is correct.
		params = []any{req.TorrentURL, options, map[string]any{}}
	default:
		return downloads.AddResult{}, fmt.Errorf("deluge: AddRequest has neither RawBytes, Magnet, nor TorrentURL")
	}

	// Deluge returns either the torrent_id (a hash string) or
	// null. A null result means the daemon accepted the input but
	// could not parse a hash from it; we surface that as an empty
	// item id rather than an error.
	var hash *string
	if err := c.call(ctx, method, params, &hash); err != nil {
		return downloads.AddResult{}, err
	}
	itemID := ""
	if hash != nil {
		itemID = strings.ToLower(*hash)
	} else if req.Magnet != "" {
		// Best-effort fallback: extract from the magnet URI we
		// were just handed. Avoids leaving a freshly-added item
		// without an id when the daemon dedup-collapses to null.
		itemID = infohashFromMagnet(req.Magnet)
	}
	return downloads.AddResult{
		ClientID: c.id,
		ItemID:   itemID,
	}, nil
}

// buildAddOptions assembles the per-add options dict Deluge accepts
// alongside any of the core.add_torrent_* methods. We surface the
// fields the AddRequest models; everything else falls through to
// the daemon's own per-torrent defaults.
func buildAddOptions(req downloads.AddRequest) map[string]any {
	options := map[string]any{
		// add_paused == false is the Loom default ("start
		// immediately"). Operators that want a paused-on-add
		// behaviour can call Pause() right after.
		"add_paused": false,
	}
	if req.SavePath != "" {
		options["download_location"] = req.SavePath
		// move_completed is intentionally left to the daemon
		// default. SavePath is the *active* download location;
		// move-on-completion is a per-client policy decision and
		// does not belong in a per-add hint.
	}
	return options
}

// filenameFor picks a stable filename for the torrent payload.
// Deluge uses the filename to populate the torrent's display name
// when the bencoded info dict is missing one, so a sensible value
// is operator-friendly.
func filenameFor(req downloads.AddRequest) string {
	name := req.Title
	if name == "" {
		name = "loom-upload"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	return name
}

// infohashFromMagnet extracts the BTIH (BitTorrent v1 infohash)
// from a magnet URI's xt parameter. Returns "" if the URI does not
// carry a recognisable hash.
func infohashFromMagnet(magnet string) string {
	u, err := url.Parse(magnet)
	if err != nil {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		const prefix = "urn:btih:"
		if strings.HasPrefix(xt, prefix) {
			return strings.ToLower(strings.TrimPrefix(xt, prefix))
		}
	}
	return ""
}
