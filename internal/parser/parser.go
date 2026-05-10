package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ebenderooock/loom/internal/languages"
)

// Release represents parsed metadata extracted from a release name.
type Release struct {
	Name            string   // Original release name
	Title           string   // Extracted clean title
	Codec           string   // h264, h265, av1, vp9, etc.
	Bitdepth        int      // 8, 10, 12
	Year            int      // Release year (YYYY format)
	Resolution      int      // 480, 720, 1080, 2160, etc.
	Source          string   // BluRay, HDTV, WebDL, DVDRip, etc.
	Season          int      // TV season number (-1 if not found)
	Episode         int      // TV episode number, first if multi (-1 if not found)
	Episodes        []int    // All episodes for multi-episode releases (e.g., [1,2,3])
	IsSeasonPack    bool     // True when season detected without episode (S01, Season 1 Complete)
	IsProper        bool     // PROPER tag detected
	IsRepack        bool     // REPACK or RERIP tag detected
	IsReal          bool     // REAL tag detected (fixes fake/nuked releases)
	DailyDate       string   // "2024-01-30" for daily shows (empty if not daily)
	AbsoluteEpisode int      // Anime absolute episode number (-1 if not found)
	Audio           string   // DTS-HD MA, TrueHD, Atmos, AAC, FLAC, AC3, EAC3, etc.
	Edition         string   // Movie edition: Director's Cut, Extended, IMAX, Theatrical, etc.
	Group           string   // Release group name (e.g., "SPARKS", "FGT")
	Languages       []string // Detected languages (e.g., ["English", "French"])
	IsMulti         bool     // MULTI tag detected (multiple audio tracks)
	IsDualAudio     bool     // Dual-audio release
	IsRemux         bool     // REMUX tag detected (lossless rip from disc)
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
		Name:            releaseName,
		Codec:           "",
		Bitdepth:        0,
		Year:            0,
		Resolution:      0,
		Source:          "",
		Season:          -1,
		Episode:         -1,
		AbsoluteEpisode: -1,
	}

	// Lowercase for pattern matching
	lower := strings.ToLower(releaseName)

	// Extract codec
	r.Codec = extractCodec(lower)

	// Extract bitdepth
	r.Bitdepth = extractBitdepth(lower)

	// Extract daily date BEFORE year (daily shows like "Show.2024.05.07")
	r.DailyDate = extractDailyDate(lower)

	// Extract year (skips daily date digits if already detected)
	r.Year = extractYear(lower)
	// If daily date found but year wasn't extracted separately, use the date's year
	if r.DailyDate != "" && r.Year == 0 {
		if len(r.DailyDate) >= 4 {
			if y, err := strconv.Atoi(r.DailyDate[:4]); err == nil {
				r.Year = y
			}
		}
	}

	// Extract resolution
	r.Resolution = extractResolution(lower)

	// Extract source
	r.Source = extractSource(lower)

	// Extract season and episode (for TV shows)
	season, episode, episodes, isSeasonPack := extractSeasonEpisodeV2(lower)
	r.Season = season
	r.Episode = episode
	r.Episodes = episodes
	r.IsSeasonPack = isSeasonPack

	// Extract anime absolute episode (only if no S##E## was found)
	if r.Season == -1 && r.Episode == -1 {
		r.AbsoluteEpisode = extractAbsoluteEpisode(lower, releaseName)
	}

	// Extract proper/repack/real/remux flags
	r.IsProper = extractFlag(lower, `(?:^|[\s.\-_])(proper)(?:$|[\s.\-_\d])`)
	r.IsRepack = extractFlag(lower, `(?:^|[\s.\-_])(repack|rerip)(?:$|[\s.\-_\d])`)
	r.IsReal = extractFlag(lower, `(?:^|[\s.\-_])(real)(?:$|[\s.\-_])`)
	r.IsRemux = extractFlag(lower, `(?:^|[\s.\-_])(remux|bdremux)(?:$|[\s.\-_])`)

	// Extract audio codec
	r.Audio = extractAudio(lower)

	// Extract movie edition (Director's Cut, Extended, IMAX, etc.)
	r.Edition = extractEdition(lower)

	// Extract title (everything before year or quality markers)
	r.Title = extractTitle(releaseName, r.Year)

	// Extract release group (last segment after a dash)
	r.Group = extractGroup(releaseName)

	// Extract languages via the dedicated language parser.
	langResult := languages.ParseTitle(releaseName)
	for _, lang := range langResult.Languages {
		r.Languages = append(r.Languages, lang.Name)
	}
	r.IsMulti = langResult.IsMulti
	r.IsDualAudio = langResult.IsDualAudio

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

