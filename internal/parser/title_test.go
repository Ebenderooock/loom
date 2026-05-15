package parser

import "testing"

func TestCleanSeriesTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"basic", "The Walking Dead", "thewalkingdead"},
		{"mid articles stripped", "Lord of the Rings", "lordrings"},
		{"leading article kept", "The Punisher", "thepunisher"},
		{"trailing article kept", "Castle of", "castleof"},
		{"punctuation stripped", "Marvel's Agents of S.H.I.E.L.D.", "marvelsagentsshield"},
		{"underscores stripped", "Game_of_Thrones", "gamethrones"},
		{"diacritics removed", "Amélie", "amelie"},
		{"numeric only returns as-is", "24", "24"},
		{"percent replaced", "The 100%", "the100percent"},
		{"empty returns empty", "", ""},
		{"whitespace only returns empty", "   ", ""},
		{"single word", "Dexter", "dexter"},
		{"two words no articles", "Breaking Bad", "breakingbad"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CleanSeriesTitle(tc.input)
			if got != tc.want {
				t.Errorf("CleanSeriesTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRemoveDiacritics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input, want string
	}{
		{"Amélie", "Amelie"},
		{"naïve café", "naive cafe"},
		{"Ñoño", "Nono"},
		{"plain ascii", "plain ascii"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := RemoveDiacritics(tc.input)
			if got != tc.want {
				t.Errorf("RemoveDiacritics(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNewsnabifyTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input, want string
	}{
		{"Breaking Bad", "Breaking+Bad"},
		{"C++ Programming", "C+++Programming"},
		{"hello world", "hello+world"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := NewsnabifyTitle(tc.input)
			if got != tc.want {
				t.Errorf("NewsnabifyTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
