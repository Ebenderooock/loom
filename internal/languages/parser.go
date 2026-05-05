package languages

import (
	"regexp"
	"strings"
)

// tagMapping maps release-name tokens to ISO 639-1 codes.
var tagMapping = map[string]string{
	"english":    "en",
	"eng":        "en",
	"french":     "fr",
	"truefrench": "fr",
	"vff":        "fr",
	"vfq":        "fr",
	"german":     "de",
	"ger":        "de",
	"spanish":    "es",
	"spa":        "es",
	"italian":    "it",
	"ita":        "it",
	"portuguese": "pt",
	"por":        "pt",
	"japanese":   "ja",
	"jpn":        "ja",
	"korean":     "ko",
	"kor":        "ko",
	"chinese":    "zh",
	"chi":        "zh",
	"russian":    "ru",
	"rus":        "ru",
	"arabic":     "ar",
	"ara":        "ar",
	"hindi":      "hi",
	"hin":        "hi",
	"dutch":      "nl",
	"nld":        "nl",
	"swedish":    "sv",
	"swe":        "sv",
	"norwegian":  "no",
	"nor":        "no",
	"danish":     "da",
	"dan":        "da",
	"finnish":    "fi",
	"fin":        "fi",
	"polish":     "pl",
	"pol":        "pl",
	"czech":      "cs",
	"cze":        "cs",
	"turkish":    "tr",
	"tur":        "tr",
	"thai":       "th",
	"tha":        "th",
	"vietnamese": "vi",
	"vie":        "vi",
	"indonesian": "id",
	"ind":        "id",
	"malay":      "ms",
	"msa":        "ms",
	"greek":      "el",
	"gre":        "el",
	"hebrew":     "he",
	"heb":        "he",
	"romanian":   "ro",
	"ron":        "ro",
	"hungarian":  "hu",
	"hun":        "hu",
	"ukrainian":  "uk",
	"ukr":        "uk",
	"bulgarian":  "bg",
	"bul":        "bg",
}

// Special token patterns
var (
	// tokenRe splits on dots, dashes, underscores, and spaces.
	tokenRe = regexp.MustCompile(`[.\-_ ]+`)

	// vostfrRe detects VOSTFR (French-subbed, original-audio releases).
	vostfrRe = regexp.MustCompile(`(?i)\bVOSTFR\b`)
)

// ParseResult holds everything detected from a release title.
type ParseResult struct {
	Languages        []Language `json:"languages"`
	IsMulti          bool       `json:"is_multi"`
	IsDualAudio      bool       `json:"is_dual_audio"`
	SubtitleLanguages []Language `json:"subtitle_languages,omitempty"`
}

// ParseTitle extracts language information from a release title.
func ParseTitle(title string) ParseResult {
	res := ParseResult{}
	seen := map[string]bool{}

	upper := strings.ToUpper(title)
	tokens := tokenRe.Split(upper, -1)

	for _, tok := range tokens {
		lower := strings.ToLower(tok)

		switch lower {
		case "multi":
			res.IsMulti = true
			continue
		case "dual", "dualaudio":
			res.IsDualAudio = true
			continue
		}

		// Check for subtitle indicators.
		if strings.HasPrefix(lower, "sub") {
			// e.g. SUBFRENCH → fr subtitle
			rest := strings.TrimPrefix(lower, "sub")
			if code, ok := tagMapping[rest]; ok {
				if lang, ok2 := LookupLanguage(code); ok2 {
					res.SubtitleLanguages = append(res.SubtitleLanguages, lang)
				}
			}
			if lower == "subs" || lower == "sub" {
				// generic subs marker, no specific language
			}
			continue
		}

		if code, ok := tagMapping[lower]; ok {
			if !seen[code] {
				seen[code] = true
				if lang, ok2 := LookupLanguage(code); ok2 {
					res.Languages = append(res.Languages, lang)
				}
			}
		}
	}

	// Also check combined tokens that tokenRe doesn't split
	if strings.Contains(upper, "DUAL") && strings.Contains(upper, "AUDIO") {
		res.IsDualAudio = true
	}

	// VOSTFR = original audio + French subtitles
	if vostfrRe.MatchString(title) {
		if !hasSub(res.SubtitleLanguages, "fr") {
			if lang, ok := LookupLanguage("fr"); ok {
				res.SubtitleLanguages = append(res.SubtitleLanguages, lang)
			}
		}
	}

	// If no explicit language was found, default to English.
	if len(res.Languages) == 0 && !res.IsMulti && !res.IsDualAudio {
		if lang, ok := LookupLanguage("en"); ok {
			res.Languages = append(res.Languages, lang)
		}
	}

	return res
}

func hasSub(subs []Language, code string) bool {
	for _, s := range subs {
		if s.Code == code {
			return true
		}
	}
	return false
}
