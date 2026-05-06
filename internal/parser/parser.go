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
	Title      string // Extracted clean title
	Codec      string // h264, h265, av1, vp9, etc.
	Bitdepth   int    // 8, 10, 12
	Year       int    // Release year (YYYY format)
	Resolution int    // 480, 720, 1080, 2160, etc.
	Source     string // BluRay, HDTV, WebDL, DVDRip, etc.
	Season     int    // TV season number (-1 if not found)
	Episode    int    // TV episode number (-1 if not found)
	Group      string // Release group name (e.g., "SPARKS", "FGT")
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

	// Extract title (everything before year or quality markers)
	r.Title = extractTitle(releaseName, r.Year)

	// Extract release group (last segment after a dash)
	r.Group = extractGroup(releaseName)

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

// extractTitle extracts a clean movie/show title from a release name.
// It strips everything from the year or first quality/codec marker onward,
// then cleans up separators.
func extractTitle(name string, year int) string {
	clean := name

	// Remove file extension
	if idx := strings.LastIndex(clean, "."); idx > 0 {
		ext := strings.ToLower(clean[idx:])
		if ext == ".mkv" || ext == ".mp4" || ext == ".avi" || ext == ".m4v" || ext == ".wmv" || ext == ".ts" || ext == ".flv" || ext == ".mov" {
			clean = clean[:idx]
		}
	}

	// Remove content in square brackets at the start (group tags like [GroupName])
	p := getPattern("title_leading_bracket", `^\[.*?\]\s*`)
	clean = p.ReplaceAllString(clean, "")

	// Find the cutoff point: year in parentheses, year standalone, or first quality marker
	cutPatterns := []string{
		`\s*[\(\[]?\d{4}[\)\]]?[\s\.\-]`,        // year with optional parens/brackets
		`(?i)\s*[\.\-\s](?:720p?|1080p?|2160p?|4k|uhd)`, // resolution
		`(?i)\s*[\.\-\s](?:bluray|brrip|webrip|webdl|web[\-\s]dl|hdtv|dvdrip|remux)`, // source
		`(?i)\s*[\.\-\s](?:h\.?264|h\.?265|hevc|x\.?264|x\.?265|avc)`,               // codec
		`(?i)\s*[\.\-\s](?:proper|repack|rerip|internal|limited|directors|extended)`,  // tags
	}

	cutIdx := len(clean)
	for _, pat := range cutPatterns {
		re := regexp.MustCompile(pat)
		if loc := re.FindStringIndex(clean); loc != nil && loc[0] < cutIdx {
			cutIdx = loc[0]
		}
	}
	clean = clean[:cutIdx]

	// Replace common separators with spaces
	clean = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(clean)

	// Collapse whitespace and trim
	spaceRe := getPattern("multi_space", `\s+`)
	clean = strings.TrimSpace(spaceRe.ReplaceAllString(clean, " "))

	return clean
}

// extractGroup returns the release group from a release name.
// Patterns: "Title-GROUP", "Title-GROUP.mkv", "[GROUP] Title", "Title [GROUP]".
func extractGroup(name string) string {
	// Strip file extension first
	clean := name
	if idx := strings.LastIndex(clean, "."); idx > 0 {
		ext := strings.ToLower(clean[idx:])
		switch ext {
		case ".mkv", ".mp4", ".avi", ".m4v", ".wmv", ".ts", ".flv", ".mov", ".nzb", ".torrent":
			clean = clean[:idx]
		}
	}

	// Pattern 1: trailing "-GROUP" (most common, e.g. "Movie.2024.1080p.BluRay-SPARKS")
	re := getPattern("group_trailing", `\-([A-Za-z0-9]+)$`)
	if m := re.FindStringSubmatch(clean); m != nil {
		g := m[1]
		// Reject false positives that are common quality/codec markers
		lower := strings.ToLower(g)
		switch lower {
		case "dl", "rip", "mux", "remux", "264", "265", "hevc", "avc", "internal":
			// not a group
		default:
			return g
		}
	}

	// Pattern 2: leading "[GROUP]" (common in anime)
	re2 := getPattern("group_leading", `^\[([^\]]+)\]`)
	if m := re2.FindStringSubmatch(clean); m != nil {
		return m[1]
	}

	return ""
}
