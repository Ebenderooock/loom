// Package languages provides language profiles for media automation —
// detection, scoring, and priority-based selection similar to Radarr's
// language handling but designed for TV-series workflows (MULTi, dual
// audio, VOSTFR, etc.).
package languages

import "time"

// Language represents a single spoken / written language.
type Language struct {
	Code       string `json:"code"`        // ISO 639-1
	Name       string `json:"name"`        // English name
	NativeName string `json:"native_name"` // Autonym
}

// LanguagePriority ties a Language to a profile with ordering.
type LanguagePriority struct {
	Language Language `json:"language"`
	Allowed  bool     `json:"allowed"`
	Priority int      `json:"priority"` // lower = higher priority
}

// LanguageProfile is a named, ordered set of language preferences.
type LanguageProfile struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Languages       []LanguagePriority `json:"languages"`
	CutoffLanguage  string             `json:"cutoff_language"`  // ISO 639-1 code
	UpgradeAllowed  bool               `json:"upgrade_allowed"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// LanguageScore is the result of scoring a release against a profile.
type LanguageScore struct {
	Score            int        `json:"score"`
	MatchedLanguages []Language `json:"matched_languages"`
	MeetsMinimum     bool       `json:"meets_minimum"`
}

// ---------- pre-defined language catalogue ----------

// AllLanguages is the canonical list of languages Loom knows about.
var AllLanguages = []Language{
	{Code: "en", Name: "English", NativeName: "English"},
	{Code: "fr", Name: "French", NativeName: "Français"},
	{Code: "de", Name: "German", NativeName: "Deutsch"},
	{Code: "es", Name: "Spanish", NativeName: "Español"},
	{Code: "it", Name: "Italian", NativeName: "Italiano"},
	{Code: "pt", Name: "Portuguese", NativeName: "Português"},
	{Code: "ja", Name: "Japanese", NativeName: "日本語"},
	{Code: "ko", Name: "Korean", NativeName: "한국어"},
	{Code: "zh", Name: "Chinese", NativeName: "中文"},
	{Code: "ru", Name: "Russian", NativeName: "Русский"},
	{Code: "ar", Name: "Arabic", NativeName: "العربية"},
	{Code: "hi", Name: "Hindi", NativeName: "हिन्दी"},
	{Code: "nl", Name: "Dutch", NativeName: "Nederlands"},
	{Code: "sv", Name: "Swedish", NativeName: "Svenska"},
	{Code: "no", Name: "Norwegian", NativeName: "Norsk"},
	{Code: "da", Name: "Danish", NativeName: "Dansk"},
	{Code: "fi", Name: "Finnish", NativeName: "Suomi"},
	{Code: "pl", Name: "Polish", NativeName: "Polski"},
	{Code: "cs", Name: "Czech", NativeName: "Čeština"},
	{Code: "tr", Name: "Turkish", NativeName: "Türkçe"},
	{Code: "th", Name: "Thai", NativeName: "ไทย"},
	{Code: "vi", Name: "Vietnamese", NativeName: "Tiếng Việt"},
	{Code: "id", Name: "Indonesian", NativeName: "Bahasa Indonesia"},
	{Code: "ms", Name: "Malay", NativeName: "Bahasa Melayu"},
	{Code: "el", Name: "Greek", NativeName: "Ελληνικά"},
	{Code: "he", Name: "Hebrew", NativeName: "עברית"},
	{Code: "ro", Name: "Romanian", NativeName: "Română"},
	{Code: "hu", Name: "Hungarian", NativeName: "Magyar"},
	{Code: "uk", Name: "Ukrainian", NativeName: "Українська"},
	{Code: "bg", Name: "Bulgarian", NativeName: "Български"},
}

// languageByCode is a lookup cache built at init time.
var languageByCode map[string]Language

func init() {
	languageByCode = make(map[string]Language, len(AllLanguages))
	for _, l := range AllLanguages {
		languageByCode[l.Code] = l
	}
}

// LookupLanguage returns the Language for the given ISO 639-1 code.
func LookupLanguage(code string) (Language, bool) {
	l, ok := languageByCode[code]
	return l, ok
}
