package music

import "testing"

func TestBaseAlbumTitleOnlyStripsKnownEditionAndQualitySuffixes(t *testing.T) {
	for _, tt := range []struct{ title, want string }{
		{"From Zero (Deluxe Edition)", "From Zero"},
		{"From Zero [Deluxe] [24bit-48kHz]", "From Zero"},
		{"From Zero - Deluxe Edition", "From Zero"},
		{"From Zero (24-bit / 96 kHz)", "From Zero"},
		{"From Zero (Hi-Res)", "From Zero"},
		{"From Zero (Deluxe) 24bit 44.1kHz", "From Zero"},
		{"From Zero 24-bit", "From Zero"},
		{"From Zero (Live)", "From Zero (Live)"},
		{"From Zero (Remixes)", "From Zero (Remixes)"},
		{"From Zero (Part II)", "From Zero (Part II)"},
		{"From Zero (Unrelated Label)", "From Zero (Unrelated Label)"},
		{"Deluxe", "Deluxe"},
		{"(Deluxe Edition)", "(Deluxe Edition)"},
	} {
		if got := BaseAlbumTitle(tt.title); got != tt.want {
			t.Errorf("BaseAlbumTitle(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
