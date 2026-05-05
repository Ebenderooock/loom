package mediainfo

import (
	"regexp"
	"strings"
)

// Audio codec patterns ordered from most to least specific.
var audioCodecPatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\bTrueHD[\.\s]*Atmos\b`), "TrueHD Atmos"},
	{regexp.MustCompile(`(?i)\bDTS[\-\.\s]?HD[\.\s]?MA\b`), "DTS-HD MA"},
	{regexp.MustCompile(`(?i)\bDTS[\-\.\s]?X\b`), "DTS-X"},
	{regexp.MustCompile(`(?i)\bDTS[\-\.\s]?HD\b`), "DTS-HD"},
	{regexp.MustCompile(`(?i)\bAtmos\b`), "Atmos"},
	{regexp.MustCompile(`(?i)\bTrueHD\b`), "TrueHD"},
	{regexp.MustCompile(`(?i)\bFLAC\b`), "FLAC"},
	{regexp.MustCompile(`(?i)\bEAC[\-\.]?3\b|(?i)\bDDP?\b`), "EAC3"},
	{regexp.MustCompile(`(?i)\bAC[\-\.]?3\b`), "AC3"},
	{regexp.MustCompile(`(?i)\bDTS\b`), "DTS"},
	{regexp.MustCompile(`(?i)\bAAC\b`), "AAC"},
	{regexp.MustCompile(`(?i)\bOPUS\b`), "OPUS"},
	{regexp.MustCompile(`(?i)\bMP3\b`), "MP3"},
}

var videoCodecPatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\bAV1\b`), "AV1"},
	{regexp.MustCompile(`(?i)\bVP9\b`), "VP9"},
	{regexp.MustCompile(`(?i)\b[xh][\.\-]?265\b|(?i)\bHEVC\b`), "x265"},
	{regexp.MustCompile(`(?i)\b[xh][\.\-]?264\b|(?i)\bAVC\b`), "x264"},
	{regexp.MustCompile(`(?i)\bXviD\b`), "XviD"},
	{regexp.MustCompile(`(?i)\bDivX\b`), "DivX"},
	{regexp.MustCompile(`(?i)\bMPEG[\-]?2\b`), "MPEG-2"},
	{regexp.MustCompile(`(?i)\bVC[\-]?1\b`), "VC-1"},
}

var hdrPatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\bHDR10[\+\s]?Plus\b|(?i)\bHDR10\+`), "HDR10+"},
	{regexp.MustCompile(`(?i)\bDolby[\.\s]?Vision\b|(?i)\bDo?Vi\b|(?i)\bDV\b`), "Dolby Vision"},
	{regexp.MustCompile(`(?i)\bHDR10\b`), "HDR10"},
	{regexp.MustCompile(`(?i)\bHDR\b`), "HDR"},
	{regexp.MustCompile(`(?i)\bHLG\b`), "HLG"},
}

var resolutionPatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\b2160p\b|(?i)\b4K\b|(?i)\bUHD\b`), "2160p"},
	{regexp.MustCompile(`(?i)\b1080p\b`), "1080p"},
	{regexp.MustCompile(`(?i)\b1080i\b`), "1080i"},
	{regexp.MustCompile(`(?i)\b720p\b`), "720p"},
	{regexp.MustCompile(`(?i)\b576p\b`), "576p"},
	{regexp.MustCompile(`(?i)\b480p\b`), "480p"},
}

var sourcePatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\bBlu[\-\.]?Ray\b|(?i)\bBDRip\b|(?i)\bBRRip\b`), "BluRay"},
	{regexp.MustCompile(`(?i)\bWEB[\-\.]?DL\b`), "WEB-DL"},
	{regexp.MustCompile(`(?i)\bWEBRip\b|(?i)\bWEB[\-\.]?Rip\b`), "WEBRip"},
	{regexp.MustCompile(`(?i)\bWEB\b`), "WEB"},
	{regexp.MustCompile(`(?i)\bHDTV\b`), "HDTV"},
	{regexp.MustCompile(`(?i)\bDVDRip\b|(?i)\bDVD\b`), "DVD"},
	{regexp.MustCompile(`(?i)\bHDRip\b`), "HDRip"},
	{regexp.MustCompile(`(?i)\bRemux\b`), "Remux"},
	{regexp.MustCompile(`(?i)\bCAM\b|(?i)\bTS\b|(?i)\bTelesync\b`), "CAM"},
}

var channelsRe = regexp.MustCompile(`\b(7\.1|5\.1|2\.0|2\.1|1\.0)\b`)

var subPatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\bHARDSUB\b|(?i)\bHC\b`), "hardcoded"},
	{regexp.MustCompile(`(?i)\bSUBBED\b|(?i)\bSUB\b`), "softcoded"},
	{regexp.MustCompile(`(?i)\.srt\b`), "softcoded"},
	{regexp.MustCompile(`(?i)\bFORCED\b`), "forced"},
}

