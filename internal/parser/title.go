package parser

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Regex patterns ported from Sonarr's Parser.cs.
var (
	// normalizeRegex strips mid-title articles (a/an/the/and/or/of) and
	// all non-word characters. The negative look-behind/ahead prevent
	// stripping the first/last word even if it is an article.
	// Sonarr: ((?:\b|_)(?<!^)([aà](?!$)|an|the|and|or|of)(?!$)(?:\b|_))|\W|_
	//
	// Go's regexp2 doesn't support look-arounds, so we implement the
	// article-stripping + punctuation removal in two steps.
	reArticleMid = regexp.MustCompile(`(?i)(?:\b|_)(a|aà|an|the|and|or|of)(?:\b|_)`)
	reNonWord    = regexp.MustCompile(`[\W_]+`)

	// percentRegex replaces % when preceded by digits (e.g. "24%" → "24 percent").
	// Sonarr: (?<=\b\d+)%
	// Go doesn't support look-behind, so we capture the digits.
	rePercent = regexp.MustCompile(`(\d+)%`)
)

// CleanSeriesTitle is a faithful port of Sonarr's Parser.CleanSeriesTitle().
// It normalizes a title for comparison: strips mid-title articles, removes
// punctuation/underscores, lowercases, and removes diacritics. The result
// is suitable for fuzzy equality checks between a search request title and
// a parsed release title.
//
// This MUST NOT be used for building indexer query strings — use the raw
// title for that.
func CleanSeriesTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return ""
	}

	// If numeric-only, return as-is (Sonarr: if title is all digits, return).
	allDigits := true
	for _, r := range title {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	if allDigits {
		return title
	}

	// Replace trailing % on numbers with "percent".
	t := rePercent.ReplaceAllString(title, "${1} percent")

	// Lowercase.
	t = strings.ToLower(t)

	// Remove diacritics.
	t = RemoveDiacritics(t)

	// Strip articles that are NOT at the very start or end.
	// We do this by splitting into words, removing mid-position articles,
	// then rejoining.
	words := strings.Fields(reNonWord.ReplaceAllString(t, " "))
	if len(words) > 2 {
		filtered := make([]string, 0, len(words))
		articles := map[string]bool{
			"a": true, "an": true, "the": true,
			"and": true, "or": true, "of": true,
		}
		for i, w := range words {
			// Keep first and last word unconditionally.
			if i == 0 || i == len(words)-1 {
				filtered = append(filtered, w)
				continue
			}
			if !articles[w] {
				filtered = append(filtered, w)
			}
		}
		words = filtered
	}

	return strings.Join(words, "")
}

// CleanMovieTitle normalizes a movie title for comparison. Currently the
// same logic as CleanSeriesTitle, but split out so we can diverge later
// if needed (Radarr has subtle differences in some edge cases).
func CleanMovieTitle(title string) string {
	return CleanSeriesTitle(title)
}

// NewsnabifyTitle prepares a title for use in a Newznab/Torznab q= parameter.
// It replaces "+" with space (since + is interpreted as space by many
// indexers) and then URL-encodes the result.
//
// IMPORTANT: if using url.Values, do NOT call this — url.Values.Encode()
// handles escaping. This is only for manual URL string building.
func NewsnabifyTitle(title string) string {
	t := strings.ReplaceAll(title, "+", " ")
	return url.QueryEscape(t)
}

// RemoveDiacritics strips combining diacritical marks from a string,
// converting characters like "é" to "e", "ñ" to "n", etc.
func RemoveDiacritics(s string) string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Mn = nonspacing mark
	}), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}
