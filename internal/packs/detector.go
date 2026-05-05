package packs

import (
	"regexp"
	"strconv"
	"strings"
)

// Patterns for release-name pack detection.
var (
	// "Series.Name.S01-S03" or "Series.Name.S01-S03.720p"
	reMultiSeason = regexp.MustCompile(`(?i)[.\s]S(\d{1,2})\s*-\s*S(\d{1,2})(?:[.\s]|$)`)

	// "Series.Name.S02.Complete" or "Series.Name.S02.COMPLETE"
	reSingleSeasonComplete = regexp.MustCompile(`(?i)[.\s]S(\d{1,2})[.\s](?:Complete|COMPLETE|Full)(?:[.\s]|$)`)

	// "Series.Name.Complete.Series" or "Series.Name.Complete.Collection"
	reCompleteSeries = regexp.MustCompile(`(?i)[.\s](?:Complete[.\s](?:Series|Collection|Pack)|Integrale|Integrala)(?:[.\s]|$)`)

	// "Series.Name.S01E01-E12" or "S01E01-E24"
	reEpisodeRange = regexp.MustCompile(`(?i)[.\s]S(\d{1,2})E(\d{1,4})\s*-\s*E(\d{1,4})(?:[.\s]|$)`)

	// Simple full-season: "S01" without episode number
	reFullSeason = regexp.MustCompile(`(?i)[.\s]S(\d{1,2})(?:[.\s]|$)`)

	// Negative lookahead – if SxxExx is present it's a single episode
	reSingleEpisode = regexp.MustCompile(`(?i)S\d{1,2}E\d{1,4}(?:\s*-\s*E\d{1,4})?`)
)

// Detect parses a release title and returns pack information.
func Detect(title string) DetectedPack {
	d := DetectedPack{Title: title}

	// Multi-season: S01-S03
	if m := reMultiSeason.FindStringSubmatch(title); len(m) == 3 {
		s1, _ := strconv.Atoi(m[1])
		s2, _ := strconv.Atoi(m[2])
		d.Type = PackTypeMultiSeason
		d.SeasonStart = s1
		d.SeasonEnd = s2
		d.IsPack = true
		return d
	}

	// Complete series
	if reCompleteSeries.MatchString(title) {
		d.Type = PackTypeCompleteSeries
		d.SeasonStart = -1
		d.SeasonEnd = -1
		d.IsPack = true
		return d
	}

	// Episode range: S01E01-E12
	if m := reEpisodeRange.FindStringSubmatch(title); len(m) == 4 {
		s, _ := strconv.Atoi(m[1])
		e1, _ := strconv.Atoi(m[2])
		e2, _ := strconv.Atoi(m[3])
		d.Type = PackTypeEpisodeRange
		d.SeasonStart = s
		d.SeasonEnd = s
		d.EpisodeStart = e1
		d.EpisodeEnd = e2
		d.IsPack = true
		return d
	}

	// Single season complete: S02.Complete
	if m := reSingleSeasonComplete.FindStringSubmatch(title); len(m) == 2 {
		s, _ := strconv.Atoi(m[1])
		d.Type = PackTypeSingleSeason
		d.SeasonStart = s
		d.SeasonEnd = s
		d.IsPack = true
		return d
	}

	// Full season (S01 without episode)
	if m := reFullSeason.FindStringSubmatch(title); len(m) == 2 {
		// Only treat as pack if there's no single-episode marker
		if !reSingleEpisode.MatchString(title) {
			s, _ := strconv.Atoi(m[1])
			d.Type = PackTypeSingleSeason
			d.SeasonStart = s
			d.SeasonEnd = s
			d.IsPack = true
			return d
		}
	}

	return d
}

// IsPack returns true if the release title looks like a season/multi-season pack.
func IsPack(title string) bool {
	return Detect(title).IsPack
}

// CleanSeriesTitle extracts a rough series name from a release title by
// stripping quality/season/episode tokens.
func CleanSeriesTitle(title string) string {
	// Replace dots and underscores with spaces
	clean := strings.NewReplacer(".", " ", "_", " ").Replace(title)
	// Remove season/episode markers and everything after
	for _, re := range []*regexp.Regexp{reMultiSeason, reSingleSeasonComplete, reCompleteSeries, reEpisodeRange, reFullSeason} {
		loc := re.FindStringIndex(clean)
		if loc != nil {
			clean = clean[:loc[0]]
		}
	}
	return strings.TrimSpace(clean)
}
