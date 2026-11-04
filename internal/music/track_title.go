package music

import (
	"regexp"
	"strings"
)

var (
	trailingQualifierRe = regexp.MustCompile(`\s*[\(\[][^\(\)\[\]]*[\)\]]\s*$`)
	featuringSuffixRe   = regexp.MustCompile(`(?i)\s+(?:feat\.?|ft\.?|featuring)\s+.*$`)
)

// BaseTrackTitle strips trailing qualifiers such as "(album version)",
// "[Remastered]" and "feat. Artist" so tagged titles can be compared with
// MusicBrainz track titles that credit guests or editions differently.
func BaseTrackTitle(title string) string {
	base := strings.TrimSpace(title)
	for {
		next := featuringSuffixRe.ReplaceAllString(base, "")
		next = strings.TrimSpace(trailingQualifierRe.ReplaceAllString(next, ""))
		if next == base || next == "" {
			return base
		}
		base = next
	}
}

// MatchTrackByBaseTitle matches a tagged title after stripping qualifiers.
// It first compares the tag's base title with full track titles (so
// "Heavy (feat. Kiiara)" prefers "Heavy" over "Heavy (remix)"), then with
// track base titles. A tier with more than one candidate returns nil.
func MatchTrackByBaseTitle(tracks []*Track, title string, normalize func(string) string) *Track {
	want := normalize(BaseTrackTitle(title))
	if want == "" {
		return nil
	}
	for _, candidateTitle := range []func(*Track) string{
		func(t *Track) string { return t.Title },
		func(t *Track) string { return BaseTrackTitle(t.Title) },
	} {
		var match *Track
		ambiguous := false
		for _, candidate := range tracks {
			if normalize(candidateTitle(candidate)) != want {
				continue
			}
			if match != nil {
				ambiguous = true
				break
			}
			match = candidate
		}
		if ambiguous {
			return nil
		}
		if match != nil {
			return match
		}
	}
	return nil
}
