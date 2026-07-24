package qbittorrent

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Pause implements downloads.DownloadClient. With no ids the call
// pauses every torrent on the server (qBittorrent treats "all" as
// the magic hashes value).
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	if _, err := c.postForm(ctx, "torrents/pause", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
}

// Resume implements downloads.DownloadClient.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	if _, err := c.postForm(ctx, "torrents/resume", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
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
	if _, err := c.postForm(ctx, "torrents/delete", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
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

// SetPriority implements downloads.DownloadClient.
func (c *Client) SetPriority(ctx context.Context, priority downloads.Priority, ids ...string) error {
	var endpoint string
	switch priority {
	case downloads.PriorityTop:
		endpoint = "torrents/topPrio"
	case downloads.PriorityBottom:
		endpoint = "torrents/bottomPrio"
	case downloads.PriorityUp:
		endpoint = "torrents/increasePrio"
	case downloads.PriorityDown:
		endpoint = "torrents/decreasePrio"
	default:
		return fmt.Errorf("qbittorrent: unknown priority %q", priority)
	}
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	if _, err := c.postForm(ctx, endpoint, form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
}

// SetSpeedLimit implements downloads.DownloadClient.
func (c *Client) SetSpeedLimit(ctx context.Context, limitBytesPerSec int64, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	form.Set("limit", strconv.FormatInt(limitBytesPerSec, 10))
	if _, err := c.postForm(ctx, "torrents/setDownloadLimit", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
}

// ForceStart implements downloads.DownloadClient.
func (c *Client) ForceStart(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	form.Set("value", "true")
	if _, err := c.postForm(ctx, "torrents/setForceStart", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
}

// Recheck implements downloads.DownloadClient.
func (c *Client) Recheck(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	if _, err := c.postForm(ctx, "torrents/recheck", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
}

// Reannounce implements downloads.DownloadClient.
func (c *Client) Reannounce(ctx context.Context, ids ...string) error {
	form := url.Values{}
	form.Set("hashes", joinHashesOrAll(ids))
	if _, err := c.postForm(ctx, "torrents/reannounce", form); err != nil {
		return err
	}
	c.invalidateReadCaches()
	return nil
}
