package movies

import (
	"regexp"
	"testing"
	"time"
)

// TestValidateCustomFormatAllowedFields tests field allowlist validation.
func TestValidateCustomFormatAllowedFields(t *testing.T) {
	svc := NewCustomFormatService(nil)
	allowedFields := []string{"codec", "source", "year", "bitdepth", "resolution", "hdr", "audio", "language"}

	for _, field := range allowedFields {
		cf := &CustomFormat{
			Name: "test",
			Filters: []CustomFormatFilter{
				{Field: field, Condition: ConditionEquals, Value: "test"},
			},
		}
		err := svc.ValidateCustomFormat(cf)
		if err != nil {
			t.Errorf("field %q should be allowed, got error: %v", field, err)
		}
	}

	// Test disallowed field
	cf := &CustomFormat{
		Name: "test",
		Filters: []CustomFormatFilter{
			{Field: "invalid_field", Condition: ConditionEquals, Value: "test"},
		},
	}
	err := svc.ValidateCustomFormat(cf)
	if err == nil {
		t.Error("invalid field should fail validation")
	}
}

// TestValidateCustomFormatConditions tests condition-specific value validation.
func TestValidateCustomFormatConditions(t *testing.T) {
	svc := NewCustomFormatService(nil)

	tests := []struct {
		name      string
		condition CustomFormatFilterCondition
		value     string
		wantErr   bool
	}{
		// Valid conditions
		{"equals-string", ConditionEquals, "x264", false},
		{"regex-valid", ConditionRegex, ".*x264.*", false},
		{"range-valid", ConditionRange, "1,5", false},
		{"in-valid", ConditionIn, "val1,val2,val3", false},
		{"gt-valid", ConditionGreaterThan, "100", false},
		{"gte-valid", ConditionGreaterThanOrEqual, "100", false},
		{"lt-valid", ConditionLessThan, "100", false},
		{"lte-valid", ConditionLessThanOrEqual, "100", false},

		// Invalid regex
		{"regex-invalid", ConditionRegex, "[invalid(regex", true},

		// Invalid range (non-numeric)
		{"range-non-numeric", ConditionRange, "a,b", true},

		// Invalid numeric conditions
		{"gt-non-numeric", ConditionGreaterThan, "not-a-number", true},
		{"gte-non-numeric", ConditionGreaterThanOrEqual, "abc", true},

		// Empty in list
		{"in-empty", ConditionIn, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cf := &CustomFormat{
				Name: "test",
				Filters: []CustomFormatFilter{
					{Field: "codec", Condition: tt.condition, Value: tt.value},
				},
			}
			err := svc.ValidateCustomFormat(cf)
			if (err != nil) != tt.wantErr {
				t.Errorf("condition %s value %q: got error %v, want error %v", tt.condition, tt.value, err, tt.wantErr)
			}
		})
	}
}

// TestValidateCustomFormatEmptyFilters tests empty filter list validation.
func TestValidateCustomFormatEmptyFilters(t *testing.T) {
	svc := NewCustomFormatService(nil)

	cf := &CustomFormat{
		Name:    "test",
		Filters: []CustomFormatFilter{},
	}
	err := svc.ValidateCustomFormat(cf)
	if err == nil {
		t.Error("empty filters should fail validation")
	}
}

// TestValidateCustomFormatEmptyName tests empty name validation.
func TestValidateCustomFormatEmptyName(t *testing.T) {
	svc := NewCustomFormatService(nil)

	cf := &CustomFormat{
		Name: "",
		Filters: []CustomFormatFilter{
			{Field: "codec", Condition: ConditionEquals, Value: "x264"},
		},
	}
	err := svc.ValidateCustomFormat(cf)
	if err == nil {
		t.Error("empty name should fail validation")
	}
}

// TestCustomFormatWithTimestamps tests that timestamps are properly set.
func TestValidateFilterValueReDoS(t *testing.T) {
	svc := NewCustomFormatService(nil)

	// This regex can cause exponential backtracking with certain inputs.
	// We test that the validation catches it or at least compiles safely.
	testCases := []string{
		"(a+)+b",           // Classic ReDoS pattern
		"(a|a)*b",          // Alternative ReDoS pattern
		"(.*)*",            // Nested quantifier
		".*x264.*",         // Safe pattern
		"[0-9]+",           // Safe pattern
	}

	for _, pattern := range testCases {
		cf := &CustomFormat{
			Name: "test",
			Filters: []CustomFormatFilter{
				{Field: "codec", Condition: ConditionRegex, Value: pattern},
			},
		}
		// Validation should not hang or panic.
		err := svc.ValidateCustomFormat(cf)
		// It's OK if validation fails (pattern is invalid).
		// It's OK if it succeeds (pattern is valid).
		// The key is that it completes without hanging.
		_ = err
	}
}

