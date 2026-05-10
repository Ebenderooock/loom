package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ebenderooock/loom/internal/mediafiles"
)

// Validator performs post-download file validation.
type Validator struct {
	rules []ValidationRule
}

// NewValidator creates a Validator with the given rules.
func NewValidator(rules []ValidationRule) *Validator {
	if len(rules) == 0 {
		rules = DefaultRules()
	}
	return &Validator{rules: rules}
}

// Rules returns the current validation rules.
func (v *Validator) Rules() []ValidationRule {
	return v.rules
}

// SetRules replaces all validation rules.
func (v *Validator) SetRules(rules []ValidationRule) {
	v.rules = rules
}

// Validate runs all enabled checks against a file path.
func (v *Validator) Validate(path string) ValidationResult {
	result := ValidationResult{
		Valid: true,
		Path:  path,
	}

	for _, rule := range v.rules {
		if !rule.Enabled {
			continue
		}
		check := v.runCheck(rule, path)
		result.Checks = append(result.Checks, check)
		if !check.Passed {
			result.Valid = false
		}
	}

	return result
}

func (v *Validator) runCheck(rule ValidationRule, path string) ValidationCheck {
	switch rule.ID {
	case "file_size":
		return v.checkFileSize(rule, path)
	case "extension":
		return v.checkExtension(path)
	case "archive_detection":
		return v.checkArchive(path)
	case "min_duration":
		return ValidationCheck{Rule: rule.ID, Passed: true, Message: "duration check skipped (mediainfo not available)"}
	default:
		return ValidationCheck{Rule: rule.ID, Passed: true, Message: "unknown rule, skipped"}
	}
}

func (v *Validator) checkFileSize(rule ValidationRule, path string) ValidationCheck {
	info, err := os.Stat(path)
	if err != nil {
		return ValidationCheck{Rule: "file_size", Passed: false, Message: fmt.Sprintf("cannot stat file: %s", err)}
	}

	if info.Size() == 0 {
		return ValidationCheck{Rule: "file_size", Passed: false, Message: "file is 0 bytes"}
	}

	// Check quality-based minimums
	name := strings.ToLower(filepath.Base(path))
	cfg := rule.Config

	type qCheck struct {
		pattern string
		key     string
	}
	checks := []qCheck{
		{"2160p", "min_bytes_2160p"},
		{"1080p", "min_bytes_1080p"},
		{"720p", "min_bytes_720p"},
		{"480p", "min_bytes_480p"},
	}

	for _, qc := range checks {
		if strings.Contains(name, qc.pattern) {
			if minVal, ok := cfg[qc.key]; ok {
				var minBytes int64
				switch mv := minVal.(type) {
				case float64:
					minBytes = int64(mv)
				case int:
					minBytes = int64(mv)
				case int64:
					minBytes = mv
				}
				if minBytes > 0 && info.Size() < minBytes {
					return ValidationCheck{
						Rule:    "file_size",
						Passed:  false,
						Message: fmt.Sprintf("file size %d bytes is below minimum %d for %s content", info.Size(), minBytes, qc.pattern),
					}
				}
			}
			break
		}
	}

	return ValidationCheck{Rule: "file_size", Passed: true, Message: fmt.Sprintf("file size %d bytes OK", info.Size())}
}

func (v *Validator) checkExtension(path string) ValidationCheck {
	ext := strings.ToLower(filepath.Ext(path))

	if mediafiles.IsDangerous(ext) {
		return ValidationCheck{
			Rule:    "extension",
			Passed:  false,
			Message: fmt.Sprintf("dangerous file extension: %s", ext),
		}
	}

	if !mediafiles.IsVideo(ext) {
		// Not in allowed list but also not dangerous — warn
		if ext == ".nfo" || ext == ".txt" || ext == ".srt" || ext == ".sub" || ext == ".idx" {
			return ValidationCheck{
				Rule:    "extension",
				Passed:  true,
				Message: fmt.Sprintf("non-media file %s (subtitle/info), skipped", ext),
			}
		}
		return ValidationCheck{
			Rule:    "extension",
			Passed:  false,
			Message: fmt.Sprintf("unrecognized media extension: %s", ext),
		}
	}

	return ValidationCheck{Rule: "extension", Passed: true, Message: fmt.Sprintf("extension %s is valid", ext)}
}

func (v *Validator) checkArchive(path string) ValidationCheck {
	ext := strings.ToLower(filepath.Ext(path))

	if mediafiles.IsArchive(ext) {
		return ValidationCheck{
			Rule:    "archive_detection",
			Passed:  false,
			Message: fmt.Sprintf("archive file detected: %s (may be password-protected)", ext),
		}
	}

	return ValidationCheck{Rule: "archive_detection", Passed: true, Message: "not an archive"}
}
