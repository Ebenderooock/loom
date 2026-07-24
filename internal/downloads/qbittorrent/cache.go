package qbittorrent

import (
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

const readCacheTTL = time.Minute

type statusCache struct {
	fetchedAt time.Time
	items     []downloads.Item
}

type categoriesCache struct {
	fetchedAt time.Time
	items     []downloads.Category
}

type freeSpaceCache struct {
	fetchedAt time.Time
	bytes     int64
	ok        bool
}

func (c *Client) cachedStatus() ([]downloads.Item, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.statusCache.fetchedAt.IsZero() || c.now().Sub(c.statusCache.fetchedAt) >= readCacheTTL {
		return nil, false
	}
	return cloneItems(c.statusCache.items), true
}

func (c *Client) storeStatusCache(items []downloads.Item) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.statusCache = statusCache{
		fetchedAt: c.now(),
		items:     cloneItems(items),
	}
}

func (c *Client) cachedCategories() ([]downloads.Category, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.categoriesCache.fetchedAt.IsZero() || c.now().Sub(c.categoriesCache.fetchedAt) >= readCacheTTL {
		return nil, false
	}
	return cloneCategories(c.categoriesCache.items), true
}

func (c *Client) storeCategoriesCache(items []downloads.Category) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.categoriesCache = categoriesCache{
		fetchedAt: c.now(),
		items:     cloneCategories(items),
	}
}

func (c *Client) cachedFreeSpace() (int64, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if !c.freeSpaceCache.ok || c.freeSpaceCache.fetchedAt.IsZero() || c.now().Sub(c.freeSpaceCache.fetchedAt) >= readCacheTTL {
		return 0, false
	}
	return c.freeSpaceCache.bytes, true
}

func (c *Client) storeFreeSpaceCache(bytes int64) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.freeSpaceCache = freeSpaceCache{
		fetchedAt: c.now(),
		bytes:     bytes,
		ok:        true,
	}
}

func (c *Client) invalidateReadCaches() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.statusCache = statusCache{}
	c.categoriesCache = categoriesCache{}
	c.freeSpaceCache = freeSpaceCache{}
}

func cloneItems(items []downloads.Item) []downloads.Item {
	if items == nil {
		return nil
	}
	out := make([]downloads.Item, len(items))
	copy(out, items)
	return out
}

func cloneCategories(items []downloads.Category) []downloads.Category {
	if items == nil {
		return nil
	}
	out := make([]downloads.Category, len(items))
	copy(out, items)
	return out
}
