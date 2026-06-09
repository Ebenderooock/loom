package music

import (
	"regexp"
	"strconv"
	"strings"
)

// MusicRelease is the parsed form of an indexer release title for a music
// release. Fields are best-effort; zero values mean "not detected".
type MusicRelease struct {
	Title    string // original release title
	Artist   string // parsed artist (best-effort)
	Album    string // parsed album/title (best-effort)
	Year     int
	Format   string // normalized: FLAC, MP3, AAC, ALAC, OPUS, OGG, WAV, APE, DSD
	Bitrate  int    // kbps for lossy CBR; 0 when unknown/lossless
	VBR      bool   // variable bitrate (V0/V2/APS/APX)
	VBRTag   string // V0, V2, APS, APX when present
	Lossless bool
	Bits     int    // sample bit depth (e.g. 16, 24) when detected
	Media    string // CD, WEB, Vinyl, SACD, Cassette when detected
}

var (
	reYear      = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	reBitrate   = regexp.MustCompile(`(?i)\b(\d{2,4})\s*kbps\b|\b(96|112|128|160|192|224|256|320)\b`)
	reBitDepth  = regexp.MustCompile(`(?i)\b(16|24|32)[\s\-]?bit\b`)
	reMP3VBR    = regexp.MustCompile(`(?i)\b(V0|V2|APS|APX)\b`)
	reSplitDash = regexp.MustCompile(`\s+-\s+`)
	// reTagBlock matches trailing/inline bracket or brace tag groups.
	reTagBlock = regexp.MustCompile(`[\[\({][^\]\)}]*[\]\)}]`)
)

// formatKeywords maps lowercase tokens to a normalized format label and whether
// it is lossless.
var formatKeywords = []struct {
	token    string
	format   string
	lossless bool
}{
	{"flac", "FLAC", true},
	{"alac", "ALAC", true},
	{"ape", "APE", true},
	{"wavpack", "WavPack", true},
	{"wav", "WAV", true},
	{"aiff", "AIFF", true},
	{"dsd", "DSD", true},
	{"dsf", "DSD", true},
	{"sacd", "DSD", true},
	{"opus", "Opus", false},
	{"vorbis", "OGG", false},
	{"ogg", "OGG", false},
	{"aac", "AAC", false},
	{"m4a", "AAC", false},
	{"mp3", "MP3", false},
}

var mediaKeywords = []struct {
	token string
	media string
}{
	{"web-dl", "WEB"},
	{"webflac", "WEB"},
	{"web", "WEB"},
	{"cd", "CD"},
	{"vinyl", "Vinyl"},
	{"lp", "Vinyl"},
	{"sacd", "SACD"},
	{"cassette", "Cassette"},
	{"tape", "Cassette"},
}

// ParseMusic extracts structured quality/identity hints from a release title.
func ParseMusic(title string) *MusicRelease {
	r := &MusicRelease{Title: title}
	lower := strings.ToLower(title)

	// Format (first keyword wins; ordered so lossless/specific tokens win).
	for _, fk := range formatKeywords {
		if containsToken(lower, fk.token) {
			r.Format = fk.format
			r.Lossless = fk.lossless
			break
		}
	}

	// Bit depth (e.g. 24bit).
	if m := reBitDepth.FindStringSubmatch(lower); m != nil {
		r.Bits, _ = strconv.Atoi(m[1])
	}

	// MP3 VBR tags.
	if m := reMP3VBR.FindStringSubmatch(title); m != nil {
		r.VBR = true
		r.VBRTag = strings.ToUpper(m[1])
	}

	// Lossy bitrate (skip for lossless formats).
	if !r.Lossless {
		if m := reBitrate.FindStringSubmatch(lower); m != nil {
			val := m[1]
			if val == "" {
				val = m[2]
			}
			if n, err := strconv.Atoi(val); err == nil {
				r.Bitrate = n
			}
		}
	}

	// Media source.
	for _, mk := range mediaKeywords {
		if containsToken(lower, mk.token) {
			r.Media = mk.media
			break
		}
	}

	// Year.
	if m := reYear.FindStringSubmatch(title); m != nil {
		r.Year, _ = strconv.Atoi(m[1])
	}

	r.Artist, r.Album = parseArtistAlbum(title)
	return r
}

// parseArtistAlbum splits "Artist - Album ..." on the first " - " separator and
// strips trailing tag/quality/year noise from the album portion.
func parseArtistAlbum(title string) (artist, album string) {
	parts := reSplitDash.Split(title, 3)
	if len(parts) >= 2 {
		artist = strings.TrimSpace(parts[0])
		album = strings.TrimSpace(parts[1])
	} else {
		album = strings.TrimSpace(title)
	}
	album = cleanAlbum(album)
	return artist, album
}

// cleanAlbum removes bracketed tag groups, trailing year and quality keywords
// from a candidate album string.
func cleanAlbum(s string) string {
	s = reTagBlock.ReplaceAllString(s, " ")
	s = reYear.ReplaceAllString(s, " ")
	// Drop everything from the first standalone quality keyword onward.
	lower := strings.ToLower(s)
	cut := len(s)
	for _, kw := range []string{"flac", "mp3", "aac", "alac", "ape", "wav", "opus", "ogg", "dsd", "web", "vinyl", "cd ", "kbps"} {
		if idx := indexToken(lower, kw); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	s = s[:cut]
	s = strings.Trim(s, " -_.([{")
	return strings.Join(strings.Fields(s), " ")
}

// containsToken reports whether s contains token as a whole "word" (delimited by
// non-alphanumeric boundaries), so "cd" does not match "record".
func containsToken(s, token string) bool {
	return indexToken(s, token) >= 0
}

// indexToken returns the byte index of token in s when bounded by non-alphanumeric
// characters, or -1.
func indexToken(s, token string) int {
	from := 0
	for {
		idx := strings.Index(s[from:], token)
		if idx < 0 {
			return -1
		}
		abs := from + idx
		if isBounded(s, abs, len(token)) {
			return abs
		}
		from = abs + 1
		if from >= len(s) {
			return -1
		}
	}
}

func isBounded(s string, start, length int) bool {
	if start > 0 && isAlnum(s[start-1]) {
		return false
	}
	end := start + length
	if end < len(s) && isAlnum(s[end]) {
		return false
	}
	return true
}

func isAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
