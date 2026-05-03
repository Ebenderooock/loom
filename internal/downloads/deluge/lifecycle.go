package deluge

import (
	"context"
	"strings"
)

// Pause implements downloads.DownloadClient. With one id Deluge has
// a singular core.pause_torrent; with many it offers core.pause_torrents.
// Empty ids means "pause everything", which we implement as a Status
// + batch pause so we honour the same semantics qBittorrent does.
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	hashes, err := c.expandIDs(ctx, ids)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		return nil
	}
	if len(hashes) == 1 {
		return c.call(ctx, "core.pause_torrent", []any{hashes[0]}, nil)
	}
	return c.call(ctx, "core.pause_torrents", []any{hashes}, nil)
}

// Resume implements downloads.DownloadClient.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	hashes, err := c.expandIDs(ctx, ids)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		return nil
	}
	if len(hashes) == 1 {
		return c.call(ctx, "core.resume_torrent", []any{hashes[0]}, nil)
	}
	return c.call(ctx, "core.resume_torrents", []any{hashes}, nil)
}

// Remove implements downloads.DownloadClient. When deleteFiles is
// true the on-disk content is wiped along with the torrent record;
// false leaves the download directory untouched.
//
// Deluge has core.remove_torrent(id, remove_data) but no batch
// variant in the public API across all 2.x builds, so we loop on
// the client side. Errors short-circuit — partial removes leave
// the operator with a clear error rather than silent half-success.
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	hashes, err := c.expandIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, h := range hashes {
		if err := c.call(ctx, "core.remove_torrent",
			[]any{h, deleteFiles}, nil); err != nil {
			return err
		}
	}
	return nil
}

// expandIDs resolves the `empty slice = all` convention into a
// concrete list of hashes by asking the daemon for the full set when
// the caller did not specify one. Lowercases all hashes for
// consistent comparison with what Deluge stores internally.
func (c *Client) expandIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) > 0 {
		out := make([]string, len(ids))
		for i, h := range ids {
			out[i] = strings.ToLower(h)
		}
		return out, nil
	}
	var session []string
	if err := c.call(ctx, "core.get_session_state", nil, &session); err != nil {
		return nil, err
	}
	for i, h := range session {
		session[i] = strings.ToLower(h)
	}
	return session, nil
}
