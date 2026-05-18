package imports

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
)

func TestRecycle_PreservesPathStructure(t *testing.T) {
	t.Parallel()
	libRoot := t.TempDir()
	binDir := t.TempDir()

	// Create a file inside a library sub-folder.
	movieDir := filepath.Join(libRoot, "Movie (2024)")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	movieFile := filepath.Join(movieDir, "Movie.mkv")
	if err := os.WriteFile(movieFile, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &RecycleBin{Enabled: true, Path: binDir}
	if err := r.Recycle(movieFile, libRoot); err != nil {
		t.Fatalf("Recycle failed: %v", err)
	}

	// Original should be gone.
	if _, err := os.Stat(movieFile); !os.IsNotExist(err) {
		t.Fatal("original file should have been removed")
	}

	// File should exist under recycled path.
	libBase := filepath.Base(libRoot)
	recycled := filepath.Join(binDir, libBase, "Movie (2024)", "Movie.mkv")
	if _, err := os.Stat(recycled); err != nil {
		t.Fatalf("recycled file not found at %s: %v", recycled, err)
	}
}

func TestRecycle_DisabledDeletesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &RecycleBin{Enabled: false}
	if err := r.Recycle(f, dir); err != nil {
		t.Fatalf("Recycle (disabled) failed: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("file should have been deleted when recycle bin is disabled")
	}
}

func TestNewRecycleBin_FromConfig(t *testing.T) {
	t.Parallel()
	cfg := &appconfig.Config{
		RecycleBin: appconfig.RecycleBinConfig{
			Enabled:       true,
			Path:          "/some/path",
			RetentionDays: 30,
		},
	}
	rb := NewRecycleBin(cfg)
	if !rb.Enabled {
		t.Fatal("expected enabled")
	}
	if rb.Path != "/some/path" {
		t.Fatalf("expected path /some/path, got %s", rb.Path)
	}
}

func TestCleanOld_RemovesExpiredFiles(t *testing.T) {
	t.Parallel()
	binDir := t.TempDir()

	// Create an old file.
	oldFile := filepath.Join(binDir, "old.mkv")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Backdate modification time.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a recent file.
	newFile := filepath.Join(binDir, "new.mkv")
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &RecycleBin{Enabled: true, Path: binDir}
	if err := r.CleanOld(24 * time.Hour); err != nil {
		t.Fatalf("CleanOld failed: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old file should have been removed")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("new file should still exist")
	}
}
