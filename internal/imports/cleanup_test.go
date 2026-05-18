package imports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsJunk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		file string
		want bool
	}{
		{"thumbs.db", "Thumbs.db", true},
		{"ds_store", ".DS_Store", true},
		{"desktop.ini", "desktop.ini", true},
		{"nfo file", "movie.nfo", true},
		{"txt file", "readme.txt", true},
		{"url file", "website.url", true},
		{"website file", "link.website", true},
		{"case insensitive", "THUMBS.DB", true},
		{"media file", "movie.mkv", false},
		{"subtitle", "movie.srt", false},
		{"image", "poster.jpg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsJunk(tt.file); got != tt.want {
				t.Errorf("IsJunk(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestCleanFolder_OnlyJunk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, f := range []string{"Thumbs.db", ".DS_Store", "info.nfo", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("junk"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	c := &FolderCleaner{}
	cleaned, err := c.CleanFolder(dir)
	if err != nil {
		t.Fatalf("CleanFolder failed: %v", err)
	}
	if !cleaned {
		t.Fatal("expected folder to be cleaned")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("folder should have been removed")
	}
}

func TestCleanFolder_WithRealFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Thumbs.db"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &FolderCleaner{}
	cleaned, err := c.CleanFolder(dir)
	if err != nil {
		t.Fatalf("CleanFolder failed: %v", err)
	}
	if cleaned {
		t.Fatal("folder should NOT be cleaned when real files exist")
	}
}

func TestCleanFolder_EmptyFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	c := &FolderCleaner{}
	cleaned, err := c.CleanFolder(dir)
	if err != nil {
		t.Fatalf("CleanFolder failed: %v", err)
	}
	if !cleaned {
		t.Fatal("empty folder should be cleaned")
	}
}

func TestCleanFolder_NestedJunkOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "subfolder")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &FolderCleaner{}
	cleaned, err := c.CleanFolder(dir)
	if err != nil {
		t.Fatalf("CleanFolder failed: %v", err)
	}
	if !cleaned {
		t.Fatal("folder with only nested junk should be cleaned")
	}
}
