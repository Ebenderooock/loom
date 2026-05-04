package parser

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Release represents parsed metadata extracted from a release name.
type Release struct {
	Name       string // Original release name
	Codec      string // h264, h265, av1, vp9, etc.
	Bitdepth   int    // 8, 10, 12
	Year       int    // Release year (YYYY format)
	Resolution int    // 480, 720, 1080, 2160, etc.
	Source     string // BluRay, HDTV, WebDL, DVDRip, etc.
	Season     int    // TV season number (-1 if not found)
	Episode    int    // TV episode number (-1 if not found)
}

// patternCache holds compiled regex patterns to avoid recompilation.
var patternCache struct {
	sync.RWMutex
	patterns map[string]*regexp.Regexp
}

func init() {
	patternCache.patterns = make(map[string]*regexp.Regexp)
}

// getPattern returns a compiled regex pattern from cache or compiles and caches it.
func getPattern(key string, pattern string) *regexp.Regexp {
	patternCache.RLock()
	if p, ok := patternCache.patterns[key]; ok {
		patternCache.RUnlock()
		return p
	}
	patternCache.RUnlock()

	p := regexp.MustCompile(pattern)
	patternCache.Lock()
	patternCache.patterns[key] = p
	patternCache.Unlock()
	return p
}

// Parse parses a release name and extracts metadata.
func Parse(releaseName string) *Release {
	r := &Release{
		Name:       releaseName,
		Codec:      "",
		Bitdepth:   0,
		Year:       0,
		Resolution: 0,
		Source:     "",
		Season:     -1,
		Episode:    -1,
	}

	// Lowercase for pattern matching
	lower := strings.ToLower(releaseName)

	// Extract codec
	r.Codec = extractCodec(lower)

	// Extract bitdepth
	r.Bitdepth = extractBitdepth(lower)

	// Extract year
	r.Year = extractYear(lower)

	// Extract resolution
	r.Resolution = extractResolution(lower)

	// Extract source
	r.Source = extractSource(lower)

	// Extract season and episode (for TV shows)
	season, episode := extractSeasonEpisode(lower)
	r.Season = season
	r.Episode = episode

	return r
}

// extractCodec extracts video codec from release name.
func extractCodec(lower string) string {
	// h265/hevc patterns
	if matched, _ := regexp.MatchString(`(?:h\.?265|hevc|x\.?265)`, lower); matched {
		return "h265"
	}
	// h264/avc patterns
	if matched, _ := regexp.MatchString(`(?:h\.?264|avc|x\.?264)`, lower); matched {
		return "h264"
	}
	// VP9
	if matched, _ := regexp.MatchString(`(?:vp9)`, lower); matched {
		return "vp9"
	}
	// AV1
	if matched, _ := regexp.MatchString(`(?:av1|av01)`, lower); matched {
		return "av1"
	}
	// MPEG-2 (common in DVDRip and HDTV)
	if matched, _ := regexp.MatchString(`(?:mpeg2)`, lower); matched {
		return "mpeg2"
	}
	return ""
}

// extractBitdepth extracts video bitdepth (8, 10, 12 bit).
func extractBitdepth(lower string) int {
	// 12-bit patterns
	if matched, _ := regexp.MatchString(`(?:12[\s\-]?bit|12bit)`, lower); matched {
		return 12
	}
	// 10-bit patterns (hi10p, 10-bit, etc.)
	if matched, _ := regexp.MatchString(`(?:10[\s\-]?bit|10bit|hi10p|hi10)`, lower); matched {
		return 10
	}
	return 0 // Default to 8-bit (implicit)
}

// extractYear extracts year in [YYYY] or YYYY format.
func extractYear(lower string) int {
	// [YYYY] format
	p := getPattern("year_bracket", `\[(\d{4})\]`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 1 {
		if year, err := strconv.Atoi(matches[1]); err == nil {
			return year
		}
	}

	// YYYY format (4 consecutive digits, typically year-like)
	// Use lookahead/lookbehind to avoid false positives in resolution/bitrate
	p = getPattern("year_standard", `(?:^|[\s\-\.]|19|20)(\d{4})(?:$|[\s\-\.]|[\.\s]rip)`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 1 {
		if year, err := strconv.Atoi(matches[1]); err == nil {
			y := year
			// Sanity check: reasonable year range
			if y >= 1900 && y <= 2100 {
				return y
			}
		}
	}

	return 0
}

// extractResolution extracts video resolution (480, 720, 1080, 2160, etc.).
func extractResolution(lower string) int {
	// 4K / 2160p patterns
	if matched, _ := regexp.MatchString(`(?:4k|2160p?)`, lower); matched {
		return 2160
	}
	// 1080p patterns
	if matched, _ := regexp.MatchString(`1080p?`, lower); matched {
		return 1080
	}
	// 720p patterns
	if matched, _ := regexp.MatchString(`720p?`, lower); matched {
		return 720
	}
	// 480p patterns
	if matched, _ := regexp.MatchString(`480p?`, lower); matched {
		return 480
	}
	// 360p patterns
	if matched, _ := regexp.MatchString(`360p?`, lower); matched {
		return 360
	}

	return 0 // Unknown/SD
}

// extractSource extracts release source (BluRay, HDTV, WebDL, DVDRip, etc.).
func extractSource(lower string) string {
	// Check patterns in order of specificity
	// NOTE: Order matters! More specific patterns must come before general ones
	sources := []struct {
		pattern string
		name    string
	}{
		{`(?:blu[\s\-]?ray|brrip)`, "BluRay"},
		{`(?:webdl|web[\s\-]?dl)`, "WebDL"},  // Must come before WebRip (more specific)
		{`(?:webrip|web[\s\-]?rip|web)`, "WebRip"},
		{`(?:hdtv|hd[\s\-]?tv)`, "HDTV"},
		{`(?:dvdrip|dvd[\s\-]?rip)`, "DVDRip"},
		{`(?:dvd)`, "DVD"},
		{`(?:bdrip|bd[\s\-]?rip)`, "BluRay"},
		{`(?:uhd)`, "UHD"},
		{`(?:remux)`, "Remux"},
		{`(?:tvrip|tv[\s\-]?rip)`, "TVRip"},
	}

	for _, src := range sources {
		if matched, _ := regexp.MatchString(src.pattern, lower); matched {
			return src.name
		}
	}

	return ""
}

// extractSeasonEpisode extracts season and episode numbers from release name.
// Returns (season, episode) where -1 means not found.
func extractSeasonEpisode(lower string) (int, int) {
	season := -1
	episode := -1

	// S##E## pattern (e.g., S01E05, s1e5)
	p := getPattern("sxe", `s(\d{1,2})e(\d{1,2})`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 2 {
		if s, err := strconv.Atoi(matches[1]); err == nil {
			season = s
		}
		if e, err := strconv.Atoi(matches[2]); err == nil {
			episode = e
		}
	}

	return season, episode
}
