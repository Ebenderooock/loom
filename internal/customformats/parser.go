package customformats

import (
	"regexp"
	"strings"
)

var (
	reResolution = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p)\b`)
	reSource     = regexp.MustCompile(`(?i)\b(blu-?ray|bdremux|bdrip|web-?dl|webrip|web|hdtv|dvdrip|dvd|cam|ts|telesync|telecine)\b`)
	reCodec      = regexp.MustCompile(`(?i)\b(x264|h\.?264|x265|h\.?265|hevc|av1|vp9|xvid|divx|mpeg2)\b`)
	reAudio      = regexp.MustCompile(`(?i)\b(dts-hd[. ]?ma|dts-hd|truehd|atmos|flac|aac|dd5\.?1|ddp?5\.?1|ac3|dts|lpcm|opus|eac3)\b`)
	reGroup      = regexp.MustCompile(`-([A-Za-z0-9]+)(?:\.[a-z]{2,4})?$`)
	reLanguage   = regexp.MustCompile(`(?i)\b(multi|dual|french|german|spanish|italian|portuguese|russian|japanese|korean|chinese|hindi|arabic|dutch|swedish|norwegian|danish|finnish|polish|czech|turkish|thai|vietnamese|english)\b`)
)

// ParseReleaseName extracts structured metadata from a release title.
func ParseReleaseName(title string) ReleaseInfo {
	ri := ReleaseInfo{Title: title}

	if m := reResolution.FindString(title); m != "" {
		ri.Resolution = normaliseResolution(m)
	}

	if m := reSource.FindString(title); m != "" {
		ri.Source = normaliseSource(m)
	}

	if m := reCodec.FindString(title); m != "" {
		ri.Codec = normaliseCodec(m)
	}

	if m := reAudio.FindString(title); m != "" {
		ri.Audio = normaliseAudio(m)
	}

	if m := reGroup.FindStringSubmatch(title); len(m) > 1 {
		ri.Group = m[1]
	}

	langs := reLanguage.FindAllString(title, -1)
	seen := make(map[string]bool, len(langs))
	for _, l := range langs {
		key := strings.ToUpper(l)
		if !seen[key] {
			seen[key] = true
			ri.Languages = append(ri.Languages, normaliseLang(l))
		}
	}

	return ri
}

func normaliseResolution(s string) string {
	return strings.ToLower(s)
}

func normaliseSource(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "blu") || lower == "bdremux" || lower == "bdrip":
		return "BluRay"
	case lower == "web-dl" || lower == "webdl":
		return "WEB-DL"
	case lower == "webrip":
		return "WEBRip"
	case lower == "web":
		return "WEB-DL"
	case lower == "hdtv":
		return "HDTV"
	case lower == "dvdrip" || lower == "dvd":
		return "DVDRip"
	case lower == "cam":
		return "CAM"
	case lower == "ts" || lower == "telesync":
		return "TS"
	case lower == "telecine":
		return "TC"
	default:
		return s
	}
}

func normaliseCodec(s string) string {
	lower := strings.ToLower(strings.ReplaceAll(s, ".", ""))
	switch {
	case lower == "x265" || lower == "h265" || lower == "hevc":
		return "x265"
	case lower == "x264" || lower == "h264":
		return "x264"
	case lower == "av1":
		return "AV1"
	case lower == "vp9":
		return "VP9"
	case lower == "xvid":
		return "XviD"
	case lower == "divx":
		return "DivX"
	default:
		return s
	}
}

func normaliseAudio(s string) string {
	lower := strings.ToLower(strings.ReplaceAll(s, " ", ""))
	lower = strings.ReplaceAll(lower, ".", "")
	switch {
	case strings.HasPrefix(lower, "dts-hdma") || strings.HasPrefix(lower, "dtshdma"):
		return "DTS-HD MA"
	case strings.HasPrefix(lower, "dts-hd") || strings.HasPrefix(lower, "dtshd"):
		return "DTS-HD"
	case lower == "truehd":
		return "TrueHD"
	case lower == "atmos":
		return "Atmos"
	case lower == "flac":
		return "FLAC"
	case lower == "aac":
		return "AAC"
	case strings.Contains(lower, "dd51") || strings.Contains(lower, "ddp51") || lower == "ac3":
		return "DD5.1"
	case lower == "dts":
		return "DTS"
	case lower == "eac3":
		return "EAC3"
	case lower == "opus":
		return "Opus"
	default:
		return s
	}
}

func normaliseLang(s string) string {
	switch strings.ToUpper(s) {
	case "MULTI":
		return "MULTi"
	case "DUAL":
		return "DUAL"
	default:
		// Capitalise first letter.
		if len(s) == 0 {
			return s
		}
		return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
	}
}