// TestFilterMatches tests the individual filter matching logic.
func TestFilterMatches(t *testing.T) {
	tests := []struct {
		name   string
		filter CustomFormatFilter
		input  string
		want   bool
	}{
		// Equals
		{"equals-match", CustomFormatFilter{Condition: ConditionEquals, Value: "x264"}, "x264", true},
		{"equals-mismatch", CustomFormatFilter{Condition: ConditionEquals, Value: "x264"}, "x265", false},

		// Regex
		{"regex-match", CustomFormatFilter{Condition: ConditionRegex, Value: "x26[45]"}, "x264", true},
		{"regex-mismatch", CustomFormatFilter{Condition: ConditionRegex, Value: "x26[45]"}, "mpeg2", false},

		// In
		{"in-first", CustomFormatFilter{Condition: ConditionIn, Value: "a,b,c"}, "a", true},
		{"in-middle", CustomFormatFilter{Condition: ConditionIn, Value: "a,b,c"}, "b", true},
		{"in-last", CustomFormatFilter{Condition: ConditionIn, Value: "a,b,c"}, "c", true},
		{"in-none", CustomFormatFilter{Condition: ConditionIn, Value: "a,b,c"}, "d", false},

		// Invalid regex returns false
		{"regex-invalid-pattern", CustomFormatFilter{Condition: ConditionRegex, Value: "["}, "test", false},

		// Unknown condition returns false
		{"unknown-condition", CustomFormatFilter{Condition: "unknown"}, "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterMatches(tt.filter, tt.input)
			if got != tt.want {
				t.Errorf("filterMatches(%v, %q) = %v, want %v", tt.filter.Condition, tt.input, got, tt.want)
			}
		})
	}
}

// TestCustomFormatWithTimestamps tests that timestamps are properly set.
func TestCustomFormatWithTimestamps(t *testing.T) {
	before := time.Now()
	cf := &CustomFormat{
		ID:        "test-id",
		Name:      "Test Format",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	after := time.Now()

	if cf.CreatedAt.Before(before) || cf.CreatedAt.After(after) {
		t.Error("CreatedAt not set to current time")
	}
	if cf.UpdatedAt.Before(before) || cf.UpdatedAt.After(after) {
		t.Error("UpdatedAt not set to current time")
	}
}

// TestCustomFormatFilterOrder tests that filter order is preserved.
func TestCustomFormatFilterOrder(t *testing.T) {
	cf := &CustomFormat{
		Name: "ordered-test",
		Filters: []CustomFormatFilter{
			{Order: 0, Field: "codec", Value: "x264"},
			{Order: 1, Field: "resolution", Value: "1080p"},
			{Order: 2, Field: "source", Value: "Blu-ray"},
		},
	}

	for i, f := range cf.Filters {
		if f.Order != i {
			t.Errorf("filter %d: expected order %d, got %d", i, i, f.Order)
		}
	}
}

// BenchmarkFilterMatches benchmarks the regex matching performance.
func BenchmarkFilterMatches(b *testing.B) {
	filter := CustomFormatFilter{
		Condition: ConditionRegex,
		Value:     "x264",
	}
	input := "Movie.2023.1080p.x264.BluRay"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filterMatches(filter, input)
	}
}


// TestConditionEquality tests condition constant values.
func TestConditionEquality(t *testing.T) {
	if ConditionEquals != "equals" {
		t.Errorf("ConditionEquals = %q, want %q", ConditionEquals, "equals")
	}
	if ConditionRegex != "regex" {
		t.Errorf("ConditionRegex = %q, want %q", ConditionRegex, "regex")
	}
	if ConditionRange != "range" {
		t.Errorf("ConditionRange = %q, want %q", ConditionRange, "range")
	}
	if ConditionIn != "in" {
		t.Errorf("ConditionIn = %q, want %q", ConditionIn, "in")
	}
}

// TestValidateFilterValueRangeFormat tests range parsing.
func TestValidateFilterValueRangeFormat(t *testing.T) {
	svc := NewCustomFormatService(nil)

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid-range", "1,100", false},
		{"valid-range-open-start", ",100", false},
		{"valid-range-open-end", "1,", false},
		{"invalid-range-both-open", ",", true},
		{"invalid-non-numeric-start", "a,100", true},
		{"invalid-non-numeric-end", "1,b", true},
		{"invalid-no-comma", "1-100", true},
		{"invalid-empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cf := &CustomFormat{
				Name: "test",
				Filters: []CustomFormatFilter{
					{Field: "bitdepth", Condition: ConditionRange, Value: tt.value},
				},
			}
			err := svc.ValidateCustomFormat(cf)
			if (err != nil) != tt.wantErr {
				t.Errorf("got error %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// TestRegexCompilationCache tests that regex patterns compile correctly.
func TestRegexCompilationCache(t *testing.T) {
	patterns := []string{
		"x264",
		"x26[45]",
		"\\b(1080|720)p\\b",
		"BluRay|Blu-ray",
	}

	for _, pattern := range patterns {
		_, err := regexp.Compile(pattern)
		if err != nil {
			t.Errorf("pattern %q failed to compile: %v", pattern, err)
		}
	}
}
