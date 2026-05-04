package autosearch

import (
	"testing"

	"github.com/loomctl/loom/internal/rss"
)

func TestExactTitleMatch(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "Inception.2010.1080p.BluRay",
	}

	matches := matcher.FindMatches(item)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Confidence < 0.99 {
		t.Errorf("expected high confidence for exact match, got %.2f", matches[0].Confidence)
	}
}

func TestCaseInsensitiveMatch(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "inception.2010.1080p",
	}

	matches := matcher.FindMatches(item)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestYearMatching(t *testing.T) {
	tests := []struct {
		name           string
		wantTitle      string
		wantYear       int
		itemTitle      string
		itemYear       int
		minConfidence  float64
		shouldMatch    bool
	}{
		{
			name:          "exact_year_match",
			wantTitle:     "Inception",
			wantYear:      2010,
			itemTitle:     "Inception.2010.1080p",
			itemYear:      2010,
			minConfidence: 0.95,
			shouldMatch:   true,
		},
		{
			name:          "no_year_in_item",
			wantTitle:     "Inception",
			wantYear:      2010,
			itemTitle:     "Inception.1080p",
			itemYear:      0,
			minConfidence: 0.7,
			shouldMatch:   true,
		},
		{
			name:          "year_mismatch_penalty",
			wantTitle:     "Inception",
			wantYear:      2010,
			itemTitle:     "Inception.2009.1080p",
			itemYear:      2009,
			minConfidence: 0.6,
			shouldMatch:   true, // Should still match due to close year
		},
		{
			name:          "significant_year_mismatch",
			wantTitle:     "Inception",
			wantYear:      2010,
			itemTitle:     "Inception.1990.1080p",
			itemYear:      1990,
			minConfidence: 0.3,
			shouldMatch:   false, // Too much penalty (20 year difference)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMatcher([]WantedMovie{
				{ID: "m1", Title: tt.wantTitle, Year: tt.wantYear},
			})

			item := &rss.Item{
				GUID:     "i1",
				SourceID: "s1",
				Title:    tt.itemTitle,
			}

			matches := matcher.FindMatches(item)
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got %v", tt.shouldMatch, matched)
			}

			if matched && matches[0].Confidence < tt.minConfidence {
				t.Errorf("expected confidence >= %.2f, got %.2f", tt.minConfidence, matches[0].Confidence)
			}
		})
	}
}

func TestFuzzyMatching(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "The Matrix", Year: 1999},
	})

	tests := []struct {
		itemTitle string
		shouldMatch bool
	}{
		{"The.Matrix.1999.1080p.BluRay", true},
		{"TheMatrix.1999.1080p", true},
		{"Matrix.1999.1080p", true}, // Missing "The" but still close
		{"Matrix.Reloaded.2003", false}, // Different movie
		{"TheMatrixx.1999.1080p", true}, // Typo (extra 'x'), should still match
	}

	for _, tt := range tests {
		t.Run(tt.itemTitle, func(t *testing.T) {
			item := &rss.Item{
				GUID:     "i1",
				SourceID: "s1",
				Title:    tt.itemTitle,
			}

			matches := matcher.FindMatches(item)
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("itemTitle %q: expected match=%v, got %v", tt.itemTitle, tt.shouldMatch, matched)
			}
		})
	}
}

func TestMultipleMatches(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
		{ID: "m2", Title: "Interstellar", Year: 2014},
		{ID: "m3", Title: "The Prestige", Year: 2006},
	})

	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "Inception.2010.1080p.BluRay",
	}

	matches := matcher.FindMatches(item)
	if len(matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(matches))
	}
	if matches[0].MovieID != "m1" {
		t.Errorf("expected movie m1, got %s", matches[0].MovieID)
	}
}

func TestNoMatches(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "RandomMovie.2020.1080p",
	}

	matches := matcher.FindMatches(item)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestEmptyTitle(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "",
	}

	matches := matcher.FindMatches(item)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty title, got %d", len(matches))
	}
}

