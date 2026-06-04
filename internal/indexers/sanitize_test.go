package indexers_test

import (
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
)

func TestSanitizeTerm(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		in            string
		seasonEpisode bool
		want          string
	}{
		{"apostrophe stripped not spaced", "Clarkson's Farm", false, "Clarksons Farm"},
		{"smart apostrophe stripped", "Clarkson\u2019s Farm", false, "Clarksons Farm"},
		{"contraction", "Don't Look Up", false, "Dont Look Up"},
		{"double quotes stripped", "\"Star Wars\"", false, "Star Wars"},
		{"smart double quotes stripped", "\u201CStar Wars\u201D", false, "Star Wars"},
		{"backtick stripped", "Rock`n Roll", false, "Rockn Roll"},
		{"colon becomes space", "Mission: Impossible", false, "Mission Impossible"},
		{"collapses whitespace", "a   b", false, "a b"},
		{"season stripped with params", "The Show S01E02", true, "The Show"},
		{"season kept without params", "The Show S01E02", false, "The Show S01E02"},
		{"empty", "", false, ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := indexers.SanitizeTerm(tc.in, tc.seasonEpisode)
			if got != tc.want {
				t.Errorf("SanitizeTerm(%q, %v) = %q, want %q", tc.in, tc.seasonEpisode, got, tc.want)
			}
		})
	}
}
