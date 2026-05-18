package imports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSubtitles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a video file
	videoPath := filepath.Join(dir, "Movie.2024.mkv")
	os.WriteFile(videoPath, []byte("video"), 0o644)

	// Create matching subtitle files
	for _, name := range []string{
		"Movie.2024.srt",
		"Movie.2024.en.srt",
		"Movie.2024.eng.srt",
		"Movie.2024.en.forced.srt",
		"Movie.2024.eng.sdh.ass",
		"Movie.2024.en.cc.vtt",
	} {
		os.WriteFile(filepath.Join(dir, name), []byte("sub"), 0o644)
	}

	// Create non-matching files
	for _, name := range []string{
		"OtherMovie.srt",
		"Movie.2024.mkv.bak",
		"Movie.2024.txt.bak",
	} {
		os.WriteFile(filepath.Join(dir, name), []byte("other"), 0o644)
	}

	svc := &SubtitleService{}
	subs, err := svc.FindSubtitles(videoPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(subs) != 6 {
		t.Fatalf("expected 6 subtitles, got %d", len(subs))
	}

	// Build a map for easier assertions
	byPath := make(map[string]SubtitleFile)
	for _, s := range subs {
		byPath[filepath.Base(s.Path)] = s
	}

	// Plain .srt
	if s := byPath["Movie.2024.srt"]; s.Language != "" || s.IsForced || s.IsSDH {
		t.Errorf("plain srt: unexpected lang=%q forced=%v sdh=%v", s.Language, s.IsForced, s.IsSDH)
	}

	// .en.srt
	if s := byPath["Movie.2024.en.srt"]; s.Language != "en" {
		t.Errorf("en srt: expected lang=en, got %q", s.Language)
	}

	// .eng.srt
	if s := byPath["Movie.2024.eng.srt"]; s.Language != "eng" {
		t.Errorf("eng srt: expected lang=eng, got %q", s.Language)
	}

	// .en.forced.srt
	if s := byPath["Movie.2024.en.forced.srt"]; s.Language != "en" || !s.IsForced {
		t.Errorf("en forced srt: expected lang=en forced=true, got lang=%q forced=%v", s.Language, s.IsForced)
	}

	// .eng.sdh.ass
	if s := byPath["Movie.2024.eng.sdh.ass"]; s.Language != "eng" || !s.IsSDH {
		t.Errorf("eng sdh ass: expected lang=eng sdh=true, got lang=%q sdh=%v", s.Language, s.IsSDH)
	}

	// .en.cc.vtt
	if s := byPath["Movie.2024.en.cc.vtt"]; !s.IsSDH {
		t.Errorf("en cc vtt: expected sdh=true, got %v", s.IsSDH)
	}
}

func TestFindSubtitles_NoSubtitles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "Movie.mkv")
	os.WriteFile(videoPath, []byte("video"), 0o644)

	svc := &SubtitleService{}
	subs, err := svc.FindSubtitles(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 subtitles, got %d", len(subs))
	}
}

func TestExtractLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		suffix string
		want   string
	}{
		{"", ""},
		{".en", "en"},
		{".eng", "eng"},
		{".English", "English"},
		{".en.forced", "en"},
		{".eng.sdh", "eng"},
		{".forced", ""},
		{".sdh", ""},
		{".cc", ""},
	}
	for _, tc := range tests {
		got := extractLanguage(tc.suffix)
		if got != tc.want {
			t.Errorf("extractLanguage(%q) = %q, want %q", tc.suffix, got, tc.want)
		}
	}
}

func TestImportSubtitle(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	subPath := filepath.Join(srcDir, "Movie.2024.en.forced.srt")
	os.WriteFile(subPath, []byte("subtitle content"), 0o644)

	destVideo := filepath.Join(dstDir, "Movie (2024)", "Movie (2024).mkv")
	os.MkdirAll(filepath.Dir(destVideo), 0o755)
	os.WriteFile(destVideo, []byte("video"), 0o644)

	svc := &SubtitleService{}
	sub := SubtitleFile{
		Path:     subPath,
		Language: "en",
		IsForced: true,
		IsSDH:    false,
	}
	if err := svc.ImportSubtitle(sub, destVideo, ImportModeCopy); err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(dstDir, "Movie (2024)", "Movie (2024).en.forced.srt")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Fatalf("expected subtitle at %s", expected)
	}
}
