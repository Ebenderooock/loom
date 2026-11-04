package imports

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	musiclib "github.com/ebenderooock/loom/internal/music"
)

type musicMatchServiceStub struct {
	musiclib.Service
	artists []*musiclib.Artist
	albums  []*musiclib.Album
	tracks  map[string]*musiclib.Album
}

func (s *musicMatchServiceStub) ListArtists(context.Context) ([]*musiclib.Artist, error) {
	return s.artists, nil
}

func (s *musicMatchServiceStub) ListAlbums(context.Context) ([]*musiclib.Album, error) {
	return s.albums, nil
}

func (s *musicMatchServiceStub) LoadAlbumTracks(_ context.Context, id string) (*musiclib.Album, error) {
	album := s.tracks[id]
	if album == nil {
		return nil, musiclib.ErrNotFound
	}
	return album, nil
}

func TestNormalizeMusicTextPreservesUnicodeLetters(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Beyoncé", want: "beyonce"},
		{input: "東京", want: "東京"},
	}
	for _, tt := range tests {
		if got := normalizeMusicText(tt.input); got != tt.want {
			t.Errorf("normalizeMusicText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestScanAudioFilesOnlyReturnsRegularAudioFiles(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "track.flac")
	videoPath := filepath.Join(dir, "video.mkv")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := scanAudioFiles(context.Background(), dir)
	if err != nil {
		t.Fatalf("scanAudioFiles: %v", err)
	}
	if len(files) != 1 || files[0] != audioPath {
		t.Fatalf("scanAudioFiles = %v, want [%s]", files, audioPath)
	}
}

func TestFindMusicTrackDoesNotUseUnrelatedSameTitleAlbum(t *testing.T) {
	owner := &musiclib.Artist{ID: "artist-1", Name: "Album Artist"}
	albums := []*musiclib.Album{
		{ID: "album-1", ArtistID: owner.ID, Title: "Shared Title", Artists: musiclib.AlbumArtistCredits{{Name: owner.Name}}},
		{ID: "album-2", ArtistID: "artist-2", Title: "Shared Title", Artists: musiclib.AlbumArtistCredits{{Name: "Other Artist"}}},
	}
	stub := &musicMatchServiceStub{
		artists: []*musiclib.Artist{owner, {ID: "artist-2", Name: "Other Artist"}},
		albums:  albums,
		tracks: map[string]*musiclib.Album{
			"album-1": {ID: "album-1", Tracks: []*musiclib.Track{{
				ID: "track-1", Title: "Album Artist Song", TrackNumber: 1, ArtistName: "Album Artist",
			}}},
			"album-2": {ID: "album-2", Tracks: []*musiclib.Track{{
				ID: "track-2", Title: "Other Song", TrackNumber: 1, ArtistName: "Other Artist",
			}}},
		},
	}
	pipeline := &ImportPipeline{musicSvc: stub}
	_, _, _, err := pipeline.findMusicTrack(context.Background(), "Unrelated Artist", "Unrelated Artist", "Shared Title", &musiclib.AudioTags{
		AlbumArtist: "Unrelated Artist", Artist: "Unrelated Artist", Album: "Shared Title",
		Title: "Album Artist Song", TrackNumber: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "credited artist") {
		t.Fatalf("unrelated same-title album was accepted, error=%v", err)
	}
}

func TestFindMusicTrackMatchesUnmanagedCompilationContributor(t *testing.T) {
	compilation := &musiclib.Album{
		ID: "compilation", Title: "Shared Soundtrack", AlbumType: "Compilation",
		Artists: musiclib.AlbumArtistCredits{
			{Name: "Guest Performer"}, {Name: "Another Guest"},
		},
	}
	stub := &musicMatchServiceStub{
		albums: []*musiclib.Album{compilation},
		tracks: map[string]*musiclib.Album{
			compilation.ID: {ID: compilation.ID, Tracks: []*musiclib.Track{{
				ID: "guest-song", Title: "Theme Song", TrackNumber: 1,
				ArtistName: "Guest Performer",
			}}},
		},
	}
	pipeline := &ImportPipeline{musicSvc: stub}
	artist, album, track, err := pipeline.findMusicTrack(context.Background(),
		"Various Artists", "Guest Performer", compilation.Title,
		&musiclib.AudioTags{
			AlbumArtist: "Various Artists", Artist: "Guest Performer",
			Album: compilation.Title, Title: "Theme Song", TrackNumber: 1,
		})
	if err != nil {
		t.Fatalf("findMusicTrack: %v", err)
	}
	if artist != nil || album.ID != compilation.ID || track.ID != "guest-song" {
		t.Fatalf("unexpected compilation match: artist=%+v album=%+v track=%+v", artist, album, track)
	}
	for _, tags := range []*musiclib.AudioTags{
		{AlbumArtist: "Various Artists", Artist: "Wrong Performer", Album: compilation.Title, Title: "Theme Song", TrackNumber: 1},
		{AlbumArtist: "Various Artists", Artist: "Guest Performer", Album: compilation.Title, Title: "Wrong Song", TrackNumber: 1},
	} {
		if _, _, _, err := pipeline.findMusicTrack(context.Background(), tags.AlbumArtist, tags.Artist, compilation.Title, tags); err == nil {
			t.Errorf("compilation track with mismatched artist/title was accepted: %+v", tags)
		}
	}
}

func TestFindMusicTrackRejectsAmbiguousCompilation(t *testing.T) {
	makeCompilation := func(id string) *musiclib.Album {
		return &musiclib.Album{ID: id, Title: "Duplicate Soundtrack", AlbumType: "Compilation"}
	}
	first, second := makeCompilation("comp-1"), makeCompilation("comp-2")
	stub := &musicMatchServiceStub{
		albums: []*musiclib.Album{first, second},
		tracks: map[string]*musiclib.Album{
			first.ID:  {ID: first.ID, Tracks: []*musiclib.Track{{ID: "track-1", Title: "Theme", TrackNumber: 1, ArtistName: "Guest"}}},
			second.ID: {ID: second.ID, Tracks: []*musiclib.Track{{ID: "track-2", Title: "Theme", TrackNumber: 1, ArtistName: "Guest"}}},
		},
	}
	pipeline := &ImportPipeline{musicSvc: stub}
	_, _, _, err := pipeline.findMusicTrack(context.Background(), "Various Artists", "Guest", first.Title, &musiclib.AudioTags{
		AlbumArtist: "Various Artists", Artist: "Guest", Album: first.Title, Title: "Theme", TrackNumber: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous music album") {
		t.Fatalf("expected explicit ambiguous album error, got %v", err)
	}
}

func TestMatchTaggedTrackDoesNotPreferPositionOverTitle(t *testing.T) {
	tracks := []*musiclib.Track{
		{ID: "position-match", Title: "Different Song", TrackNumber: 1, ArtistName: "Artist"},
		{ID: "title-match", Title: "Expected Song", TrackNumber: 2, ArtistName: "Artist"},
	}
	track, err := matchTaggedTrack(tracks, &musiclib.AudioTags{
		Artist: "Artist", Title: "Expected Song", TrackNumber: 1,
	}, false)
	if err != nil || track == nil || track.ID != "title-match" {
		t.Fatalf("matched track = %+v, error=%v; want title-match", track, err)
	}
}

func TestMatchTaggedTrackPreservesQualifierMatchingAndArtistIdentity(t *testing.T) {
	tracks := []*musiclib.Track{
		{ID: "qualified", Title: "Heavy (feat. Kiiara)", ArtistName: "Linkin Park"},
	}
	track, err := matchTaggedTrack(tracks, &musiclib.AudioTags{
		Artist: "Linkin Park", Title: "Heavy",
	}, true)
	if err != nil || track == nil || track.ID != "qualified" {
		t.Fatalf("qualified track = %+v, error=%v", track, err)
	}
	track, err = matchTaggedTrack(tracks, &musiclib.AudioTags{
		Artist: "Unrelated Artist", Title: "Heavy",
	}, true)
	if err != nil || track != nil {
		t.Fatalf("unrelated artist matched qualified track: track=%+v error=%v", track, err)
	}
}
