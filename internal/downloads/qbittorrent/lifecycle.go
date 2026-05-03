package qbittorrent

import (
	"context"
	"net/url"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// Pause implements downloads.DownloadClient. With no ids the call
// pauses every torrent on the server (qBittorrent treats "all" as
// the magic hashes value).
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	_, err := c.postForm(ctx, "torrents/pause", form)
	return err
}

// Resume implements downloads.DownloadClient.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	_, err := c.postForm(ctx, "torrents/resume", form)
	return err
}

// Remove implements downloads.DownloadClient. When deleteFiles is
// true the on-disk content is wiped along with the torrent record;
// false leaves the download directory untouched (qBittorrent's
// `deleteFiles=false` semantics).
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	if deleteFiles {
		form.Set("deleteFiles", "true")
	} else {
		form.Set("deleteFiles", "false")
	}
	_, err := c.postForm(ctx, "torrents/delete", form)
	return err
}

// joinHashesOrAll returns qBittorrent's pipe-delimited hash list, or
// the literal string "all" when the caller passed an empty slice.
func joinHashesOrAll(ids []string) string {
	if len(ids) == 0 {
		return "all"
	}
	return strings.Join(ids, "|")
}

// guard against unused-import lint when this file is the only consumer
// of the package symbol set.
var _ = downloads.StatusItemPaused
