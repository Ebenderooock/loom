package deluge

import (
	"context"
	"sort"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// Categories implements downloads.DownloadClient by exposing
// Deluge's Label plugin. Loom's neutral abstraction names the
// concept "category"; Deluge calls it "label". Both serve the same
// purpose: a free-form grouping tag that downstream automation
// (Sonarr/Radarr-style) reads to decide where files end up.
//
// The Label plugin is optional. When it is not enabled on the
// daemon, label.get_labels surfaces as an RPC error; we swallow
// that and return an empty list rather than an error so dashboards
// continue to render. Test() reports the missing plugin as a soft
// signal so operators discover the gap proactively.
//
// Deluge does not associate a save_path with a label at the daemon
// level (auto-move-on-add is configured per-label but not exposed
// over the public RPC), so SavePath on the returned Category is
// always empty.
func (c *Client) Categories(ctx context.Context) ([]downloads.Category, error) {
	var labels []string
	if err := c.call(ctx, "label.get_labels", nil, &labels); err != nil {
		if isLabelPluginMissing(err) {
			return []downloads.Category{}, nil
		}
		return nil, err
	}
	out := make([]downloads.Category, 0, len(labels))
	for _, name := range labels {
		out = append(out, downloads.Category{Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// isLabelPluginMissing recognises the RPC errors Deluge surfaces
// when the Label plugin is not enabled. Wording varies by version:
//
//	1.x: "Unknown method: label.get_labels"
//	2.x: "RPC method label.get_labels doesn't exist"
//	dev: "KeyError: 'label'"
func isLabelPluginMissing(err error) bool {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "label.get_labels"),
		strings.Contains(msg, "label plugin"),
		strings.Contains(msg, "unknown method"),
		strings.Contains(msg, "doesn't exist"),
		strings.Contains(msg, "no such method"):
		return true
	}
	return false
}
