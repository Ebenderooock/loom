package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ebenderooock/loom/internal/downloads"
)

// categoryEntry is qBittorrent's per-category record on
// /api/v2/torrents/categories. The endpoint returns a JSON object
// keyed by category name; we project it onto a stable slice.
type categoryEntry struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// Categories implements downloads.DownloadClient. The response is
// alphabetised so callers can rely on stable ordering across calls.
func (c *Client) Categories(ctx context.Context) ([]downloads.Category, error) {
	body, err := c.get(ctx, "torrents/categories", nil)
	if err != nil {
		return nil, err
	}
	raw := map[string]categoryEntry{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("qbittorrent: parsing torrents/categories: %w", err)
	}
	out := make([]downloads.Category, 0, len(raw))
	for name, entry := range raw {
		// The map key is authoritative; some qBittorrent versions
		// ship empty `name` fields inside the value object.
		display := entry.Name
		if display == "" {
			display = name
		}
		out = append(out, downloads.Category{
			Name:     display,
			SavePath: entry.SavePath,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
