package sonarrv3

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// idCache provides bidirectional mapping between Loom string UUIDs and
// Sonarr-compatible integer IDs. The integer is derived deterministically
// from the first 8 hex chars of the UUID.
type idCache struct {
	mu      sync.RWMutex
	strToInt map[string]int
	intToStr map[int]string
}

func newIDCache() *idCache {
	return &idCache{
		strToInt: make(map[string]int),
		intToStr: make(map[int]string),
	}
}

// toInt converts a string UUID to a deterministic positive int and
// caches the mapping for reverse lookups.
func (c *idCache) toInt(uuid string) int {
	c.mu.RLock()
	if n, ok := c.strToInt[uuid]; ok {
		c.mu.RUnlock()
		return n
	}
	c.mu.RUnlock()

	n := intID(uuid)

	c.mu.Lock()
	c.strToInt[uuid] = n
	c.intToStr[n] = uuid
	c.mu.Unlock()
	return n
}

// toStr resolves a numeric Sonarr ID back to the Loom string UUID.
func (c *idCache) toStr(id int) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.intToStr[id]
	return s, ok
}

// intID derives a deterministic positive int from a string UUID by
// reading the first 8 hex characters as a uint32. This mirrors the
// strategy used by other compat shims.
func intID(uuid string) int {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) < 8 {
		var h uint32
		for _, b := range []byte(uuid) {
			h = h*31 + uint32(b)
		}
		return int(h & 0x7FFFFFFF)
	}
	b, err := hex.DecodeString(clean[:8])
	if err != nil {
		var h uint32
		for _, c := range []byte(uuid) {
			h = h*31 + uint32(c)
		}
		return int(h & 0x7FFFFFFF)
	}
	return int(binary.BigEndian.Uint32(b) & 0x7FFFFFFF)
}

// intIDStr parses a numeric ID from a URL parameter string.
func intIDStr(s string) (int, error) {
	var id int
	_, err := fmt.Sscanf(s, "%d", &id)
	return id, err
}
