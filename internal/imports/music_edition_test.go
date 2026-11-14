package imports

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	musiclib "github.com/ebenderooock/loom/internal/music"
)

func writeEditionMusic(t *testing.T, path, album, title string, position int) {
	t.Helper()
	var frames []byte
	for _, field := range []struct{ id, value string }{
		{"TIT2", title}, {"TALB", album}, {"TPE1", "Linkin Park"},
		{"TPE2", "Linkin Park"}, {"TRCK", fmt.Sprintf("%d/15", position)},
	} {
		header := make([]byte, 10)
		copy(header, field.id)
		payload := append([]byte{0}, []byte(field.value)...)
		binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
		frames = append(frames, header...)
		frames = append(frames, payload...)
	}
	size := len(frames)
	data := append([]byte{'I', 'D', '3', 3, 0, 0, byte(size >> 21 & 127), byte(size >> 14 & 127), byte(size >> 7 & 127), byte(size & 127)}, frames...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMusicTourEditionImportKeepsMissingRecordingsUnmatched(t *testing.T) {
	titles := []string{
		"Wake", "Given Up", "Leave Out All the Rest", "Bleed it Out",
		"Shadow of the Day", "What I've Done", "Hands Held High", "No More Sorrow",
		"Valentine's Day", "In Between", "In Pieces", "The Little Things Give You Away",
		"No Roads Left (Bonus)", "What I've Done (Distorted Remix)", "Given Up (Third Encore Session)",
	}
	for _, taggedAlbum := range []string{"Minutes to Midnight", "Minutes to Midnight [Tour Edition]", "Minutes to Midnight (Tour Edition)"} {
		t.Run(taggedAlbum, func(t *testing.T) {
			ctx := context.Background()
			p, repo, album, _ := newMusicTransferPipeline(t, ImportModeCopy, true)
			artist, err := repo.GetArtist(ctx, album.ArtistID)
			if err != nil {
				t.Fatal(err)
			}
			artist.Name = "Linkin Park"
			if err := repo.UpdateArtist(ctx, artist); err != nil {
				t.Fatal(err)
			}
			album.Title = "Minutes to Midnight"
			album.Artists = musiclib.AlbumArtistCredits{{ArtistID: artist.ID}}
			if err := repo.UpdateAlbum(ctx, album); err != nil {
				t.Fatal(err)
			}
			tracks := make([]*musiclib.Track, 0, 12)
			for i, title := range titles[:12] {
				tracks = append(tracks, &musiclib.Track{
					ID: fmt.Sprintf("studio-%d", i), AlbumID: album.ID, Title: title,
					TrackNumber: i + 1, DiscNumber: 1,
				})
			}

			if err := repo.ReplaceTracks(ctx, album.ID, tracks); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for i, title := range titles {
				writeEditionMusic(t, filepath.Join(dir, fmt.Sprintf("%02d.mp3", i+1)), taggedAlbum, title, i+1)
			}
			paths, err := p.processImport(ctx, &downloads.DownloadCompletedEvent{
				DownloadID: "synthetic", Category: "music", Title: "Linkin Park - " + taggedAlbum,
			}, dir)
			if err == nil || !strings.Contains(err.Error(), "3 of 15 files failed") || len(paths) != 12 {
				t.Fatalf("want 12 imports and 3 explicit failures, got paths=%d error=%v", len(paths), err)
			}
			for _, title := range titles[12:] {
				if !strings.Contains(err.Error(), title) {
					t.Errorf("missing recording not reported: %q, error=%v", title, err)
				}
			}
			files, err := repo.ListTrackFilesByAlbum(ctx, album.ID)
			if err != nil || len(files) != 12 {
				t.Fatalf("wrong registered track files: %d, %v", len(files), err)
			}
			for i := range titles {
				if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%02d.mp3", i+1))); err != nil {
					t.Fatalf("synthetic source was consumed: %v", err)
				}
			}
		})
	}
}

func TestMusicEditionMatchingRejectsUnverifiedAlternativeAlbum(t *testing.T) {
	owner := &musiclib.Artist{ID: "artist", Name: "Linkin Park"}
	album := &musiclib.Album{ID: "album", ArtistID: owner.ID, Title: "Minutes to Midnight"}
	alternative := &musiclib.Album{ID: "alternative", ArtistID: owner.ID, Title: "Minutes to Midnight (Tour Edition)"}
	stub := &musicMatchServiceStub{
		artists: []*musiclib.Artist{owner}, albums: []*musiclib.Album{album, alternative},
		tracks: map[string]*musiclib.Album{
			album.ID: {ID: album.ID, Tracks: []*musiclib.Track{{ID: "studio", Title: "Wake"}}},
		},
	}
	p := &ImportPipeline{musicSvc: stub}
	tags := &musiclib.AudioTags{Title: "Wake", Artist: owner.Name}
	_, _, _, err := p.findMusicTrack(context.Background(), owner.Name, owner.Name, alternative.Title, tags)
	if err == nil || !strings.Contains(err.Error(), "ambiguous music album") || !errors.Is(err, musiclib.ErrNotFound) {
		t.Fatalf("unverified edition was ignored: %v", err)
	}
	stub.tracks[alternative.ID] = &musiclib.Album{ID: alternative.ID, Tracks: []*musiclib.Track{{ID: "tour", Title: "Wake"}}}
	_, _, _, err = p.findMusicTrack(context.Background(), owner.Name, owner.Name, alternative.Title, tags)
	if err == nil || !strings.Contains(err.Error(), "ambiguous music album") {
		t.Fatalf("duplicate album/edition recording accepted: %v", err)
	}
}

func TestMusicEditionMatchingRequiresUnambiguousRecordingIdentity(t *testing.T) {
	owner := &musiclib.Artist{ID: "artist", Name: "Linkin Park"}
	album := &musiclib.Album{ID: "album", ArtistID: owner.ID, Title: "Minutes to Midnight"}
	studio := &musiclib.Track{ID: "studio", Title: "Given Up", ArtistName: owner.Name, TrackNumber: 2}
	for _, tt := range []struct {
		name   string
		artist string
		album  string
		title  string
		tracks []*musiclib.Track
		want   string
	}{
		{"shifted position", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up", []*musiclib.Track{studio}, studio.ID},
		{"session missing", owner.Name, album.Title, "Given Up (Third Encore Session)", []*musiclib.Track{studio}, ""},
		{"remix missing", owner.Name, album.Title, "Given Up (Remix)", []*musiclib.Track{studio}, ""},
		{"remix exact", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up (Remix)", []*musiclib.Track{{ID: "remix", Title: "Given Up (Remix)", ArtistName: owner.Name}}, "remix"},
		{"credit does not erase remix", owner.Name, album.Title, "Given Up (Remix) (feat. Guest)", []*musiclib.Track{studio}, ""},
		{"studio credit", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up (feat. Guest)", []*musiclib.Track{studio}, studio.ID},
		{"bonus missing", owner.Name, album.Title, "Given Up (Bonus)", []*musiclib.Track{studio}, ""},
		{"studio missing", owner.Name, album.Title, "Given Up", []*musiclib.Track{{ID: "live", Title: "Given Up (Live)", ArtistName: owner.Name}}, ""},
		{"session exact", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up (Third Encore Session)", []*musiclib.Track{{ID: "session", Title: "Given Up (Third Encore Session)", ArtistName: owner.Name}}, "session"},
		{"wrong artist", "Other Artist", "Minutes to Midnight [Tour Edition]", "Given Up", []*musiclib.Track{studio}, ""},
		{"conflicting track artist", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up", []*musiclib.Track{{ID: "collision", Title: "Given Up", ArtistName: "Other Artist"}}, ""},
		{"owner establishes missing track artist", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up", []*musiclib.Track{{ID: "unknown", Title: "Given Up"}}, "unknown"},
		{"live album", owner.Name, "Minutes to Midnight (Live)", "Given Up", []*musiclib.Track{studio}, ""},
		{"missing title", owner.Name, "Minutes to Midnight [Tour Edition]", "", []*musiclib.Track{studio}, ""},
		{"missing metadata", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up", nil, ""},
		{"duplicate titles across editions", owner.Name, "Minutes to Midnight [Tour Edition]", "Given Up", []*musiclib.Track{studio, {ID: "other", Title: studio.Title, ArtistName: owner.Name, TrackNumber: 1}}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stub := &musicMatchServiceStub{
				artists: []*musiclib.Artist{owner}, albums: []*musiclib.Album{album},
				tracks: map[string]*musiclib.Album{album.ID: {ID: album.ID, Tracks: tt.tracks}},
			}
			p := &ImportPipeline{musicSvc: stub}
			_, _, track, err := p.findMusicTrack(context.Background(), tt.artist, tt.artist, tt.album, &musiclib.AudioTags{
				Artist: tt.artist, Title: tt.title, TrackNumber: 1,
			})
			if tt.want == "" {
				if track != nil || err == nil {
					t.Fatalf("unsafe match: track=%+v error=%v", track, err)
				}
			} else if err != nil || track == nil || track.ID != tt.want {
				t.Fatalf("want %q, got track=%+v error=%v", tt.want, track, err)
			}
		})
	}
}

func TestMusicEditionMatchingRejectsStaleSelectedReleaseTrack(t *testing.T) {
	owner := &musiclib.Artist{ID: "artist", Name: "Linkin Park"}
	album := &musiclib.Album{ID: "album", ArtistID: owner.ID, Title: "Minutes to Midnight"}
	full := &musiclib.Album{
		ID: album.ID, SelectedReleaseID: "selected",
		Tracks: []*musiclib.Track{{ID: "track", Title: "Wake", ReleaseID: "previous"}},
	}
	p := &ImportPipeline{musicSvc: &musicMatchServiceStub{
		artists: []*musiclib.Artist{owner}, albums: []*musiclib.Album{album},
		tracks: map[string]*musiclib.Album{album.ID: full},
	}}
	tags := &musiclib.AudioTags{Title: "Wake", Artist: owner.Name}
	_, _, track, err := p.findMusicTrack(context.Background(), owner.Name, owner.Name, "Minutes to Midnight [Tour Edition]", tags)
	if track != nil || err == nil {
		t.Fatalf("track from stale release accepted: track=%+v err=%v", track, err)
	}
	full.Tracks[0].ReleaseID = full.SelectedReleaseID
	_, _, track, err = p.findMusicTrack(context.Background(), owner.Name, owner.Name, "Minutes to Midnight [Tour Edition]", tags)
	if track == nil || err != nil {
		t.Fatalf("track from selected release rejected: track=%+v err=%v", track, err)
	}
}
