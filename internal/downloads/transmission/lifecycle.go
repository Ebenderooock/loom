package transmission

import "context"

// idsArg shapes the `ids` argument shared by torrent-start /
// torrent-stop / torrent-remove. An empty slice is encoded as
// nil so Transmission applies the operation to every torrent.
func idsArg(ids []string) any {
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// Pause implements downloads.DownloadClient via torrent-stop. With
// no ids the daemon stops every torrent.
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	args := map[string]any{}
	if v := idsArg(ids); v != nil {
		args["ids"] = v
	}
	return c.call(ctx, "torrent-stop", args, nil)
}

// Resume implements downloads.DownloadClient via torrent-start.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	args := map[string]any{}
	if v := idsArg(ids); v != nil {
		args["ids"] = v
	}
	return c.call(ctx, "torrent-start", args, nil)
}

// Remove implements downloads.DownloadClient via torrent-remove. When
// deleteFiles is true the daemon unlinks the torrent's on-disk
// payload as well as forgetting the metadata; false leaves the files
// in place (Transmission's default).
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	args := map[string]any{
		"delete-local-data": deleteFiles,
	}
	if v := idsArg(ids); v != nil {
		args["ids"] = v
	}
	return c.call(ctx, "torrent-remove", args, nil)
}
