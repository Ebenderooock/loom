package customformats

import (
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// ParseReleaseName extracts structured metadata from a release title.
func ParseReleaseName(title string) ReleaseInfo {
	return FromParsedRelease(parser.Parse(title))
}

// FromParsedRelease shares the acquisition/import parser's evidence with the
// custom-format tester, retaining the field names used by saved format rules.
func FromParsedRelease(release *parser.Release) ReleaseInfo {
	ri := ReleaseInfo{
		Title:     release.Name,
		Quality:   qualityprofiles.QualityNameFromRelease(release),
		Source:    normaliseSource(release.Source),
		Codec:     normaliseCodec(release.Codec),
		Audio:     normaliseAudio(release.Audio),
		Group:     release.Group,
		Languages: release.Languages,
	}
	if release.Resolution > 0 {
		ri.Resolution = fmt.Sprintf("%dp", release.Resolution)
	}
	return ri
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
	switch lower {
	case "x265", "h265", "hevc":
		return "x265"
	case "x264", "h264":
		return "x264"
	case "av1":
		return "AV1"
	case "vp9":
		return "VP9"
	case "xvid":
		return "XviD"
	case "divx":
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
