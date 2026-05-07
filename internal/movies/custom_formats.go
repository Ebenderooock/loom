package movies

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/parser"
)

// CustomFormatRepository defines database operations for custom formats.
type CustomFormatRepository interface {
	// Custom formats
	AddCustomFormat(ctx context.Context, cf *CustomFormat) error
	GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error)
	UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error
	DeleteCustomFormat(ctx context.Context, id string) error
	ListCustomFormats(ctx context.Context) ([]*CustomFormat, error)
	GetCustomFormatByName(ctx context.Context, name string) (*CustomFormat, error)
}

// CustomFormatService defines business logic for custom format management.
type CustomFormatService interface {
	// Custom formats
	AddCustomFormat(ctx context.Context, cf *CustomFormat) error
	GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error)
	UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error
	DeleteCustomFormat(ctx context.Context, id string) error
	ListCustomFormats(ctx context.Context) ([]*CustomFormat, error)

	// Evaluation
	EvaluateCustomFormats(ctx context.Context, releaseName string) ([]CustomFormatScore, error)

	// Validation
	ValidateCustomFormat(cf *CustomFormat) error
}

// customFormatService implements CustomFormatService.
type customFormatService struct {
	repo CustomFormatRepository
}

// NewCustomFormatService creates a new CustomFormatService instance.
func NewCustomFormatService(repo CustomFormatRepository) CustomFormatService {
	return &customFormatService{repo: repo}
}

// AddCustomFormat adds a new custom format.
func (cfs *customFormatService) AddCustomFormat(ctx context.Context, cf *CustomFormat) error {
	if cf == nil {
		return fmt.Errorf("movies: custom format required")
	}
	if cf.Name == "" {
		return fmt.Errorf("movies: custom format name required")
	}
	if len(cf.Filters) == 0 {
		return fmt.Errorf("movies: custom format requires at least one filter")
	}

	// Check for duplicate name
	existing, err := cfs.repo.GetCustomFormatByName(ctx, cf.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("movies: custom format %q already exists", cf.Name)
	}

	// Validate all filters
	if err := cfs.ValidateCustomFormat(cf); err != nil {
		return err
	}

	cf.CreatedAt = time.Now()
	cf.UpdatedAt = time.Now()
	cf.DeletedAt = nil

	return cfs.repo.AddCustomFormat(ctx, cf)
}

// GetCustomFormat retrieves a custom format by ID.
func (cfs *customFormatService) GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: custom format ID required")
	}
	return cfs.repo.GetCustomFormat(ctx, id)
}

// UpdateCustomFormat updates an existing custom format.
func (cfs *customFormatService) UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error {
	if cf == nil {
		return fmt.Errorf("movies: custom format required")
	}
	if cf.ID == "" {
		return fmt.Errorf("movies: custom format ID required")
	}
	if cf.Name == "" {
		return fmt.Errorf("movies: custom format name required")
	}
	if len(cf.Filters) == 0 {
		return fmt.Errorf("movies: custom format requires at least one filter")
	}

	// Check for duplicate name (excluding self)
	existing, err := cfs.repo.GetCustomFormatByName(ctx, cf.Name)
	if err == nil && existing != nil && existing.ID != cf.ID {
		return fmt.Errorf("movies: custom format %q already exists", cf.Name)
	}

	// Validate all filters
	if err := cfs.ValidateCustomFormat(cf); err != nil {
		return err
	}

	cf.UpdatedAt = time.Now()
	return cfs.repo.UpdateCustomFormat(ctx, cf)
}

// DeleteCustomFormat marks a custom format as deleted.
func (cfs *customFormatService) DeleteCustomFormat(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: custom format ID required")
	}
	return cfs.repo.DeleteCustomFormat(ctx, id)
}

// ListCustomFormats retrieves all non-deleted custom formats.
func (cfs *customFormatService) ListCustomFormats(ctx context.Context) ([]*CustomFormat, error) {
	return cfs.repo.ListCustomFormats(ctx)
}

