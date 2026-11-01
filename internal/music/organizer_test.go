package music

import (
	"path/filepath"
	"testing"
)

func TestBuildTrackPath_SingleDisc(t *testing.T) {
	artist := &Artist{Name: "Pink Floyd"}
	album := &Album{Title: "The Dark Side of the Moon", ReleaseDate: "1973-03-01"}
	track := &Track{Title: "Money", TrackNumber: 6, DiscNumber: 1}

	got := BuildTrackPath(artist, album, track, false, ".flac")
	want := filepath.FromSlash("Pink Floyd/The Dark Side of the Moon (1973)/06 - Money.flac")
	if got != want {
		t.Fatalf("BuildTrackPath = %q, want %q", got, want)
	}
}

func TestBuildTrackPath_MultiDisc(t *testing.T) {
	artist := &Artist{Name: "The Beatles"}
	album := &Album{Title: "The Beatles", ReleaseDate: "1968"}
	track := &Track{Title: "Helter Skelter", TrackNumber: 4, DiscNumber: 2}

	got := BuildTrackPath(artist, album, track, true, ".mp3")
	want := filepath.FromSlash("The Beatles/The Beatles (1968)/2-04 - Helter Skelter.mp3")
	if got != want {
		t.Fatalf("BuildTrackPath = %q, want %q", got, want)
	}
}

func TestBuildTrackPath_SanitizesIllegalChars(t *testing.T) {
	artist := &Artist{Name: "AC/DC"}
	album := &Album{Title: "Back in Black"}
	track := &Track{Title: "What Do You Do for Money: Honey?", TrackNumber: 2, DiscNumber: 1}

	got := BuildTrackPath(artist, album, track, false, ".flac")
	want := filepath.FromSlash("AC-DC/Back in Black/02 - What Do You Do for Money - Honey.flac")
	if got != want {
		t.Fatalf("BuildTrackPath = %q, want %q", got, want)
	}
}

func TestBuildTrackPath_MissingMetadataFallbacks(t *testing.T) {
	got := BuildTrackPath(nil, nil, nil, false, "")
	want := filepath.FromSlash("Unknown Artist/Unknown Album/00 - Unknown Track")
	if got != want {
		t.Fatalf("BuildTrackPath = %q, want %q", got, want)
	}
}

func TestBuildTrackPath_CompilationUsesVariousArtistsFolder(t *testing.T) {
	album := &Album{
		Title: "A Shared Record", ReleaseDate: "2024",
		Artists: AlbumArtistCredits{
			{Name: "First Artist"}, {Name: "Second Artist"},
		},
	}
	track := &Track{Title: "Opening Theme", TrackNumber: 1, DiscNumber: 1}
	got := BuildTrackPath(nil, album, track, false, ".flac")
	want := filepath.FromSlash("Various Artists/A Shared Record (2024)/01 - Opening Theme.flac")
	if got != want {
		t.Fatalf("BuildTrackPath = %q, want %q", got, want)
	}
}

func TestBuildTrackPath_StandaloneSingleArtistUsesCreditedName(t *testing.T) {
	album := &Album{
		Title:   "Solo Record",
		Artists: AlbumArtistCredits{{Name: "Guest Performer"}},
	}
	got := BuildTrackPath(nil, album, &Track{Title: "Song", TrackNumber: 1}, false, ".flac")
	want := filepath.FromSlash("Guest Performer/Solo Record/01 - Song.flac")
	if got != want {
		t.Fatalf("BuildTrackPath = %q, want %q", got, want)
	}
}

func TestAlbumYear(t *testing.T) {
	cases := map[string]string{
		"1973-03-01": "1973",
		"1968":       "1968",
		"":           "",
		"abc":        "",
	}
	for in, want := range cases {
		if got := albumYear(&Album{ReleaseDate: in}); got != want {
			t.Errorf("albumYear(%q) = %q, want %q", in, got, want)
		}
	}
}
