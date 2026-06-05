package autosearch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/rss"
)

// Match represents a potential media match from an RSS item against a library want.
type Match struct {
	ItemID     string  // RSS item ID
	SourceID   string  // RSS source ID
	MovieID    string  // Library movie ID
	Title      string  // Movie title
	Year       int     // Movie year
	Confidence float64 // Match confidence (0.0-1.0)
	Reason     string  // Why it matched (for debugging)
}

// WantedMovie represents a monitored movie in the library.
type WantedMovie struct {
	ID    string
	Title string
	Year  int
}

// Matcher performs fuzzy matching between RSS items and library wants.
type Matcher struct {
	wants []WantedMovie
}

// NewMatcher creates a new title matcher with a list of wanted movies.
func NewMatcher(wants []WantedMovie) *Matcher {
	return &Matcher{
		wants: wants,
	}
}

// FindMatches compares an RSS item against all library wants and returns potential matches.
// Uses title fuzzy matching with optional year verification.
func (m *Matcher) FindMatches(item *rss.Item) []Match {
	var matches []Match

	// Normalize item title
	itemTitle := normalizeTitle(item.Title)
	if itemTitle == "" {
		return nil
	}

	// Extract year from item (if available in title)
	itemYear := extractYearFromTitle(item.Title)

	for _, want := range m.wants {
		confidence, reason := m.matchTitles(itemTitle, want.Title, itemYear, want.Year)
		if confidence >= 0.7 { // 70% confidence threshold
			matches = append(matches, Match{
				ItemID:     item.GUID,
				SourceID:   item.SourceID,
				MovieID:    want.ID,
				Title:      want.Title,
				Year:       want.Year,
				Confidence: confidence,
				Reason:     reason,
			})
		}
	}

	return matches
}

// matchTitles compares normalized titles and returns confidence score (0.0-1.0).
// Factors in:
// - Levenshtein distance between titles
// - Optional year match (exact match boosts confidence)
func (m *Matcher) matchTitles(itemTitle, wantTitle string, itemYear, wantYear int) (float64, string) {
	// Exact match (case-insensitive)
	if strings.EqualFold(itemTitle, wantTitle) {
		confidence := 1.0
		reason := "exact_title_match"

		// Apply year penalty even for exact title match
		if itemYear > 0 && wantYear > 0 {
			yearDiff := abs(itemYear - wantYear)
			if yearDiff == 0 {
				confidence = 1.0
				reason = "exact_title_and_year_match"
			} else if yearDiff == 1 {
				confidence = 0.95
				reason = "exact_title_within_1year"
			} else if yearDiff > 5 {
				confidence = 0.5 // Heavy penalty for large mismatch (>5 years)
				reason = fmt.Sprintf("exact_title_large_year_mismatch (got %d, want %d)", itemYear, wantYear)
			} else {
				confidence = 0.8 // Penalty for significant year mismatch (2-5 years)
				reason = fmt.Sprintf("exact_title_year_mismatch (got %d, want %d)", itemYear, wantYear)
			}
		}
		return confidence, reason
	}

	// Levenshtein distance
	distance := levenshteinDistance(strings.ToLower(itemTitle), strings.ToLower(wantTitle))
	maxLen := max(len(itemTitle), len(wantTitle))
	if maxLen == 0 {
		return 0, "empty_titles"
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)

	// Year boost if both years available and match
	yearBoost := 0.0
	yearPenalty := 0.0
	if itemYear > 0 && wantYear > 0 {
		yearDiff := abs(itemYear - wantYear)
		if yearDiff == 0 {
			yearBoost = 0.15 // Significant boost for year match
		} else if yearDiff == 1 {
			yearBoost = 0.05 // Small boost for within 1 year
		} else if yearDiff > 5 {
			yearPenalty = 0.5 // Heavy penalty for large mismatch (>5 years)
		} else {
			yearPenalty = 0.2 // Penalty for significant year mismatch (2-5 years)
		}
	}

	confidence := similarity + yearBoost - yearPenalty
	if confidence > 1.0 {
		confidence = 1.0
	}

	// Determine reason
	reason := fmt.Sprintf("fuzzy_match_%.1f%%", confidence*100)
	if yearBoost > 0 {
		reason += "_with_year_match"
	}
	if yearPenalty > 0 {
		reason = fmt.Sprintf("fuzzy_match_%.1f%%_year_mismatch", confidence*100)
	}

	return confidence, reason
}

// normalizeTitle removes common punctuation, release info, and extra whitespace from a title.
func normalizeTitle(title string) string {
	// Remove common punctuation and prefixes/suffixes
	title = strings.TrimSpace(title)

	// Remove leading article prefixes (common in titles)
	prefixes := []string{"the ", "a ", "an "}
	lower := strings.ToLower(title)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			title = title[len(prefix):]
			break
		}
	}

	// Remove dots, dashes, underscores (common in release names)
	title = strings.NewReplacer(
		".", " ",
		"-", " ",
		"_", " ",
	).Replace(title)

	// Remove extra whitespace
	title = strings.Join(strings.Fields(title), " ")

	// Extract just the title part (everything before resolution or other video metadata)
	// Common patterns: 1080p, 720p, 480p, 4K, 2160p, 10bit, HDTV, BluRay, WebDL, etc.
	re := regexp.MustCompile(`(?i)\s+(1080p|720p|480p|360p|4k|2160p|1440p|10bit|8bit|HDTV|BluRay|BRRip|WebDL|WebRip|DvdRip|Remux|x264|x265|h264|h265|HEVC|AV1|VP9|aac|ac3|dts|flac).*$`)
	title = re.ReplaceAllString(title, "")

	// Remove year patterns too  (they'll be extracted separately)
	re = regexp.MustCompile(`(?i)\s+\b(19\d{2}|20\d{2})\b.*$`)
	title = re.ReplaceAllString(title, "")

	return strings.TrimSpace(title)
}

// extractYearFromTitle attempts to find a 4-digit year (1900-2100) in the title.
func extractYearFromTitle(title string) int {
	re := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	matches := re.FindAllString(title, -1)
	if len(matches) > 0 {
		// Take the last year found (usually the movie year, not a page number)
		if year, err := strconv.Atoi(matches[len(matches)-1]); err == nil {
			return year
		}
	}
	return 0
}

// levenshteinDistance computes the Levenshtein distance between two strings.
// Used for fuzzy title matching.
func levenshteinDistance(s1, s2 string) int {
	len1, len2 := len(s1), len(s2)
	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	// Create distance matrix
	d := make([][]int, len1+1)
	for i := range d {
		d[i] = make([]int, len2+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			d[i][j] = min(
				d[i-1][j]+1, // deletion
				min(
					d[i][j-1]+1,      // insertion
					d[i-1][j-1]+cost, // substitution
				),
			)
		}
	}

	return d[len1][len2]
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