// EvaluateCustomFormats evaluates all custom formats against a release name.
// Returns the IDs and scores of custom formats that matched.
func (cfs *customFormatService) EvaluateCustomFormats(ctx context.Context, releaseName string) ([]CustomFormatScore, error) {
	if releaseName == "" {
		return []CustomFormatScore{}, nil
	}

	formats, err := cfs.ListCustomFormats(ctx)
	if err != nil {
		return nil, err
	}

	var scores []CustomFormatScore
	for _, format := range formats {
		if cfs.matchesAllFilters(format, releaseName) {
			// Score will be assigned via quality profile FormatItems
			scores = append(scores, CustomFormatScore{
				CustomFormatID: format.ID,
				Score:          0, // Placeholder; actual score comes from profile
			})
		}
	}

	return scores, nil
}

// matchesAllFilters checks if all filters in the custom format match the release name.
// All filters use AND logic (all must match).
func (cfs *customFormatService) matchesAllFilters(format *CustomFormat, releaseName string) bool {
	// Parse release name once to extract metadata
	release := parser.Parse(releaseName)

	for _, filter := range format.Filters {
		if !cfs.filterMatches(filter, releaseName, release) {
			return false
		}
	}
	return true
}

// filterMatches checks if a single filter matches the release name.
// release is the parsed metadata extracted from the release name; can be nil for non-numeric conditions.
func (cfs *customFormatService) filterMatches(filter CustomFormatFilter, releaseName string, release *parser.Release) bool {
	switch filter.Condition {
	case ConditionEquals:
		// Case-insensitive exact match
		return strings.EqualFold(releaseName, filter.Value)
	case ConditionRegex:
		// Regex match
		re, err := regexp.Compile(filter.Value)
		if err != nil {
			return false
		}
		return re.MatchString(releaseName)
	case ConditionIn:
		// Member of list (comma-separated)
		values := strings.Split(filter.Value, ",")
		for _, v := range values {
			if strings.EqualFold(strings.TrimSpace(releaseName), strings.TrimSpace(v)) {
				return true
			}
		}
		return false
	case ConditionRange, ConditionGreaterThan, ConditionGreaterThanOrEqual, ConditionLessThan, ConditionLessThanOrEqual:
		// Numeric comparison using parsed release metadata
		return cfs.evaluateNumericCondition(filter, release)
	default:
		return false
	}
}

