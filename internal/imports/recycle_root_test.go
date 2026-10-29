package imports

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecycleFromRoot(t *testing.T) {
	t.Parallel()
	library := t.TempDir()
	title := filepath.Join(library, "Title")
	if err := os.Mkdir(title, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(title)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	bin := &RecycleBin{Enabled: true, Path: t.TempDir()}
	for range 2 {
		if err := root.WriteFile("movie.mkv", []byte("movie"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := bin.RecycleFromRoot(root, "movie.mkv", library); err != nil {
			t.Fatal(err)
		}
		if _, err := root.Stat("movie.mkv"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source not removed: %v", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(bin.Path, filepath.Base(library), "Title"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("collision overwrote recycled file: %v %v", entries, err)
	}
}

func TestRecycleFromRootRejectsSymlinkEscapes(t *testing.T) {
	t.Parallel()
	library := t.TempDir()
	title := filepath.Join(library, "Title")
	if err := os.Mkdir(title, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(title)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	bin := &RecycleBin{Enabled: true, Path: t.TempDir()}
	outside := t.TempDir()
	precious := filepath.Join(outside, "movie.mkv")
	if err := os.WriteFile(precious, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(precious, filepath.Join(title, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := bin.RecycleFromRoot(root, "escape", library); err == nil {
		t.Fatal("source symlink escape accepted")
	}
	if err := root.WriteFile("movie.mkv", []byte("movie"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(bin.Path, filepath.Base(library))); err != nil {
		t.Fatal(err)
	}
	if err := bin.RecycleFromRoot(root, "movie.mkv", library); err == nil {
		t.Fatal("destination symlink escape accepted")
	}
	contents, err := os.ReadFile(precious)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("outside file changed: %q %v", contents, err)
	}
	if _, err := root.Stat("movie.mkv"); err != nil {
		t.Fatalf("failed recycle removed source: %v", err)
	}
}
