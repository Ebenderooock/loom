package sabnzbd

import (
	"context"
	"encoding/json"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Categories returns the SAB-configured categories. SAB exposes two
// shapes: a flat list under mode=get_cats (just names) and a richer
// per-category map under mode=get_config&section=categories. We try
// the rich form first — operators almost always want the SavePath —
// and fall back to the flat list if the config endpoint refuses.
func (c *Client) Categories(ctx context.Context) ([]downloads.Category, error) {
	if cats, err := c.fetchRichCategories(ctx); err == nil && cats != nil {
		return cats, nil
	}
	return c.fetchFlatCategories(ctx)
}

// fetchFlatCategories hits mode=get_cats. SAB returns
// {"categories": ["*", "movies", ...]}. The first element is "*",
// the SAB sentinel for "default", which we drop because it is not a
// real category.
func (c *Client) fetchFlatCategories(ctx context.Context) ([]downloads.Category, error) {
	var resp struct {
		Categories []string `json:"categories"`
	}
	if err := c.getJSON(ctx, c.endpoint("get_cats", nil), &resp); err != nil {
		return nil, err
	}
	out := make([]downloads.Category, 0, len(resp.Categories))
	for _, name := range resp.Categories {
		if name == "*" || name == "" {
			continue
		}
		out = append(out, downloads.Category{Name: name})
	}
	return out, nil
}

// fetchRichCategories pulls the per-category config so we can fill
// SavePath. The endpoint is gated behind SAB's "advanced" config so
// older builds may 403 — callers fall back to the flat list.
func (c *Client) fetchRichCategories(ctx context.Context) ([]downloads.Category, error) {
	var raw json.RawMessage
	if err := c.getJSON(ctx, c.endpoint("get_config", nil), &raw); err != nil {
		return nil, err
	}
	var wrapper struct {
		Config struct {
			Categories []struct {
				Name string `json:"name"`
				Dir  string `json:"dir"`
			} `json:"categories"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil || len(wrapper.Config.Categories) == 0 {
		return nil, err
	}
	out := make([]downloads.Category, 0, len(wrapper.Config.Categories))
	for _, c := range wrapper.Config.Categories {
		if c.Name == "*" || c.Name == "" {
			continue
		}
		out = append(out, downloads.Category{Name: c.Name, SavePath: c.Dir})
	}
	return out, nil
}
