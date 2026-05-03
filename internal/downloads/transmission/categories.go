package transmission

import (
	"context"
	"sort"

	"github.com/loomctl/loom/internal/downloads"
)

// Categories implements downloads.DownloadClient.
//
// Transmission does not have a native "category" concept. Loom maps
// **labels** (Transmission 3.x+) onto categories instead: the union
// of labels currently present across torrents is returned, sorted.
//
// An operator who wants a stable list — for example to drive a
// Sonarr/Radarr dropdown before any torrent has been added — can
// pre-populate Config.Categories. When that field is set we return
// the configured list verbatim and skip the live scan.
func (c *Client) Categories(ctx context.Context) ([]downloads.Category, error) {
	if len(c.cfg.categories) > 0 {
		out := make([]downloads.Category, 0, len(c.cfg.categories))
		for _, name := range c.cfg.categories {
			if name == "" {
				continue
			}
			out = append(out, downloads.Category{Name: name})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}

	// Probe live torrents for their labels. Asking for just two
	// fields keeps the payload small even on busy daemons.
	args := torrentGetArgs{Fields: []string{"id", "labels"}}
	var reply torrentGetReply
	if err := c.call(ctx, "torrent-get", args, &reply); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, t := range reply.Torrents {
		for _, l := range t.Labels {
			if l == "" {
				continue
			}
			seen[l] = struct{}{}
		}
	}
	out := make([]downloads.Category, 0, len(seen))
	for name := range seen {
		out = append(out, downloads.Category{Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
