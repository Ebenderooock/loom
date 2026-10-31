package music

import "testing"

func TestBaseTrackTitle(t *testing.T) {
	cases := map[string]string{
		"Heavy (feat. Kiiara)":          "Heavy",
		"Heavy feat. Kiiara":            "Heavy",
		"Heavy ft Kiiara":               "Heavy",
		"Heavy (album version)":         "Heavy",
		"Numb [Remastered] (Live)":      "Numb",
		"(Intro)":                       "(Intro)",
		"Feathers":                      "Feathers",
		"In the End":                    "In the End",
		"Somewhere I Belong (feat. X) ": "Somewhere I Belong",
	}
	for in, want := range cases {
		if got := BaseTrackTitle(in); got != want {
			t.Errorf("BaseTrackTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
