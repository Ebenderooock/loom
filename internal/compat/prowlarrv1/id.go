package prowlarrv1

import (
	"encoding/hex"
	"encoding/binary"
	"fmt"
	"strings"
)

// intID derives a deterministic positive int from a string UUID by
// reading the first 8 hex characters as a uint32. This mirrors the
// strategy used by other compat shims.
func intID(uuid string) int {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) < 8 {
		// Fallback: hash the whole string.
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

// findIDByInt scans defs looking for the Definition whose intID
// matches the given numeric id. Returns the string UUID and true if
// found.
func findIDByInt(defs []defPair, target int) (string, bool) {
	for _, d := range defs {
		if d.numID == target {
			return d.strID, true
		}
	}
	return "", false
}

type defPair struct {
	strID string
	numID int
}

// intIDStr parses the URL parameter and returns the numeric value.
func intIDStr(s string) (int, error) {
	var id int
	_, err := fmt.Sscanf(s, "%d", &id)
	return id, err
}
