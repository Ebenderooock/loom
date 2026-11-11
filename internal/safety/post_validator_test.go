package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostValidatorAcceptsAudioWithoutMovieSizeMinimum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.flac")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewPostValidator(DefaultConfig()).ValidateDownload(dir)
	if err != nil || !result.Pass {
		t.Fatalf("audio incorrectly rejected as a small/missing movie: %+v, %v", result, err)
	}
}

func TestPostValidatorRejectsExecutablesBesideAudio(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"track.mp3", "installer.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("contents"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewPostValidator(DefaultConfig()).ValidateDownload(dir)
	if err != nil || result.Pass || !strings.Contains(strings.Join(result.Reasons, "; "), "executable") {
		t.Fatalf("audio bypassed executable validation: %+v, %v", result, err)
	}
}

func TestPostValidatorStillRejectsSmallVideos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("small video"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewPostValidator(DefaultConfig()).ValidateDownload(path)
	if err != nil || result.Pass || !strings.Contains(strings.Join(result.Reasons, "; "), "suspiciously small") {
		t.Fatalf("movie size validation changed: %+v, %v", result, err)
	}
}
