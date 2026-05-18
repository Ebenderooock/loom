package imports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	videoPath := filepath.Join(dir, "Movie.2024.mkv")
	os.WriteFile(videoPath, []byte("video"), 0o644)

	// Matching extras
	for _, name := range []string{
		"Movie.2024.nfo",
		"Movie.2024.jpg",
		"Movie.2024.png",
	} {
		os.WriteFile(filepath.Join(dir, name), []byte("extra"), 0o644)
	}

	// Non-matching files
	for _, name := range []string{
		"OtherMovie.nfo",
		"Movie.2024.srt", // subtitle, not extra
		"random.txt",
	} {
		os.WriteFile(filepath.Join(dir, name), []byte("other"), 0o644)
	}

	svc := &ExtraService{}
	extras, err := svc.FindExtras(videoPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(extras) != 3 {
		t.Fatalf("expected 3 extras, got %d: %v", len(extras), extras)
	}

	// Verify all are from the expected set
	names := make(map[string]bool)
	for _, e := range extras {
		names[filepath.Base(e)] = true
	}
	for _, want := range []string{"Movie.2024.nfo", "Movie.2024.jpg", "Movie.2024.png"} {
		if !names[want] {
			t.Errorf("expected extra %s not found", want)
		}
	}
}

func TestFindExtras_NoExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "Movie.mkv")
	os.WriteFile(videoPath, []byte("video"), 0o644)

	svc := &ExtraService{}
	extras, err := svc.FindExtras(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(extras) != 0 {
		t.Fatalf("expected 0 extras, got %d", len(extras))
	}
}

func TestImportExtra(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	extraPath := filepath.Join(srcDir, "Movie.2024.nfo")
	os.WriteFile(extraPath, []byte("nfo content"), 0o644)

	destVideo := filepath.Join(dstDir, "Movie (2024)", "Movie (2024).mkv")
	os.MkdirAll(filepath.Dir(destVideo), 0o755)
	os.WriteFile(destVideo, []byte("video"), 0o644)

	svc := &ExtraService{}
	if err := svc.ImportExtra(extraPath, destVideo, ImportModeCopy); err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(dstDir, "Movie (2024)", "Movie (2024).nfo")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Fatalf("expected extra at %s", expected)
	}
}
