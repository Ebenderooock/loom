package safety

import (
	"path/filepath"
	"strings"
)

// Severity indicates how serious a validation finding is.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityBlock   Severity = "block"
)

// ValidationResult captures the outcome of a release validation check.
type ValidationResult struct {
	Safe     bool     `json:"safe"`
	Reasons  []string `json:"reasons"`
	Severity Severity `json:"severity"`
}

// ReleaseValidatorConfig holds tunables for pre-download validation.
type ReleaseValidatorConfig struct {
	BlockDangerousExtensions bool     `json:"block_dangerous_extensions"`
	SuspiciousPatterns       []string `json:"suspicious_patterns"`
	MinMovieSizeMB           int64    `json:"min_movie_size_mb"`
	MaxMovieSizeMB           int64    `json:"max_movie_size_mb"`
}

// DefaultConfig returns a ReleaseValidatorConfig with sensible defaults.
func DefaultConfig() ReleaseValidatorConfig {
	return ReleaseValidatorConfig{
		BlockDangerousExtensions: true,
		SuspiciousPatterns: []string{
			"password", "passworded", "virus", "crack", "keygen", "patch",
		},
		MinMovieSizeMB: 50,
		MaxMovieSizeMB: 100_000,
	}
}

// dangerousExtensions is the set of extensions that should be blocked.
var dangerousExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".msi": true,
	".scr": true, ".pif": true, ".com": true, ".vbs": true,
	".js": true, ".wsh": true, ".wsf": true, ".ps1": true,
}

// ReleaseValidator performs pre-download safety checks on release metadata.
type ReleaseValidator struct {
	cfg ReleaseValidatorConfig
}

// NewReleaseValidator creates a validator with the given config.
func NewReleaseValidator(cfg ReleaseValidatorConfig) *ReleaseValidator {
	return &ReleaseValidator{cfg: cfg}
}

// ValidateRelease checks a release name and its listed file names for
// dangerous content. sizeMB is the total release size in megabytes;
// pass 0 to skip size checks. files is the list of filenames included
// in the release (may be nil when unavailable).
func (v *ReleaseValidator) ValidateRelease(releaseName string, sizeMB int64, files []string) ValidationResult {
	var reasons []string
	severity := SeverityWarning

	// Check suspicious patterns in release name.
	lower := strings.ToLower(releaseName)
	for _, pat := range v.cfg.SuspiciousPatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			reasons = append(reasons, "release name contains suspicious pattern: "+pat)
		}
	}

	// Check dangerous extensions in listed files.
	if v.cfg.BlockDangerousExtensions {
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f))
			if dangerousExtensions[ext] {
				reasons = append(reasons, "contains dangerous file type: "+ext)
				severity = SeverityBlock
			}
		}
	}

	// Size anomaly checks.
	if sizeMB > 0 {
		if v.cfg.MinMovieSizeMB > 0 && sizeMB < v.cfg.MinMovieSizeMB {
			reasons = append(reasons, "release is suspiciously small for a movie")
		}
		if v.cfg.MaxMovieSizeMB > 0 && sizeMB > v.cfg.MaxMovieSizeMB {
			reasons = append(reasons, "release is suspiciously large")
		}
	}

	if len(reasons) == 0 {
		return ValidationResult{Safe: true, Severity: SeverityWarning}
	}
	return ValidationResult{Safe: false, Reasons: reasons, Severity: severity}
}
