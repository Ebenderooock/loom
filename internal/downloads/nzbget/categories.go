package nzbget

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

// categoryCacheTTL bounds how long a successful Categories result
// is reused. NZBGet only re-reads its config on daemon reload, so a
// short TTL is fine: we trade a tiny window of staleness for not
// hammering the JSON-RPC endpoint on every dashboard refresh.
const categoryCacheTTL = 30 * time.Second

// categoryCache holds the most recent Categories result for the
// life of one *Client. The mutex is held for the entire duration
// of the upstream call when the cache is cold; callers concurrent
// with the first probe will all wait for the same response rather
// than racing duplicate JSON-RPC calls.
type categoryCache struct {
	mu      sync.Mutex
	at      time.Time
	cached  []downloads.Category
	hadOnce bool
}

// nowFunc is the monotonic clock the cache reads. Tests override it
// to drive expiry without time.Sleep.
var nowFunc = time.Now

// configOption is one of the rows NZBGet returns under the
// `config()` JSON-RPC method. Each option is a name/value pair; the
// list is large (hundreds of entries on a typical install) so we
// stream through it linearly rather than building a map.
type configOption struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// Categories returns the operator-configured NZBGet categories,
// caching for categoryCacheTTL. NZBGet stores categories under
// `Category{N}.Name`, `Category{N}.DestDir`, etc.; we walk those
// keys and collapse into downloads.Category.
//
// On error, the cache is preserved and surfaced as a fresh fetch
// only if no prior result exists. This is on purpose: a transient
// JSON-RPC blip should not erase a known-good list.
func (c *Client) Categories(ctx context.Context) ([]downloads.Category, error) {
	c.categories().mu.Lock()
	defer c.categories().mu.Unlock()

	cache := c.categories()
	if cache.hadOnce && nowFunc().Sub(cache.at) < categoryCacheTTL {
		return cloneCategories(cache.cached), nil
	}

	var rows []configOption
	if err := c.call(ctx, "config", nil, &rows); err != nil {
		if cache.hadOnce {
			return cloneCategories(cache.cached), nil
		}
		return nil, err
	}

	cats := parseCategoryConfig(rows)
	cache.cached = cats
	cache.at = nowFunc()
	cache.hadOnce = true
	return cloneCategories(cats), nil
}

// categories lazily attaches the per-client cache. We avoid a
// constructor change by stashing the cache in a small package-level
// map keyed off the client pointer; this keeps the public NewClient
// signature aligned with the other kinds.
var (
	cacheMu     sync.Mutex
	clientCache = make(map[*Client]*categoryCache)
)

func (c *Client) categories() *categoryCache {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cc, ok := clientCache[c]; ok {
		return cc
	}
	cc := &categoryCache{}
	clientCache[c] = cc
	return cc
}

// parseCategoryConfig walks an NZBGet config option list and
// extracts the Category{N}.Name / Category{N}.DestDir pairs. The
// indices are arbitrary — operators can leave gaps, so we collect
// per-index dictionaries before flattening.
func parseCategoryConfig(rows []configOption) []downloads.Category {
	type entry struct {
		name string
		dir  string
	}
	idx := make(map[string]*entry)
	order := make([]string, 0)

	for _, row := range rows {
		key := row.Name
		if !strings.HasPrefix(key, "Category") {
			continue
		}
		// "Category1.Name" → ["Category1", "Name"]
		parts := strings.SplitN(strings.TrimPrefix(key, "Category"), ".", 2)
		if len(parts) != 2 {
			continue
		}
		index := parts[0]
		field := parts[1]
		e, ok := idx[index]
		if !ok {
			e = &entry{}
			idx[index] = e
			order = append(order, index)
		}
		switch field {
		case "Name":
			e.name = row.Value
		case "DestDir":
			e.dir = row.Value
		}
	}

	out := make([]downloads.Category, 0, len(order))
	for _, k := range order {
		e := idx[k]
		if e.name == "" {
			continue
		}
		out = append(out, downloads.Category{Name: e.name, SavePath: e.dir})
	}
	return out
}

// cloneCategories returns a defensive copy so external mutations do
// not corrupt the cache.
func cloneCategories(in []downloads.Category) []downloads.Category {
	out := make([]downloads.Category, len(in))
	copy(out, in)
	return out
}
