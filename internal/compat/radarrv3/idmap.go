package radarrv3

import (
	"encoding/hex"
	"strconv"
	"sync"
)

// idMapper provides bidirectional mapping between Loom string IDs and
// Radarr-style integer IDs. String→int uses the first 8 hex characters
// of the UUID parsed as a uint32. Int→string is cached for reverse lookup.
type idMapper struct {
	mu    sync.RWMutex
	toStr map[int]string
}

func newIDMapper() *idMapper {
	return &idMapper{toStr: make(map[int]string)}
}

// toInt converts a Loom string ID to a stable Radarr integer ID and
// caches the reverse mapping.
func (m *idMapper) toInt(id string) int {
	n := hashStringID(id)
	m.mu.Lock()
	m.toStr[n] = id
	m.mu.Unlock()
	return n
}

// toString looks up the original Loom string ID for a Radarr integer ID.
func (m *idMapper) toString(n int) (string, bool) {
	m.mu.RLock()
	s, ok := m.toStr[n]
	m.mu.RUnlock()
	return s, ok
}

// hashStringID takes the first 4 bytes of the hex-decoded string ID
// and returns a positive int. Falls back to a simple hash for non-hex IDs.
func hashStringID(id string) int {
	clean := stripHyphens(id)
	if len(clean) >= 8 {
		b, err := hex.DecodeString(clean[:8])
		if err == nil && len(b) == 4 {
			v := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
			if v < 0 {
				v = -v
			}
			if v == 0 {
				v = 1
			}
			return v
		}
	}
	// Fallback: simple FNV-like hash.
	var h uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		h ^= uint32(id[i])
		h *= 16777619
	}
	v := int(h & 0x7FFFFFFF)
	if v == 0 {
		v = 1
	}
	return v
}

func stripHyphens(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// parseRadarrID parses a Radarr integer ID from a URL parameter string.
func parseRadarrID(s string) (int, error) {
	return strconv.Atoi(s)
}
