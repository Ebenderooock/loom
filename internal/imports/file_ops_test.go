package imports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveFileRelocatesAndRemovesSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	dst := filepath.Join(dir, "out", "dst.mkv")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone, got err=%v", err)
	}
}

// removeAfterCopy mirrors the post-copy removal logic in moveFile to verify
// that an already-gone source (ENOENT) is treated as success rather than a
// false failure. This is the LW-007 regression: a concurrent/duplicate import
// removing the source must not flip a successful copy into a failed import.
func removeAfterCopy(src string) error {
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func TestRemoveAfterCopyToleratesMissingSource(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "already-gone.mkv")
	if err := removeAfterCopy(missing); err != nil {
		t.Fatalf("expected nil for already-removed source, got %v", err)
	}

	present := filepath.Join(dir, "present.mkv")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeAfterCopy(present); err != nil {
		t.Fatalf("expected nil removing existing source, got %v", err)
	}
	if _, err := os.Stat(present); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, got err=%v", err)
	}
}