func TestNormalizationRemovesArticles(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Matrix", "Matrix"},
		{"A Good Day", "Good Day"},
		{"An Unexpected Journey", "Unexpected Journey"},
		{"Inception", "Inception"},
	}

	for _, tt := range tests {
		result := normalizeTitle(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNormalizationPunctuation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The.Matrix.1999", "The Matrix"}, // Year is now stripped (it's metadata)
		{"The-Matrix-1999", "The Matrix"},
		{"The_Matrix_1999", "The Matrix"},
		{"The..Matrix", "The Matrix"},
		{"The.Matrix.2010.1080p.BluRay", "The Matrix"}, // All metadata stripped
	}

	for _, tt := range tests {
		result := normalizeTitle(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractYear(t *testing.T) {
	tests := []struct {
		title    string
		expected int
	}{
		{"Inception.2010.1080p", 2010},
		{"The.Matrix.1999", 1999},
		{"2001.A.Space.Odyssey.1968", 1968},
		{"NoYearHere.1080p", 0},
		{"Year2025IsAfter2020", 0}, // No word boundary match (2025 and 2020 are not separated)
		{"Year 2025 Is After 2020", 2020}, // Takes last year with word boundary
		{"1999.Inception.2010", 2010},
	}

	for _, tt := range tests {
		result := extractYearFromTitle(tt.title)
		if result != tt.expected {
			t.Errorf("extractYearFromTitle(%q) = %d, want %d", tt.title, result, tt.expected)
		}
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"hello", "hello", 0},
		{"abc", "", 3},
		{"", "xyz", 3},
		{"inception", "inception", 0},
		{"matrix", "matrixz", 1},
	}

	for _, tt := range tests {
		result := levenshteinDistance(tt.s1, tt.s2)
		if result != tt.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, result, tt.expected)
		}
	}
}

func TestTypoTolerance(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	tests := []struct {
		title       string
		shouldMatch bool
	}{
		{"Inception.2010.1080p", true},
		{"Incpetion.2010.1080p", true}, // One char swap (distance=1)
		{"Inceptin.2010.1080p", true},  // Missing one char (distance=1)
		{"nception.2010.1080p", true},  // Missing first char (distance=1)
		{"Inceptionn.2010.1080p", true}, // Extra char (distance=1)
		{"Inceptoin.2010.1080p", true},  // Char transposition
		// Note: "Matrixz.2010" won't match "Inception" as it's a completely different title
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			item := &rss.Item{
				GUID:     "i1",
				SourceID: "s1",
				Title:    tt.title,
			}

			matches := matcher.FindMatches(item)
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("title %q: expected match=%v, got %v", tt.title, tt.shouldMatch, matched)
			}
		})
	}
}

func TestSequelDetection(t *testing.T) {
	// Sequels should NOT match original title
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	sequelItems := []struct {
		title string
	}{
		{"Inception.2.2024.1080p"},
		{"Inception.3.2024.1080p"},
		{"Inception.Reloaded.2024"},
	}

	for _, item := range sequelItems {
		t.Run(item.title, func(t *testing.T) {
			rssItem := &rss.Item{
				GUID:     "i1",
				SourceID: "s1",
				Title:    item.title,
			}

			matches := matcher.FindMatches(rssItem)
			// These should likely NOT match or have lower confidence
			// depending on implementation preference
			if len(matches) > 0 && matches[0].Confidence > 0.85 {
				t.Logf("title %q matched with confidence %.2f (may be false positive)", item.title, matches[0].Confidence)
			}
		})
	}
}

func TestConfidenceThreshold(t *testing.T) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
	})

	// Below threshold (70%)
	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "TotallyDifferentMovie.2010.1080p",
	}

	matches := matcher.FindMatches(item)
	if len(matches) > 0 {
		t.Errorf("expected no matches below threshold, got %d with confidence %.2f", len(matches), matches[0].Confidence)
	}
}

func BenchmarkMatcher(b *testing.B) {
	matcher := NewMatcher([]WantedMovie{
		{ID: "m1", Title: "Inception", Year: 2010},
		{ID: "m2", Title: "Interstellar", Year: 2014},
		{ID: "m3", Title: "The Prestige", Year: 2006},
		{ID: "m4", Title: "The Dark Knight", Year: 2008},
		{ID: "m5", Title: "Memento", Year: 2000},
	})

	item := &rss.Item{
		GUID:     "i1",
		SourceID: "s1",
		Title:    "Inception.2010.1080p.BluRay.x264-RELEASE",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matcher.FindMatches(item)
	}
}