// evaluateNumericCondition evaluates numeric filter conditions based on parsed release metadata.
func (cfs *customFormatService) evaluateNumericCondition(filter CustomFormatFilter, release *parser.Release) bool {
	if release == nil {
		return false
	}

	// Determine the release field to compare
	var releaseValue int
	switch strings.ToLower(filter.Field) {
	case "bitdepth":
		releaseValue = release.Bitdepth
	case "year":
		releaseValue = release.Year
	case "resolution":
		releaseValue = release.Resolution
	default:
		// Unknown field
		return false
	}

	// If the field was not extracted (zero value), treat it as no match
	// Some fields like year and resolution might legitimately be 0, but bitdepth
	// of 0 means 8-bit which is valid; however, if the filter explicitly requires it,
	// it should be marked
	if releaseValue == 0 && filter.Field != "bitdepth" {
		return false
	}

	// Evaluate based on condition
	switch filter.Condition {
	case ConditionGreaterThan:
		filterValue, err := strconv.Atoi(filter.Value)
		if err != nil {
			return false
		}
		return releaseValue > filterValue
	case ConditionGreaterThanOrEqual:
		filterValue, err := strconv.Atoi(filter.Value)
		if err != nil {
			return false
		}
		return releaseValue >= filterValue
	case ConditionLessThan:
		filterValue, err := strconv.Atoi(filter.Value)
		if err != nil {
			return false
		}
		return releaseValue < filterValue
	case ConditionLessThanOrEqual:
		filterValue, err := strconv.Atoi(filter.Value)
		if err != nil {
			return false
		}
		return releaseValue <= filterValue
	case ConditionRange:
		// Range format: "min,max" (e.g., "8,10" for bitdepth range)
		parts := strings.Split(filter.Value, ",")
		if len(parts) != 2 {
			return false
		}
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])

		// Handle open-ended ranges (e.g., ",10" or "8,")
		if minStr == "" && maxStr == "" {
			return false // Both empty is invalid
		}
		if minStr != "" {
			minVal, err := strconv.Atoi(minStr)
			if err != nil {
				return false
			}
			if releaseValue < minVal {
				return false
			}
		}
		if maxStr != "" {
			maxVal, err := strconv.Atoi(maxStr)
			if err != nil {
				return false
			}
			if releaseValue > maxVal {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// ValidateCustomFormat validates the syntax and semantics of a custom format.
func (cfs *customFormatService) ValidateCustomFormat(cf *CustomFormat) error {
	if cf.Name == "" {
		return fmt.Errorf("movies: custom format name required")
	}
	if len(cf.Filters) == 0 {
		return fmt.Errorf("movies: custom format requires at least one filter")
	}

	// Validate each filter
	allowedFields := map[string]bool{
		"codec":      true,
		"source":     true,
		"year":       true,
		"bitdepth":   true,
		"resolution": true,
		"hdr":        true,
		"audio":      true,
		"language":   true,
	}

	for i, filter := range cf.Filters {
		if filter.Field == "" {
			return fmt.Errorf("movies: custom format filter %d: field required", i)
		}
		if !allowedFields[filter.Field] {
			return fmt.Errorf("movies: custom format filter %d: invalid field %q", i, filter.Field)
		}

		// Validate condition
		validConditions := map[CustomFormatFilterCondition]bool{
			ConditionEquals:             true,
			ConditionRegex:              true,
			ConditionRange:              true,
			ConditionIn:                 true,
			ConditionGreaterThan:        true,
			ConditionGreaterThanOrEqual: true,
			ConditionLessThan:           true,
			ConditionLessThanOrEqual:    true,
		}
		if !validConditions[filter.Condition] {
			return fmt.Errorf("movies: custom format filter %d: invalid condition %q", i, filter.Condition)
		}

		if filter.Value == "" {
			return fmt.Errorf("movies: custom format filter %d: value required", i)
		}

		// Validate syntax based on condition
		if err := cfs.validateFilterValue(filter); err != nil {
			return fmt.Errorf("movies: custom format filter %d: %w", i, err)
		}
	}

	return nil
}

// validateFilterValue validates the value based on the filter condition.
func (cfs *customFormatService) validateFilterValue(filter CustomFormatFilter) error {
	switch filter.Condition {
	case ConditionRegex:
		// Validate regex syntax (with timeout to prevent ReDoS)
		_, err := regexp.Compile(filter.Value)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	case ConditionRange:
		// Validate range format: "min,max" or "min," or ",max"
		parts := strings.Split(filter.Value, ",")
		if len(parts) != 2 {
			return fmt.Errorf("range format must be 'min,max' or 'min,' or ',max'")
		}
		if parts[0] != "" {
			if _, err := strconv.Atoi(parts[0]); err != nil {
				return fmt.Errorf("min value must be integer: %w", err)
			}
		}
		if parts[1] != "" {
			if _, err := strconv.Atoi(parts[1]); err != nil {
				return fmt.Errorf("max value must be integer: %w", err)
			}
		}
		if parts[0] == "" && parts[1] == "" {
			return fmt.Errorf("range cannot be empty")
		}
	case ConditionGreaterThan, ConditionGreaterThanOrEqual, ConditionLessThan, ConditionLessThanOrEqual:
		// Validate numeric comparison value
		if _, err := strconv.Atoi(filter.Value); err != nil {
			return fmt.Errorf("value must be integer for numeric comparison: %w", err)
		}
	case ConditionIn:
		// Validate comma-separated list (no validation needed, just ensure not empty)
		if strings.TrimSpace(filter.Value) == "" {
			return fmt.Errorf("list cannot be empty")
		}
	}
	return nil
}
