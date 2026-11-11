package imports

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	musiclib "github.com/ebenderooock/loom/internal/music"
)

func writeTaggedFLAC(t *testing.T, path, artist, album, title string) {
	t.Helper()
	// STREAMINFO plus Vorbis comments exercises real FLAC tag and bit-depth
	// parsing without an encoder or network metadata service.
	data := make([]byte, 42)
	copy(data, "fLaC")
	data[7] = 34
	binary.BigEndian.PutUint64(data[18:26], uint64(96000)<<44|uint64(1)<<41|uint64(23)<<36)
	comments := []string{
		"ARTIST=" + artist, "ALBUMARTIST=" + artist, "ALBUM=" + album,
		"TITLE=" + title, "TRACKNUMBER=1", "DISCNUMBER=1",
	}
	var block []byte
	appendInt := func(n int) {
		block = binary.LittleEndian.AppendUint32(block, uint32(n))
	}
	appendInt(0) // vendor length
	appendInt(len(comments))
	for _, comment := range comments {
		appendInt(len(comment))
		block = append(block, comment...)
	}
	data = append(data, 0x84, byte(len(block)>>16), byte(len(block)>>8), byte(len(block)))
	data = append(data, block...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMusicDeluxeHighResAppearsInAsyncScanAndQueuedImport(t *testing.T) {
	ctx := context.Background()
	p, repo, album, _ := newMusicTransferPipeline(t, ImportModeCopy, true)
	t.Cleanup(p.Stop)
	source := filepath.Join(t.TempDir(), "Test Artist - Test Album [24bit-96kHz]", "01.FLAC")
	writeTaggedFLAC(t, source, "Test Artist", "Test Album (Deluxe Edition)", "Test Song")

	scan, err := p.StartFolderScan(ctx, filepath.Dir(source))
	if err != nil {
		t.Fatal(err)
	}
	scan = awaitFolderScan(t, p, scan.ID)
	if scan.Status != "completed" || scan.TotalFiles != 1 || scan.ProcessedFiles != 1 || len(scan.Results) != 1 {
		t.Fatalf("audio absent from paged scan: %+v", scan)
	}
	result := scan.Results[0]
	if result.MediaType != "music" || result.MatchedMediaID != "track" ||
		result.SuggestedAction != "import" || result.ImportError != "" || result.Quality != "FLAC" {
		t.Fatalf("deluxe FLAC not matched in scan: %+v", result)
	}
	job, err := p.EnqueueImport(ctx, JobRequest{Path: filepath.Dir(source)})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := p.claimJob(ctx)
	if err != nil || claimed == nil || claimed.ID != job.ID {
		t.Fatalf("claim music job: %+v, %v", claimed, err)
	}
	p.runJob(ctx, claimed)
	job, err = p.GetJob(ctx, job.ID, true)
	if err != nil || job.Status != JobImported || job.ImportedFiles != 1 {
		t.Fatalf("music queue failed: %+v, %v", job, err)
	}
	files, err := repo.ListTrackFilesByAlbum(ctx, album.ID)
	if err != nil || len(files) != 1 || files[0].TrackID != "track" ||
		files[0].Format != "FLAC" || files[0].MediaInfo["bits"] != float64(24) {
		t.Fatalf("high-res music registration: %+v, %v", files, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("copy import consumed source: %v", err)
	}
}

func TestMusicScanSurfacesMetadataAndTagFailuresWithoutHidingAudio(t *testing.T) {
	ctx := context.Background()
	p, _, album, _ := newMusicTransferPipeline(t, ImportModeCopy, true)
	t.Cleanup(p.Stop)
	dir := t.TempDir()
	source := filepath.Join(dir, "01.FLAC")
	writeTaggedFLAC(t, source, "Test Artist", "Test Album", "Test Song")
	broken := filepath.Join(dir, "02.mp3")
	if err := os.WriteFile(broken, []byte("not audio metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.musicSvc = &musicMatchServiceStub{
		artists: []*musiclib.Artist{{ID: "artist", Name: "Test Artist"}},
		albums:  []*musiclib.Album{album},
		loadErr: errors.Join(musiclib.ErrAlbumTracksUnavailable, errors.New("metadata service unavailable")),
	}
	for _, async := range []bool{false, true} {
		var results []ScanResult
		if async {
			scan, err := p.StartFolderScan(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			results = awaitFolderScan(t, p, scan.ID).Results
		} else {
			var err error
			results, err = p.ScanFolder(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(results) != 2 {
			t.Fatalf("failed matches hid audio files: %+v", results)
		}
		for _, result := range results {
			if result.MediaType != "music" || result.SuggestedAction != "manual_review" || result.ImportError == "" {
				t.Fatalf("music failure not surfaced: %+v", result)
			}
			if result.FilePath == source && !strings.Contains(result.ImportError, "album track list unavailable") {
				t.Fatalf("metadata failure misreported as unmatched track: %+v", result)
			}
			if result.FilePath == broken && !strings.Contains(result.ImportError, "read audio tags") {
				t.Fatalf("tag failure hidden: %+v", result)
			}
		}
	}
}
