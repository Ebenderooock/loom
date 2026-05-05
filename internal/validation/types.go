package validation

// ValidationResult holds the outcome of validating a downloaded file.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Path   string            `json:"path"`
	Checks []ValidationCheck `json:"checks"`
}

// ValidationCheck is a single check outcome.
type ValidationCheck struct {
	Rule    string `json:"rule"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// ValidationRule is a configurable validation rule.
type ValidationRule struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Enabled     bool           `json:"enabled"`
	Description string         `json:"description"`
	Config      map[string]any `json:"config,omitempty"`
}

// DefaultRules returns the built-in validation rules.
func DefaultRules() []ValidationRule {
	return []ValidationRule{
		{
			ID:          "file_size",
			Name:        "File Size Check",
			Enabled:     true,
			Description: "Reject files that are 0 bytes or suspiciously small for their quality",
			Config: map[string]any{
				"min_bytes_480p":  104857600,
				"min_bytes_720p":  209715200,
				"min_bytes_1080p": 419430400,
				"min_bytes_2160p": 1073741824,
			},
		},
		{
			ID:          "extension",
			Name:        "Extension Validation",
			Enabled:     true,
			Description: "Only allow known media extensions (.mkv, .mp4, .avi, etc.)",
		},
		{
			ID:          "min_duration",
			Name:        "Minimum Duration",
			Enabled:     false,
			Description: "Reject files shorter than a minimum duration (requires mediainfo)",
			Config: map[string]any{
				"min_minutes": 20,
			},
		},
		{
			ID:          "archive_detection",
			Name:        "Archive Detection",
			Enabled:     true,
			Description: "Detect password-protected or suspicious archive files",
		},
	}
}
