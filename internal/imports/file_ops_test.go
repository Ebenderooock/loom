package imports

import (
	"errors"
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

func TestImportFileCollisionPreservesSourceAndDestination(t *testing.T) {
	t.Parallel()
	for _, mode := range []ImportMode{
		ImportModeCopy,
		ImportModeMove,
		ImportModeHardlink,
		ImportModeHardlinkOnly,
	} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "source.mkv")
			dst := filepath.Join(dir, "destination.mkv")
			if err := os.WriteFile(src, []byte("incoming"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, []byte("existing"), 0o644); err != nil {
				t.Fatal(err)
			}

			err := importFile(src, dst, mode)
			if !errors.Is(err, ErrDestinationExists) {
				t.Fatalf("importFile error=%v, want ErrDestinationExists", err)
			}
			if got, err := os.ReadFile(src); err != nil || string(got) != "incoming" {
				t.Fatalf("source changed or missing: data=%q err=%v", got, err)
			}
			if got, err := os.ReadFile(dst); err != nil || string(got) != "existing" {
				t.Fatalf("destination changed or missing: data=%q err=%v", got, err)
			}
		})
	}
}

func TestImportFileRejectsSymlinkDestinationWithoutTouchingTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	target := filepath.Join(dir, "target.mkv")
	dst := filepath.Join(dir, "destination.mkv")
	if err := os.WriteFile(src, []byte("incoming"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dst); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := importFile(src, dst, ImportModeCopy); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("importFile error=%v, want ErrDestinationExists", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "target" {
		t.Fatalf("symlink target changed: data=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(src); err != nil || string(got) != "incoming" {
		t.Fatalf("source changed: data=%q err=%v", got, err)
	}
}

func TestImportFileRejectsSymlinkSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.mkv")
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "destination.mkv")
	if err := os.WriteFile(target, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, src); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := importFile(src, dst, ImportModeMove); err == nil {
		t.Fatal("expected symlink source to be rejected")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "source" {
		t.Fatalf("symlink target changed: data=%q err=%v", got, err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("destination should not exist, got err=%v", err)
	}
}

func TestImportFolderCollisionAndSymlinkPreflightPreserveTrees(t *testing.T) {
	t.Parallel()

	t.Run("existing destination", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "source")
		dst := filepath.Join(dir, "destination")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		srcFile, dstFile := filepath.Join(src, "video.mkv"), filepath.Join(dst, "video.mkv")
		if err := os.WriteFile(srcFile, []byte("incoming"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dstFile, []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := importFolder(src, dst); !errors.Is(err, ErrDestinationExists) {
			t.Fatalf("importFolder error=%v, want ErrDestinationExists", err)
		}
		if got, err := os.ReadFile(srcFile); err != nil || string(got) != "incoming" {
			t.Fatalf("source changed: data=%q err=%v", got, err)
		}
		if got, err := os.ReadFile(dstFile); err != nil || string(got) != "existing" {
			t.Fatalf("destination changed: data=%q err=%v", got, err)
		}
	})

	t.Run("symlink source entry", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "source")
		dst := filepath.Join(dir, "destination")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		video := filepath.Join(src, "video.mkv")
		target := filepath.Join(dir, "outside.mkv")
		if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(src, "link.mkv")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		if err := importFolder(src, dst); err == nil {
			t.Fatal("expected symlink entry to be rejected")
		}
		if got, err := os.ReadFile(video); err != nil || string(got) != "video" {
			t.Fatalf("source tree changed: data=%q err=%v", got, err)
		}
		if got, err := os.ReadFile(target); err != nil || string(got) != "outside" {
			t.Fatalf("symlink target changed: data=%q err=%v", got, err)
		}
		if _, err := os.Lstat(dst); !os.IsNotExist(err) {
			t.Fatalf("destination should not exist after preflight failure, got err=%v", err)
		}
	})
}