// extractDailyDate detects daily show date patterns like "2024.05.07", "2024-05-07".
// Returns "YYYY-MM-DD" format or empty string.
func extractDailyDate(lower string) string {
	p := getPattern("daily_date", `(?:^|[\s.\-_])(\d{4})[\.\-\s](\d{2})[\.\-\s](\d{2})(?:$|[\s.\-_])`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 3 {
		year, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		// Validate it's a plausible date
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
		}
	}
	return ""
}

// extractSeasonEpisodeV2 extracts season, episode(s), and season pack info.
// Returns (season, firstEpisode, allEpisodes, isSeasonPack).
func extractSeasonEpisodeV2(lower string) (int, int, []int, bool) {
	season := -1
	episode := -1
	var episodes []int

	// S##E## pattern with multi-episode support:
	// S01E01E02E03, S01E01-E03, S01E01-03, S01E01.E02, S01E01+E02
	p := getPattern("sxe_multi", `s(\d{1,2})e(\d{1,3})`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 2 {
		if s, err := strconv.Atoi(matches[1]); err == nil {
			season = s
		}
		if e, err := strconv.Atoi(matches[2]); err == nil {
			episode = e
			episodes = append(episodes, e)
		}

		// Find the position after S##E## to scan for more episodes
		loc := p.FindStringIndex(lower)
		if loc != nil {
			rest := lower[loc[1]:]
			// Scan for continuation patterns: E##, -E##, .E##, +E##, -##
			contP := getPattern("episode_cont", `^(?:[.\-+]?e(\d{1,3})|[\-](\d{1,3}))`)
			for {
				m := contP.FindStringSubmatch(rest)
				if m == nil {
					break
				}
				var nextEp int
				isRange := false
				if m[1] != "" {
					nextEp, _ = strconv.Atoi(m[1])
				} else if m[2] != "" {
					nextEp, _ = strconv.Atoi(m[2])
					isRange = true
				}
				if nextEp > 0 {
					lastEp := episodes[len(episodes)-1]
					// Range: S01E01-03 or S01E01-E03 means episodes 1,2,3
					if nextEp > lastEp && nextEp-lastEp <= 24 && (isRange || strings.HasPrefix(rest, "-e") || strings.HasPrefix(rest, "-E")) {
						for ep := lastEp + 1; ep <= nextEp; ep++ {
							episodes = append(episodes, ep)
						}
					} else {
						episodes = append(episodes, nextEp)
					}
				}
				rest = rest[len(m[0]):]
			}
		}

		// Filter out version suffixes (e.g., E01v2 should not add episode 2)
		// handled by the pattern requiring E prefix or dash for continuation

		return season, episode, episodes, false
	}

	// ##x## pattern (e.g., 1x05, 7x01)
	p = getPattern("nxn", `(?:^|[\s._-])(\d{1,2})x(\d{1,3})(?:$|[\s._-])`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 2 {
		if s, err := strconv.Atoi(matches[1]); err == nil {
			season = s
		}
		if e, err := strconv.Atoi(matches[2]); err == nil {
			episode = e
			episodes = append(episodes, e)
		}
		return season, episode, episodes, false
	}

	// Season pack detection: S## without E## following
	// Patterns: "S01", "S01.Complete", "Season 1", "Season.01"
	spP := getPattern("season_pack", `(?:^|[\s._-])s(\d{1,2})(?:$|[\s._-])(?:complete|full|pack)?`)
	if matches := spP.FindStringSubmatch(lower); len(matches) > 1 {
		if s, err := strconv.Atoi(matches[1]); err == nil {
			// Verify no E## follows within a reasonable distance
			loc := spP.FindStringIndex(lower)
			if loc != nil {
				after := lower[loc[1]:]
				hasEpisode := regexp.MustCompile(`^\.?e\d`).MatchString(after)
				if !hasEpisode {
					season = s
					return season, -1, nil, true
				}
			}
		}
	}

	// "Season ##" word pattern
	swP := getPattern("season_word", `(?:^|[\s._-])season[\s._-]*(\d{1,2})(?:$|[\s._-])`)
	if matches := swP.FindStringSubmatch(lower); len(matches) > 1 {
		if s, err := strconv.Atoi(matches[1]); err == nil {
			season = s
			return season, -1, nil, true
		}
	}

	// "Episode ##" or "Ep ##" pattern
	p = getPattern("episode_word", `(?:episode|ep)[\s._-]*(\d{1,3})`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 1 {
		if e, err := strconv.Atoi(matches[1]); err == nil {
			episode = e
			episodes = append(episodes, e)
		}
		return season, episode, episodes, false
	}

	// Standalone E## pattern (e.g., E01, e05) with boundaries
	p = getPattern("standalone_e", `(?:^|[\s._-])e(\d{1,3})(?:$|[\s._-])`)
	if matches := p.FindStringSubmatch(lower); len(matches) > 1 {
		if e, err := strconv.Atoi(matches[1]); err == nil {
			episode = e
			episodes = append(episodes, e)
		}
	}

	return season, episode, episodes, false
}

// extractAbsoluteEpisode extracts anime-style absolute episode numbers.
// Only called when no S##E## pattern was found.
// Patterns: "- 05", "- 142", "[Group] Title - 05 [1080p]"
func extractAbsoluteEpisode(lower string, original string) int {
	// Require anime-style formatting: leading [Group] bracket suggests anime
	hasAnimeGroup := regexp.MustCompile(`^\[`).MatchString(strings.TrimSpace(original))

	if hasAnimeGroup {
		// Match "- ##" pattern (common anime: "[Group] Title - 05 [1080p]")
		p := getPattern("absolute_ep", `\s-\s(\d{1,4})(?:\s|$|\[)`)
		if matches := p.FindStringSubmatch(lower); len(matches) > 1 {
			if ep, err := strconv.Atoi(matches[1]); err == nil {
				// Sanity: episode numbers above 2000 are likely years
				if ep > 0 && ep < 2000 {
					return ep
				}
			}
		}
	}

	return -1
}

// extractFlag checks if a flag pattern exists in the release name.
func extractFlag(lower string, pattern string) bool {
	p := getPattern("flag_"+pattern, pattern)
	return p.MatchString(lower)
}

// extractAudio extracts audio codec information from release name.
// Matches longest/most specific patterns first to avoid partial matches.
func extractAudio(lower string) string {
	audioPatterns := []struct {
		pattern string
		name    string
	}{
		{`(?:dts[\s.\-]?hd[\s.\-]?ma)`, "DTS-HD MA"},
		{`(?:truehd[\s.\-]?atmos|atmos[\s.\-]?truehd)`, "TrueHD Atmos"},
		{`(?:dd[\+p][\s.\-]?atmos|eac3[\s.\-]?atmos)`, "EAC3 Atmos"},
		{`(?:dts[\s.\-]?x)`, "DTS-X"},
		{`(?:dts[\s.\-]?hd)`, "DTS-HD"},
		{`(?:truehd)`, "TrueHD"},
		{`(?:\batmos\b)`, "Atmos"},
		{`(?:dd[\+p]|eac3|e[\s.\-]?ac[\s.\-]?3)`, "EAC3"},
		{`(?:\bdts\b)`, "DTS"},
		{`(?:dd5[\.\s]?1|ac[\s.\-]?3|ac3)`, "AC3"},
		{`(?:\bflac\b)`, "FLAC"},
		{`(?:\baac\b)`, "AAC"},
		{`(?:\bopus\b)`, "Opus"},
		{`(?:\bpcm\b)`, "PCM"},
		{`(?:\bmp3\b)`, "MP3"},
	}

	for _, ap := range audioPatterns {
		if matched, _ := regexp.MatchString(ap.pattern, lower); matched {
			return ap.name
		}
	}

	return ""
}

// extractSeasonEpisode is kept for backward compatibility but delegates to V2.
// Deprecated: use extractSeasonEpisodeV2 instead.
func extractSeasonEpisode(lower string) (int, int) {
	s, e, _, _ := extractSeasonEpisodeV2(lower)
	return s, e
}

// extractEdition extracts movie edition information from release names.
// Based on Radarr's EditionRegex patterns.
func extractEdition(lower string) string {
	editionPatterns := []struct {
		pattern string
		name    string
	}{
		{`(?:directors?[\s.\-]?cut)`, "Directors Cut"},
		{`(?:collector'?s?[\s.\-]?(?:cut|edition))`, "Collectors Edition"},
		{`(?:theatrical[\s.\-]?(?:cut|edition)?)`, "Theatrical"},
		{`(?:ultimate[\s.\-]?(?:cut|edition))`, "Ultimate Cut"},
		{`(?:extended[\s.\-]?(?:cut|edition)?)`, "Extended"},
		{`(?:despecialized)`, "Despecialized"},
		{`(?:uncensored)`, "Uncensored"},
		{`(?:remastered)`, "Remastered"},
		{`(?:unrated[\s.\-]?(?:cut|edition)?)`, "Unrated"},
		{`(?:uncut)`, "Uncut"},
		{`(?:\bimax\b)`, "IMAX"},
		{`(?:fan[\s.\-]?edit)`, "Fan Edit"},
		{`(?:restored)`, "Restored"},
		{`(?:anniversary[\s.\-]?(?:edition)?)`, "Anniversary Edition"},
		{`(?:criterion[\s.\-]?(?:collection|edition)?)`, "Criterion"},
		{`(?:special[\s.\-]?edition)`, "Special Edition"},
		{`(?:limited[\s.\-]?edition)`, "Limited Edition"},
	}

	for _, ep := range editionPatterns {
		if matched, _ := regexp.MatchString(ep.pattern, lower); matched {
			return ep.name
		}
	}

	return ""
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

	// Find the cutoff point: year in parentheses, year standalone, or first quality/episode marker
	cutPatterns := []string{
		`(?i)(?:^|[\s._-])s\d{1,2}e\d{1,3}`,             // S##E## season+episode
		`(?i)(?:^|[\s._-])\d{1,2}x\d{1,3}`,              // ##x## season+episode
		`\s*[\(\[]?\d{4}[\)\]]?[\s\.\-_]`,                 // year with optional parens/brackets
		`(?i)\s*[\.\-\s_](?:720p?|1080p?|2160p?|4k|uhd)`, // resolution
		`(?i)\s*[\.\-\s_](?:bluray|brrip|webrip|webdl|web[\-\s]dl|hdtv|dvdrip|remux)`, // source
		`(?i)\s*[\.\-\s_](?:h\.?264|h\.?265|hevc|x\.?264|x\.?265|avc)`,               // codec
		`(?i)\s*[\.\-\s_](?:proper|repack|rerip|internal|limited|directors|extended)`,  // tags
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
