package qualityprofiles

import (
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
)

// MatchDefinition is the single built-in quality detector shared by search and
// import. It maps a parsed release to a quality definition by source and
// resolution. assumed is true when the release name states no source and the
// definition was chosen by AssumedQualityName or a resolution-only fallback.
func MatchDefinition(rel *parser.Release, defs []*movies.QualityDefinition) (def *movies.QualityDefinition, assumed bool) {
	if rel == nil {
		return nil, false
	}

	lowerParsedRes := strings.ToLower(fmt.Sprintf("%dp", rel.Resolution))
	parsedSource := NormalizeSource(rel.Source)

	// Priority 1: exact slug match against definition Name.
	slug := parsedSource + "-" + lowerParsedRes
	if rel.IsRemux {
		slug += "-remux"
	}
	for _, d := range defs {
		if strings.ToLower(d.Name) == slug {
			return d, false
		}
	}

	// Priority 2: normalised source + resolution + modifier match.
	// Definitions use display names for Source (e.g. "Web", "TV").
	for _, d := range defs {
		if strings.ToLower(d.Resolution) == lowerParsedRes &&
			NormalizeDefSource(d.Source) == parsedSource &&
			strings.EqualFold(d.Modifier, "REMUX") == rel.IsRemux {
			return d, false
		}
	}

	// Only unknown sources and non-source tags may be assumed. Falling back
	// from a real source (for example CAM) could mislabel it as WEB or BluRay.
	switch parsedSource {
	case "", "unknown", "uhd", "remux":
	default:
		return nil, false
	}

	// Priority 3: the shared assumption for releases that state no source.
	if name := assumedName(rel); name != "" {
		for _, d := range defs {
			if strings.EqualFold(d.Name, name) {
				return d, true
			}
		}
	}

	// Priority 4: resolution + modifier only.
	for _, d := range defs {
		if strings.ToLower(d.Resolution) == lowerParsedRes &&
			strings.EqualFold(d.Modifier, "REMUX") == rel.IsRemux {
			return d, true
		}
	}

	// Priority 5: resolution only.
	for _, d := range defs {
		if strings.ToLower(d.Resolution) == lowerParsedRes && d.Modifier == "" {
			return d, true
		}
	}
	return nil, false
}

// assumedName applies AssumedQualityName to sources MatchDefinition treats
// as unknown, including bare UHD/Remux tags.
func assumedName(rel *parser.Release) string {
	copy := *rel
	switch NormalizeSource(copy.Source) {
	case "uhd", "remux":
		copy.Source = ""
	}
	return AssumedQualityName(&copy)
}

// DefinitionsFromProfile derives definitions from seeded-style item names
// (e.g. "webdl-1080p", "bluray-2160p-remux", "dvd"). It is used only when the
// real definition list is unavailable; item IDs are preserved.
func DefinitionsFromProfile(profile *QualityProfile) []*movies.QualityDefinition {
	if profile == nil {
		return nil
	}
	items, err := profile.ParseItems()
	if err != nil {
		return nil
	}
	defs := make([]*movies.QualityDefinition, 0, len(items))
	for _, item := range items {
		if item.Kind != "" {
			continue
		}
		name := strings.ToLower(item.Name)
		def := &movies.QualityDefinition{ID: item.ID, Name: item.Name, Title: item.Name}
		switch name {
		case "dvd":
			def.Source, def.Resolution = "DVD", "480p"
		case "sdtv":
			def.Source, def.Resolution = "TV", "480p"
		default:
			base := strings.TrimSuffix(name, "-remux")
			if base != name {
				def.Modifier = "REMUX"
			}
			source, res, ok := strings.Cut(base, "-")
			if !ok {
				continue
			}
			def.Source, def.Resolution = source, res
		}
		defs = append(defs, def)
	}
	return defs
}

// NormalizeSource maps parser source strings to quality definition source names.
func NormalizeSource(source string) string {
	switch strings.ToLower(source) {
	case "bluray", "blu-ray", "bdrip", "brrip":
		return "bluray"
	case "webdl", "web-dl", "web dl":
		return "webdl"
	case "webrip", "web-rip", "web rip":
		return "webrip"
	case "hdtv":
		return "hdtv"
	case "dvd", "dvdrip", "dvd-rip":
		return "dvd"
	case "remux":
		return "remux"
	case "cam", "ts", "telesync", "telecine", "tc":
		return "cam"
	case "screener", "dvdscr":
		return "screener"
	default:
		return strings.ToLower(source)
	}
}

// NormalizeDefSource maps quality definition Source display names to the
// same canonical form NormalizeSource produces from parser output.
func NormalizeDefSource(source string) string {
	switch strings.ToLower(source) {
	case "web":
		return "webdl"
	case "tv":
		return "hdtv"
	default:
		return NormalizeSource(source)
	}
}
