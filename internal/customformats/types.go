// Package customformats implements a rule-based release matching engine
// inspired by Sonarr/Radarr custom formats. Users define named formats
// with scored specifications; the engine evaluates releases against all
// loaded formats and returns aggregate scores.
package customformats

// ReleaseInfo holds parsed metadata about a release for matching.
type ReleaseInfo struct {
	Title        string   `json:"title"`
	Quality      string   `json:"quality"`
	Size         int64    `json:"size"`
	Indexer      string   `json:"indexer"`
	Source       string   `json:"source"`
	Resolution   string   `json:"resolution"`
	Codec        string   `json:"codec"`
	Audio        string   `json:"audio"`
	Group        string   `json:"group"`
	Languages    []string `json:"languages"`
	IndexerFlags []string `json:"indexer_flags"`
}

// Specification is a single matching condition within a custom format.
type Specification struct {
	Name           string         `json:"name"`
	Implementation string         `json:"implementation"`
	Negate         bool           `json:"negate"`
	Required       bool           `json:"required"`
	Fields         map[string]any `json:"fields"`
}

// CustomFormat is a named collection of specifications.
type CustomFormat struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	IncludeWhenRenaming bool            `json:"include_when_renaming"`
	Specifications      []Specification `json:"specifications"`
	CreatedAt           string          `json:"created_at,omitempty"`
	UpdatedAt           string          `json:"updated_at,omitempty"`
}

// FormatMatch records a single custom format that matched a release.
type FormatMatch struct {
	CustomFormatID   string `json:"custom_format_id"`
	CustomFormatName string `json:"custom_format_name"`
	Score            int    `json:"score"`
}

// Implementation type constants.
const (
	ImplReleaseTitle = "ReleaseTitleSpec"
	ImplQuality      = "QualitySpec"
	ImplSize         = "SizeSpec"
	ImplIndexerFlag  = "IndexerFlagSpec"
	ImplSource       = "SourceSpec"
	ImplResolution   = "ResolutionSpec"
	ImplCodec        = "CodecSpec"
	ImplAudio        = "AudioSpec"
	ImplReleaseGroup = "ReleaseGroupSpec"
	ImplLanguage     = "LanguageSpec"
)
