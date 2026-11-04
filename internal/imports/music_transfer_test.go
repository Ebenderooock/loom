package imports

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/libraries"
	musiclib "github.com/ebenderooock/loom/internal/music"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/storage"
)

func newMusicTransferPipeline(t *testing.T, mode ImportMode, managed bool) (*ImportPipeline, musiclib.Repository, *musiclib.Album, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "imports.db")},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	libs := libraries.NewStore(db.DB())
	root := t.TempDir()
	for _, lib := range []*libraries.Library{
		{ID: "music-library", Name: "Music", Path: root, MediaType: "music"},
		{ID: "other-library", Name: "Other", Path: t.TempDir(), MediaType: "music"},
	} {
		if err := libs.Create(ctx, lib); err != nil {
			t.Fatal(err)
		}
	}
	repo := musiclib.NewRepository(db.DB())
	album := &musiclib.Album{
		ID: "album", Title: "Test Album", ReleaseDate: "2026-01-01",
		LibraryID: "music-library", Artists: musiclib.AlbumArtistCredits{{Name: "Test Artist"}},
	}
	if managed {
		if err := repo.CreateArtist(ctx, &musiclib.Artist{
			ID: "artist", Name: "Test Artist", LibraryID: "music-library",
		}); err != nil {
			t.Fatal(err)
		}
		album.ArtistID = "artist"
		// Owner settings, not stale album settings, determine the library.
		album.LibraryID = "other-library"
	}
	if err := repo.CreateAlbum(ctx, album); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceTracks(ctx, album.ID, []*musiclib.Track{{
		ID: "track", AlbumID: album.ID, Title: "Test Song",
		TrackNumber: 1, DiscNumber: 1, ArtistName: "Test Artist",
	}}); err != nil {
		t.Fatal(err)
	}
	p, err := NewPipeline(PipelineOptions{
		DB: db.DB(), Bus: eventbus.NewInProc(), DownloadSvc: newDownloadsSvcForPipelineTest(t),
		MoviesSvc: &moviesSvcStub{}, SeriesSvc: &seriesSvcStub{},
		MusicSvc: musiclib.NewService(repo, nil, nil), LibStore: libs, ImportMode: mode,
		PostVal: safety.NewPostValidator(safety.DefaultConfig()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, repo, album, root
}

func writeTaggedMusic(t *testing.T, path, title string) []byte {
	t.Helper()
	// ID3v2.3 text frames exercise the actual tag reader without external tools.
	var frames []byte
	for _, field := range []struct{ id, value string }{
		{"TIT2", title}, {"TALB", "Test Album"}, {"TPE1", "Test Artist"},
		{"TPE2", "Test Artist"}, {"TRCK", "1"},
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMusicManualScanAndTransfer(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, mode := range []ImportMode{ImportModeMove, ImportModeCopy, ImportModeHardlink, ImportModeHardlinkOnly} {
			t.Run(string(mode)+"/managed="+map[bool]string{true: "yes", false: "no"}[managed], func(t *testing.T) {
				p, repo, album, root := newMusicTransferPipeline(t, mode, managed)
				source := filepath.Join(t.TempDir(), "download.mp3")
				content := writeTaggedMusic(t, source, "Test Song")
				results, err := p.ScanFolder(context.Background(), filepath.Dir(source))
				if err != nil || len(results) != 1 {
					t.Fatalf("scan: %+v, %v", results, err)
				}
				if results[0].MediaType != "music" || results[0].MatchedMediaID != "track" ||
					results[0].DetectedAlbum != album.Title || results[0].SuggestedAction != "import" {
					t.Fatalf("music preview did not match track: %+v", results[0])
				}
				if _, err := os.Stat(source); err != nil {
					t.Fatalf("scan consumed source: %v", err)
				}
				if err := p.ImportManual(context.Background(), filepath.Dir(source)); err != nil {
					t.Fatal(err)
				}
				dest := filepath.Join(root, "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
				if got, err := os.ReadFile(dest); err != nil || string(got) != string(content) {
					t.Fatalf("wrong library destination or contents: %q, %v", got, err)
				}
				_, err = os.Stat(source)
				if mode == ImportModeMove && !os.IsNotExist(err) {
					t.Fatalf("move left source behind: %v", err)
				}
				if mode != ImportModeMove && err != nil {
					t.Fatalf("%s consumed source: %v", mode, err)
				}
				files, err := repo.ListTrackFilesByAlbum(context.Background(), album.ID)
				if err != nil || len(files) != 1 || files[0].TrackID != "track" || files[0].FilePath != dest {
					t.Fatalf("music record not linked to destination: %+v, %v", files, err)
				}
				track, err := repo.GetTrack(context.Background(), "track")
				if err != nil || !track.HasFile {
					t.Fatalf("track availability not updated: %+v, %v", track, err)
				}
				index, err := p.libStore.ListFiles(context.Background(), "music-library")
				if err != nil || len(index) != 1 || index[0].Path != dest {
					t.Fatalf("library index not updated: %+v, %v", index, err)
				}
				if mode != ImportModeMove {
					if err := p.ImportManual(context.Background(), source); err != nil {
						t.Fatalf("retry was not idempotent: %v", err)
					}
				}
			})
		}
	}
}

func TestMusicManualMatchAndReimportUseSelectedTrack(t *testing.T) {
	for _, reimport := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual-match", true: "reimport"}[reimport], func(t *testing.T) {
			p, _, _, root := newMusicTransferPipeline(t, ImportModeMove, false)
			source := filepath.Join(t.TempDir(), "unrecognized.mp3")
			writeTaggedMusic(t, source, "Unrecognized Title")
			var record *ImportRecord
			var err error
			if reimport {
				record, err = p.ReimportFile(context.Background(), MediaTypeMusic, "track", source, ReimportOptions{})
			} else {
				record, err = p.ImportManualMatch(context.Background(), source, MediaTypeMusic, "track")
			}
			dest := filepath.Join(root, "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
			if err != nil || record == nil || record.DestPath != dest {
				t.Fatalf("explicit music match: %+v, %v", record, err)
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatalf("music move did not remove source: %v", err)
			}
		})
	}
}

func TestMusicDownloadPartialFailureAndFinalization(t *testing.T) {
	p, _, _, root := newMusicTransferPipeline(t, ImportModeMove, false)
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "01.mp3")
	writeTaggedMusic(t, source, "Test Song")
	unmatched := filepath.Join(sourceDir, "02.mp3")
	writeTaggedMusic(t, unmatched, "Unmatched Song")
	ev := &downloads.DownloadCompletedEvent{DownloadID: "download", Category: "music", Title: "Test Artist - Test Album"}
	paths, err := p.processImport(context.Background(), ev, sourceDir)
	if err == nil || !strings.Contains(err.Error(), "incomplete") || len(paths) != 1 {
		t.Fatalf("partial import incorrectly succeeded: %v, %v", paths, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source consumed before workflow finalization: %v", err)
	}
	want := filepath.Join(root, "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
	if paths[0] != want {
		t.Fatalf("wrong library path: %v", paths)
	}
	if err := p.FinalizeImports(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("finalization did not remove imported source: %v", err)
	}
	if _, err := os.Stat(unmatched); err != nil {
		t.Fatalf("failed track was removed: %v", err)
	}
}

func TestMusicSingleNonAudioDownloadDoesNotImportSiblings(t *testing.T) {
	p, _, _, _ := newMusicTransferPipeline(t, ImportModeMove, false)
	dir := t.TempDir()
	source := filepath.Join(dir, "track.mp3")
	writeTaggedMusic(t, source, "Test Song")
	other := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(other, []byte("artwork"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.processImport(context.Background(), &downloads.DownloadCompletedEvent{
		Category: "music",
	}, other); err == nil {
		t.Fatal("non-audio path imported neighboring audio")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("sibling audio was consumed: %v", err)
	}
}

func TestMusicDestinationCollisionPreservesBothFiles(t *testing.T) {
	p, _, _, root := newMusicTransferPipeline(t, ImportModeMove, false)
	source := filepath.Join(t.TempDir(), "track.mp3")
	content := writeTaggedMusic(t, source, "Test Song")
	dest := filepath.Join(root, "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.ImportManual(context.Background(), source); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected explicit collision: %v", err)
	}
	for path, want := range map[string]string{source: string(content), dest: "existing"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("collision changed %s: %v", path, err)
		}
	}

}

func TestMusicSamePathUpgradeKeepsReplacementRegistered(t *testing.T) {
	ctx := context.Background()
	p, repo, album, root := newMusicTransferPipeline(t, ImportModeMove, false)
	dest := filepath.Join(root, "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTrackFile(ctx, &musiclib.TrackFile{
		ID: "old-file", AlbumID: album.ID, TrackID: "track",
		FilePath: dest, Format: "MP3", Bitrate: 128,
	}); err != nil {
		t.Fatal(err)
	}
	p.recycler = &RecycleBin{Enabled: true, Path: t.TempDir()}
	full, err := p.musicSvc.LoadAlbumTracks(ctx, album.ID)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "upgrade.mp3")
	content := writeTaggedMusic(t, source, "Test Song")
	tags, err := musiclib.ReadAudioTags(source)
	if err != nil {
		t.Fatal(err)
	}
	tags.Bitrate = 320
	if _, err := p.importMatchedMusicFile(ctx, &downloads.DownloadCompletedEvent{
		Title: "Test Album",
	}, source, nil, full, full.Tracks[0], tags); err != nil {
		t.Fatal(err)
	}
	files, err := repo.ListTrackFilesByAlbum(ctx, album.ID)
	if err != nil || len(files) != 1 || files[0].ID == "old-file" || files[0].Bitrate != 320 {
		t.Fatalf("replacement record lost or stale: %+v, %v", files, err)
	}
	track, err := repo.GetTrack(ctx, "track")
	if err != nil || !track.HasFile {
		t.Fatalf("replacement track marked missing: %+v, %v", track, err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != string(content) {
		t.Fatalf("replacement file missing: %q, %v", got, err)
	}
	recycled := filepath.Join(p.recycler.Path, filepath.Base(root), "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
	if got, err := os.ReadFile(recycled); err != nil || string(got) != "old audio" {
		t.Fatalf("original audio was not preserved: %q, %v", got, err)
	}
}

func TestMusicOwnerWithoutLibraryDoesNotUseStaleAlbumLibrary(t *testing.T) {
	ctx := context.Background()
	p, repo, _, _ := newMusicTransferPipeline(t, ImportModeMove, true)
	owner, err := repo.GetArtist(ctx, "artist")
	if err != nil {
		t.Fatal(err)
	}
	owner.LibraryID = ""
	if err := repo.UpdateArtist(ctx, owner); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "track.mp3")
	writeTaggedMusic(t, source, "Test Song")
	if err := p.ImportManual(ctx, source); err == nil || !strings.Contains(err.Error(), "no configured library") {
		t.Fatalf("used a stale album library after owner library was cleared: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("unassigned library consumed source: %v", err)
	}
}

func TestMusicRunImportUsesDownloadContentPath(t *testing.T) {
	ctx := context.Background()
	p, _, _, root := newMusicTransferPipeline(t, ImportModeMove, false)
	source := filepath.Join(t.TempDir(), "download.mp3")
	content := writeTaggedMusic(t, source, "Test Song")
	if err := p.downloadSvc.Registry().Register(&resolvePathFakeClient{
		id: "client", items: []downloads.Item{{ID: "download", ContentPath: source}},
	}); err != nil {
		t.Fatal(err)
	}
	// A custom category must not send an audio file down the video matcher.
	paths, err := p.RunImport(ctx, "client", "download", "Test Artist - Test Album", "my-music")
	if err != nil || len(paths) != 1 {
		t.Fatalf("download import: %v, %v", paths, err)
	}
	want := filepath.Join(root, "Test Artist", "Test Album (2026)", "01 - Test Song.mp3")
	if paths[0] != want {
		t.Fatalf("download not imported into configured music library: %v", paths)
	}
	if got, err := os.ReadFile(want); err != nil || string(got) != string(content) {
		t.Fatalf("download media not transferred: %q, %v", got, err)
	}
	if err := p.FinalizeImports(ctx, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("download source not removed after finalization: %v", err)
	}
}
