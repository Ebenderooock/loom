package qualityprofiles

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/parser"
)

// QualityItem is the JSON structure stored in QualityProfile.Items.
type QualityItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Preferred bool   `json:"preferred"`
	Allowed   bool   `json:"allowed"`
}

// ParseItems deserialises the Items JSON into a slice of QualityItem.
func (qp *QualityProfile) ParseItems() ([]QualityItem, error) {
	if qp.Items == "" || qp.Items == "[]" {
		return nil, nil
	}
	var items []QualityItem
	if err := json.Unmarshal([]byte(qp.Items), &items); err != nil {
		return nil, fmt.Errorf("parse quality profile items: %w", err)
	}
	return items, nil
}

// Rank returns the positional rank (0-based, lower is worse) of a quality
// name within this profile's allowed items. Returns -1 if the quality is
// not in the profile.
func (qp *QualityProfile) Rank(qualityName string) int {
	items, err := qp.ParseItems()
	if err != nil || len(items) == 0 {
		return -1
	}
	norm := strings.ToLower(qualityName)
	for i, it := range items {
		if it.Allowed && strings.ToLower(it.Name) == norm {
			return i
		}
	}
	return -1
}

// IsAllowed reports whether a quality name is allowed in this profile.
func (qp *QualityProfile) IsAllowed(qualityName string) bool {
	return qp.Rank(qualityName) >= 0
}

// CutoffRank returns the rank of the profile's cutoff quality.
// Qualities at or above this rank need no further upgrading.
func (qp *QualityProfile) CutoffRank() int {
	items, err := qp.ParseItems()
	if err != nil || len(items) == 0 {
		return -1
	}
	for i, it := range items {
		if it.ID == qp.Cutoff {
			return i
		}
	}
	return -1
}

// QualityNameFromRelease derives the quality-definition name (e.g.
// "webdl-1080p", "bluray-2160p-remux") from a parsed release. This
// matches the naming scheme used in the seed profiles.
func QualityNameFromRelease(r *parser.Release) string {
	source := normaliseSource(r.Source)
	res := normaliseResolution(r.Resolution)

	if source == "" && res == "" {
		return ""
	}
	if source == "" {
		source = "unknown"
	}
	if res == "" {
		res = "unknown"
	}

	name := source + "-" + res
	if r.IsRemux {
		name += "-remux"
	}
	return name
}

func normaliseSource(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "bluray", "blu-ray", "bdrip", "brrip":
		return "bluray"
	case "webdl", "web-dl", "web dl":
		return "webdl"
	case "webrip", "web-rip":
		return "webrip"
	case "hdtv":
		return "hdtv"
	case "dvdrip", "dvd":
		return "dvd"
	case "sdtv":
		return "sdtv"
	case "":
		return ""
	default:
		return s
	}
}

func normaliseResolution(r int) string {
	switch {
	case r >= 2160:
		return "2160p"
	case r >= 1080:
		return "1080p"
	case r >= 720:
		return "720p"
	case r >= 480:
		return "480p"
	case r > 0:
		return fmt.Sprintf("%dp", r)
	default:
		return ""
	}
}
