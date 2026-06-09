package music

import (
	"encoding/json"
	"strings"
)

// AudioProfileItem is one entry in an AudioQualityProfile's Items list. It marks
// a quality definition as allowed (or not) for acquisition.
type AudioProfileItem struct {
	DefinitionID string `json:"definition_id"`
	Allowed      bool   `json:"allowed"`
}

// AudioScore is the result of evaluating a parsed release against the known
// quality definitions and (optionally) a quality profile.
type AudioScore struct {
	Definition  *AudioQualityDefinition // matched tier, nil if unmatched
	Tier        int                     // tier_order of the matched definition
	Allowed     bool                    // permitted by the profile (true if no profile)
	MeetsCutoff bool                    // tier >= profile cutoff tier
	Tiebreaker  float64                 // seeders/size based, higher = better
}

// Composite combines tier and tiebreaker into a single comparable score. Higher
// is better. Quality tier dominates; the tiebreaker only orders equal tiers.
func (s AudioScore) Composite() float64 {
	return float64(s.Tier)*1000 + s.Tiebreaker
}

// MatchAudioQuality maps a parsed release to the best-fitting quality
// definition from defs. Returns nil when defs is empty.
func MatchAudioQuality(rel *MusicRelease, defs []*AudioQualityDefinition) *AudioQualityDefinition {
	if rel == nil || len(defs) == 0 {
		return nil
	}
	byID := make(map[string]*AudioQualityDefinition, len(defs))
	for _, d := range defs {
		byID[d.ID] = d
	}

	format := strings.ToLower(rel.Format)
	switch {
	case rel.Lossless || format == "flac" || format == "alac" || format == "ape" || format == "wav" || format == "aiff" || format == "dsd":
		if rel.Bits >= 24 {
			if d := byID["aq_flac_24"]; d != nil {
				return d
			}
		}
		if d := bestLossless(defs); d != nil {
			return d
		}
	case format == "mp3":
		return matchMP3(rel, byID)
	case format == "aac" || format == "m4a":
		if rel.Bitrate >= 320 {
			if d := byID["aq_aac_320"]; d != nil {
				return d
			}
		}
		if d := byID["aq_aac_256"]; d != nil {
			return d
		}
	}

	// Fall back to a generic bitrate match against any format, else Unknown.
	if d := matchByBitrate(rel, defs); d != nil {
		return d
	}
	return byID["aq_unknown"]
}

func matchMP3(rel *MusicRelease, byID map[string]*AudioQualityDefinition) *AudioQualityDefinition {
	if rel.VBR {
		switch rel.VBRTag {
		case "V0", "APX":
			if d := byID["aq_mp3_v0"]; d != nil {
				return d
			}
		case "V2", "APS":
			if d := byID["aq_mp3_v2"]; d != nil {
				return d
			}
		}
	}
	switch {
	case rel.Bitrate >= 320:
		if d := byID["aq_mp3_320"]; d != nil {
			return d
		}
	case rel.Bitrate >= 256:
		if d := byID["aq_mp3_256"]; d != nil {
			return d
		}
	case rel.Bitrate >= 128:
		if d := byID["aq_mp3_128"]; d != nil {
			return d
		}
	}
	// MP3 of unknown bitrate: assume a conservative low tier if present.
	if d := byID["aq_mp3_128"]; d != nil {
		return d
	}
	return byID["aq_unknown"]
}

// matchByBitrate finds the highest-tier lossy definition whose bitrate is <= the
// release bitrate (closest tier at or below the release).
func matchByBitrate(rel *MusicRelease, defs []*AudioQualityDefinition) *AudioQualityDefinition {
	if rel.Bitrate <= 0 {
		return nil
	}
	var best *AudioQualityDefinition
	for _, d := range defs {
		if d.Lossless || d.Bitrate == 0 {
			continue
		}
		if d.Bitrate <= rel.Bitrate {
			if best == nil || d.TierOrder > best.TierOrder {
				best = d
			}
		}
	}
	return best
}

// bestLossless returns the lowest-tier (non-24bit) lossless definition, i.e. the
// canonical FLAC tier.
func bestLossless(defs []*AudioQualityDefinition) *AudioQualityDefinition {
	var best *AudioQualityDefinition
	for _, d := range defs {
		if !d.Lossless || d.ID == "aq_flac_24" {
			continue
		}
		if best == nil || d.TierOrder < best.TierOrder {
			best = d
		}
	}
	if best != nil {
		return best
	}
	// Fall back to any lossless definition.
	for _, d := range defs {
		if d.Lossless {
			return d
		}
	}
	return nil
}

// ScoreAudioRelease evaluates a parsed release against the quality definitions
// and an optional profile, returning a comparable AudioScore. seeders/size feed
// the tiebreaker. A nil profile means "accept all qualities".
func ScoreAudioRelease(rel *MusicRelease, defs []*AudioQualityDefinition, profile *AudioQualityProfile, seeders int, size int64) AudioScore {
	score := AudioScore{Allowed: true}
	def := MatchAudioQuality(rel, defs)
	if def != nil {
		score.Definition = def
		score.Tier = def.TierOrder
	}

	if profile != nil {
		allowedIDs, cutoffTier, hasItems := parseProfile(profile, defs)
		if hasItems {
			if def == nil || !allowedIDs[def.ID] {
				score.Allowed = false
			}
		}
		score.MeetsCutoff = def != nil && def.TierOrder >= cutoffTier
	} else {
		score.MeetsCutoff = true
	}

	score.Tiebreaker = audioTiebreaker(seeders, size)
	return score
}

// parseProfile returns the set of allowed definition IDs, the cutoff tier order,
// and whether the profile declared any items.
func parseProfile(profile *AudioQualityProfile, defs []*AudioQualityDefinition) (allowed map[string]bool, cutoffTier int, hasItems bool) {
	allowed = make(map[string]bool)
	var items []AudioProfileItem
	if len(profile.Items) > 0 {
		_ = json.Unmarshal(profile.Items, &items)
	}
	for _, it := range items {
		if it.Allowed {
			allowed[it.DefinitionID] = true
		}
	}
	hasItems = len(allowed) > 0
	if profile.Cutoff != "" {
		for _, d := range defs {
			if d.ID == profile.Cutoff {
				cutoffTier = d.TierOrder
				break
			}
		}
	}
	return allowed, cutoffTier, hasItems
}

// audioTiebreaker scores torrents by seeders (primary) with a small size nudge,
// keeping the value well under the 1000-per-tier band used by Composite.
func audioTiebreaker(seeders int, size int64) float64 {
	t := float64(seeders)
	if t > 500 {
		t = 500
	}
	// Tiny size contribution (MB/1e6) to break exact seeder ties deterministically.
	t += float64(size) / 1e12
	return t
}
