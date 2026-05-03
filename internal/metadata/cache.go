package metadata

import (
	"sync"
	"time"
)

// cacheEntry holds a cached value with its expiration time.
type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// Cache provides an in-memory cache with per-entry TTL. It is safe for
// concurrent use. Two distinct TTL tiers are supported: search results
// (30 min) and full details (7 days). The cache is keyed by ID (tmdbid,
// imdbid, tvdbid) or query hash (title+year).
type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry

	// cleanupTicker runs every 5 minutes to evict expired entries.
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
}

// NewCache creates a new in-memory cache and starts a background cleanup
// goroutine that expires stale entries every 5 minutes.
func NewCache() *Cache {
	c := &Cache{
		entries:       make(map[string]cacheEntry),
		cleanupTicker: time.NewTicker(5 * time.Minute),
		cleanupDone:   make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// cleanupLoop periodically removes expired entries from the cache.
func (c *Cache) cleanupLoop() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.cleanup()
		case <-c.cleanupDone:
			c.cleanupTicker.Stop()
			return
		}
	}
}

// cleanup removes all expired entries. Called by cleanupLoop and tests.
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// Get retrieves a value from the cache if it exists and has not expired.
// Returns (nil, false) if the key is not found or has expired.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}

	return entry.value, true
}

// Set stores a value in the cache with the given TTL.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
}

// Delete removes a key from the cache immediately.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// Clear removes all entries from the cache. Used in tests and shutdown.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]cacheEntry)
}

// Close stops the cleanup goroutine and clears the cache.
func (c *Cache) Close() {
	close(c.cleanupDone)
	c.Clear()
}

// Size returns the current number of entries in the cache (for testing).
func (c *Cache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// --- Cache TTL constants -----------------------------------------------

const (
	// TTLSearchResult is the cache lifetime for incomplete search results
	// from queries like "find movie by title+year". These results may be
	// partial or incomplete, so we revalidate them more frequently.
	TTLSearchResult = 30 * time.Minute

	// TTLFullDetails is the cache lifetime for complete metadata records
	// fetched by external ID or confirmed as complete. These are stable
	// and unlikely to change frequently.
	TTLFullDetails = 7 * 24 * time.Hour // 7 days
)