// Language hints that may indicate audio languages in release names.
var audioLangPatterns = []struct {
	re   *regexp.Regexp
	lang string
}{
	{regexp.MustCompile(`(?i)\bMULTi\b`), "multi"},
	{regexp.MustCompile(`(?i)\bDUAL[\.\s\-]?AUDIO\b|(?i)\bDUAL\b`), "dual"},
	{regexp.MustCompile(`(?i)\bFRENCH\b|(?i)\bVFF\b|(?i)\bVFQ\b|(?i)\bTRUEFRENCH\b`), "fr"},
	{regexp.MustCompile(`(?i)\bGERMAN\b`), "de"},
	{regexp.MustCompile(`(?i)\bSPANISH\b|(?i)\bLATINO\b|(?i)\bCastellano\b`), "es"},
	{regexp.MustCompile(`(?i)\bITALIAN\b`), "it"},
	{regexp.MustCompile(`(?i)\bJAPANESE\b`), "ja"},
	{regexp.MustCompile(`(?i)\bKOREAN\b`), "ko"},
	{regexp.MustCompile(`(?i)\bCHINESE\b|(?i)\bMANDARIN\b`), "zh"},
	{regexp.MustCompile(`(?i)\bRUSSIAN\b`), "ru"},
	{regexp.MustCompile(`(?i)\bPORTUGUESE\b`), "pt"},
	{regexp.MustCompile(`(?i)\bHINDI\b`), "hi"},
	{regexp.MustCompile(`(?i)\bARABIC\b`), "ar"},
}

// Subtitle language hints.
var subLangPatterns = []struct {
	re   *regexp.Regexp
	lang string
}{
	{regexp.MustCompile(`(?i)\bVOSTFR\b|(?i)\bSUB[\.\s\-]?FR\b`), "fr"},
	{regexp.MustCompile(`(?i)\bSUB[\.\s\-]?SPA\b|(?i)\bSUB[\.\s\-]?ESP\b`), "es"},
	{regexp.MustCompile(`(?i)\bSUB[\.\s\-]?ITA\b`), "it"},
	{regexp.MustCompile(`(?i)\bSUB[\.\s\-]?GER\b`), "de"},
	{regexp.MustCompile(`(?i)\bENGSUB\b|(?i)\bSUB[\.\s\-]?ENG\b`), "en"},
}

// Parse extracts a MediaInfo from a release name string.
func Parse(name string) MediaInfo {
	info := MediaInfo{}

	// Audio codec
	for _, p := range audioCodecPatterns {
		if p.re.MatchString(name) {
			info.AudioCodec = p.label
			break
		}
	}

	// Video codec
	for _, p := range videoCodecPatterns {
		if p.re.MatchString(name) {
			info.VideoCodec = p.label
			break
		}
	}

	// HDR
	for _, p := range hdrPatterns {
		if p.re.MatchString(name) {
			info.HDR = p.label
			break
		}
	}

	// Resolution
	for _, p := range resolutionPatterns {
		if p.re.MatchString(name) {
			info.Resolution = p.label
			break
		}
	}

	// Source
	for _, p := range sourcePatterns {
		if p.re.MatchString(name) {
			info.Source = p.label
			break
		}
	}

	// Audio channels
	if m := channelsRe.FindString(name); m != "" {
		info.AudioChannels = m
	}

	// Subtitle type
	for _, p := range subPatterns {
		if p.re.MatchString(name) {
			info.SubType = p.label
			break
		}
	}

	// Audio languages
	seen := map[string]bool{}
	for _, p := range audioLangPatterns {
		if p.re.MatchString(name) && !seen[p.lang] {
			seen[p.lang] = true
			info.AudioLanguages = append(info.AudioLanguages, p.lang)
		}
	}

	// Subtitle languages
	subSeen := map[string]bool{}
	for _, p := range subLangPatterns {
		if p.re.MatchString(name) && !subSeen[p.lang] {
			subSeen[p.lang] = true
			info.SubLanguages = append(info.SubLanguages, p.lang)
		}
	}

	// Ensure non-nil slices for JSON
	if info.AudioLanguages == nil {
		info.AudioLanguages = []string{}
	}
	if info.SubLanguages == nil {
		info.SubLanguages = []string{}
	}

	// Default HDR to SDR if nothing detected and we have resolution
	if info.HDR == "" && info.Resolution != "" {
		info.HDR = "SDR"
	}
	if info.SubType == "" {
		info.SubType = "none"
	}

	return info
}

// containsCI checks if slice contains s (case-insensitive).
func containsCI(slice []string, s string) bool {
	low := strings.ToLower(s)
	for _, v := range slice {
		if strings.ToLower(v) == low {
			return true
		}
	}
	return false
}
