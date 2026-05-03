package nzbget

import "context"

// statusResponse is the subset of NZBGet's `status()` JSON-RPC
// response we care about. NZBGet ships ~80 fields here; we decode
// only the disk-space accounting because the freespace probe is
// the only consumer.
type statusResponse struct {
	// FreeDiskSpaceMB is the binary-megabyte representation of the
	// free space on NZBGet's MainDir. NZBGet 21+ returns this as an
	// integer; some legacy plugins still return a string-typed
	// payload, but the official daemon does not.
	FreeDiskSpaceMB int64 `json:"FreeDiskSpaceMB"`
}

// FreeSpace returns bytes available on NZBGet's MainDir (the
// download root). NZBGet's `status()` reports FreeDiskSpaceMB in
// binary megabytes; we widen to bytes. Returns -1 when NZBGet does
// not surface the field (older builds, container mounts that lie
// about disk size, etc.).
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	var resp statusResponse
	if err := c.call(ctx, "status", nil, &resp); err != nil {
		return -1, err
	}
	if resp.FreeDiskSpaceMB <= 0 {
		return -1, nil
	}
	return resp.FreeDiskSpaceMB * 1024 * 1024, nil
}
