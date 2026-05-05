package anime

import (
	"regexp"
	"strconv"
	"strings"
)

// Compiled patterns for anime release name parsing.
var (
	// [SubGroup] Title - 01 [1080p]
	reGroupBracket = regexp.MustCompile(`^\[([^\]]+)\]`)

	// Absolute episode: " - 01", " - 001", " - 1234"
	reAbsoluteEp = regexp.MustCompile(`(?:^|[\s\-])(\d{2,4})(?:\s*v\d)?(?:\s|$|\[|\()`)

	// S01E01 with optional absolute: S01E01 - 001
	reSxE = regexp.MustCompile(`(?i)s(\d{1,2})e(\d{1,3})`)

	// Absolute after SxE: S01E01 - 001
	reAbsoluteAfterSxE = regexp.MustCompile(`(?i)s\d{1,2}e\d{1,3}\s*-\s*(\d{2,4})`)

	// Version: v2, v3 (can appear directly after digits)
	reVersion = regexp.MustCompile(`(?i)v(\d)(?:\b|\s|$|\[|\])`)

	// Dual audio patterns
	reDualAudio = regexp.MustCompile(`(?i)(?:dual[\s\-]?audio|dual)`)

	// Multi audio
	reMultiAudio = regexp.MustCompile(`(?i)multi[\s\-]?audio`)

	// Batch
	reBatch = regexp.MustCompile(`(?i)(?:\[batch\]|batch|complete[\s\-]?series|complete)`)

	// Resolution
	reResolution = regexp.MustCompile(`(?i)(\d{3,4})p`)

	// Source
	reSources = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`(?i)(?:blu[\s\-]?ray|brrip|bdremux|bdmv)`), "BluRay"},
		{regexp.MustCompile(`(?i)(?:webdl|web[\s\-]?dl)`), "WebDL"},
		{regexp.MustCompile(`(?i)(?:webrip|web[\s\-]?rip|web)`), "WebRip"},
		{regexp.MustCompile(`(?i)(?:hdtv)`), "HDTV"},
		{regexp.MustCompile(`(?i)(?:dvdrip|dvd[\s\-]?rip|dvd)`), "DVD"},
	}

	// Codec
	reCodecs = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`(?i)(?:h\.?265|hevc|x\.?265)`), "h265"},
		{regexp.MustCompile(`(?i)(?:h\.?264|avc|x\.?264)`), "h264"},
		{regexp.MustCompile(`(?i)(?:av1|av01)`), "av1"},
	}

	// Year in title
	reYear = regexp.MustCompile(`(?:^|\s)\(?(\d{4})\)?(?:\s|$|\[)`)

	// Title cleanup: strip everything from the first quality/tag marker onward
	reTitleCut = regexp.MustCompile(`(?i)[\s\-]*(?:\d{3,4}p|h\.?26[45]|hevc|avc|x\.?26[45]|av1|blu[\s\-]?ray|webdl|web[\s\-]?dl|webrip|web[\s\-]?rip|hdtv|flac|aac|opus|dual[\s\-]?audio|multi[\s\-]?audio|10[\s\-]?bit|batch|v\d).*$`)

	// Trailing brackets/parens
	reTrailingBrackets = regexp.MustCompile(`\s*[\[\(][^\]\)]*[\]\)]$`)
)

// Parse parses an anime release name and returns structured metadata.
func Parse(name string) *AnimeRelease {
	r := &AnimeRelease{
		Name:    name,
		Season:  -1,
		Episode: -1,
		Version: 1,
		SubType: SubTypeSoftsub, // default for anime
	}

	// Extract release group from leading [GroupName]
	if m := reGroupBracket.FindStringSubmatch(name); len(m) > 1 {
		r.ReleaseGroup = m[1]
	}

	// Strip leading group tag for further parsing
	working := name
	if r.ReleaseGroup != "" {
		working = strings.TrimPrefix(working, "["+r.ReleaseGroup+"]")
		working = strings.TrimSpace(working)
	}

	// Season/Episode (S01E01)
	if m := reSxE.FindStringSubmatch(name); len(m) > 2 {
		if s, err := strconv.Atoi(m[1]); err == nil {
			r.Season = s
		}
		if e, err := strconv.Atoi(m[2]); err == nil {
			r.Episode = e
		}
	}

	// Absolute episode after SxE: "S01E01 - 013"
	if m := reAbsoluteAfterSxE.FindStringSubmatch(name); len(m) > 1 {
		if abs, err := strconv.Atoi(m[1]); err == nil {
			r.AbsoluteEpisode = abs
		}
	}

	// If no absolute from SxE suffix, try standalone absolute number
	if r.AbsoluteEpisode == 0 {
		r.AbsoluteEpisode = extractAbsoluteEpisode(working)
	}

	// If we got absolute but no season/episode, set episode = absolute, season = 1
	if r.AbsoluteEpisode > 0 && r.Episode == -1 {
		r.Episode = r.AbsoluteEpisode
		r.Season = 1
	}

	// Version
	if m := reVersion.FindStringSubmatch(name); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			r.Version = v
		}
	}

	// Audio
	r.IsDualAudio = reDualAudio.MatchString(name)
	r.IsMultiAudio = reMultiAudio.MatchString(name)

	// Subtitle type
	lower := strings.ToLower(name)
	if strings.Contains(lower, "hardsub") {
		r.SubType = SubTypeHardsub
	} else if strings.Contains(lower, "raw") && !strings.Contains(lower, "erai-raws") {
		r.SubType = SubTypeRaw
	}

	// Batch
	r.IsBatch = reBatch.MatchString(name)

	// Resolution
	if m := reResolution.FindStringSubmatch(name); len(m) > 1 {
		if res, err := strconv.Atoi(m[1]); err == nil {
			r.Resolution = res
		}
	}
	if r.Resolution == 0 && strings.Contains(lower, "4k") {
		r.Resolution = 2160
	}

	// Source
	for _, src := range reSources {
		if src.re.MatchString(name) {
			r.Source = src.name
			break
		}
	}

	// Codec
	for _, c := range reCodecs {
		if c.re.MatchString(name) {
			r.Codec = c.name
			break
		}
	}

	// Year
	if m := reYear.FindStringSubmatch(name); len(m) > 1 {
		if y, err := strconv.Atoi(m[1]); err == nil && y >= 1900 && y <= 2100 {
			r.Year = y
		}
	}

	// Title: strip group, episode number, and quality markers
	r.Title = extractAnimeTitle(working)

	return r
}

// extractAbsoluteEpisode finds a standalone absolute episode number.
// Looks for " - 01", " - 001" patterns typical of anime naming.
func extractAbsoluteEpisode(s string) int {
	// Pattern: " - 01" or " - 001"
	re := regexp.MustCompile(`\s-\s(\d{2,4})(?:\s|$|\[|\()`)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

// extractAnimeTitle cleans the title from an anime release name.
func extractAnimeTitle(working string) string {
	title := working

	// Remove episode number pattern " - 01 " or " - 001"
	reEpDash := regexp.MustCompile(`\s-\s\d{2,4}(?:\s.*|$)`)
	title = reEpDash.ReplaceAllString(title, "")

	// Remove SxE
	title = reSxE.ReplaceAllString(title, "")

	// Remove quality/tag markers onward
	title = reTitleCut.ReplaceAllString(title, "")

	// Remove trailing brackets
	for reTrailingBrackets.MatchString(title) {
		title = reTrailingBrackets.ReplaceAllString(title, "")
	}

	// Clean separators
	title = strings.TrimRight(title, " -_.")
	title = strings.TrimSpace(title)

	return title
}
