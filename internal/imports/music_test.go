package imports

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
