package anime

import "fmt"

// MapAbsoluteToSeason converts an absolute episode number to a
// season/episode pair using the supplied mappings. If no mapping covers
// the absolute number it returns (0, absoluteEp) as a passthrough.
func MapAbsoluteToSeason(absoluteEp int, mappings []EpisodeMapping) (season, episode int) {
	for _, m := range mappings {
		if m.AbsoluteNumber == absoluteEp {
			return m.SeasonNumber, m.EpisodeNumber
		}
	}
	return 0, absoluteEp
}

// MapSeasonToAbsolute converts a season/episode pair to an absolute
// number. Returns 0 when no mapping exists.
func MapSeasonToAbsolute(season, episode int, mappings []EpisodeMapping) int {
	for _, m := range mappings {
		if m.SeasonNumber == season && m.EpisodeNumber == episode {
			return m.AbsoluteNumber
		}
	}
	return 0
}

// GenerateOffsetMappings builds EpisodeMapping entries for a contiguous
// range. For example, GenerateOffsetMappings(2, 26, 25) creates
// absolute 26→S02E01 through absolute 50→S02E25.
func GenerateOffsetMappings(season, absoluteStart, count int) []EpisodeMapping {
	if count <= 0 {
		return nil
	}
	out := make([]EpisodeMapping, count)
	for i := range count {
		out[i] = EpisodeMapping{
			AbsoluteNumber: absoluteStart + i,
			SeasonNumber:   season,
			EpisodeNumber:  i + 1,
		}
	}
	return out
}

// ValidateMappings checks that every mapping has sensible values.
func ValidateMappings(mappings []EpisodeMapping) error {
	seen := make(map[int]bool, len(mappings))
	for _, m := range mappings {
		if m.AbsoluteNumber < 1 {
			return fmt.Errorf("invalid absolute number %d", m.AbsoluteNumber)
		}
		if m.SeasonNumber < 0 || m.EpisodeNumber < 1 {
			return fmt.Errorf("invalid season/episode %d/%d for absolute %d",
				m.SeasonNumber, m.EpisodeNumber, m.AbsoluteNumber)
		}
		if seen[m.AbsoluteNumber] {
			return fmt.Errorf("duplicate absolute number %d", m.AbsoluteNumber)
		}
		seen[m.AbsoluteNumber] = true
	}
	return nil
}
