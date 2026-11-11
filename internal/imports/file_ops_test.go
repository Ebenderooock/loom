package imports

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type copyReaderFunc func([]byte) (int, error)

func (f copyReaderFunc) Read(p []byte) (int, error) { return f(p) }

func TestCopyPublishesOnlyCompleteFilesAndPreservesRacingDestination(t *testing.T) {
	for _, scenario := range []string{"success", "read error", "short source", "growing source", "racing file", "racing symlink"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			dst := filepath.Join(dir, "episode.mkv")
			readErr := errors.New("connection lost")
			first := true
			reader := copyReaderFunc(func(p []byte) (int, error) {
				if _, err := os.Lstat(dst); !os.IsNotExist(err) {
					t.Fatalf("destination exposed before EOF: %v", err)
				}
				staged, err := filepath.Glob(filepath.Join(dir, ".loom-import-*.partial"))
				if err != nil || len(staged) != 1 {
					t.Fatalf("expected one staged file: %v, %v", staged, err)
				}
				if first {
					first = false
					return copy(p, "partial"), nil
				}
				switch scenario {
				case "read error":
					return 0, readErr
				case "racing file":
					if err := os.WriteFile(dst, []byte("existing"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "racing symlink":
					target := filepath.Join(dir, "target")
					if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, dst); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				}
				return 0, io.EOF
			})
			size := int64(len("partial"))
			switch scenario {
			case "short source":
				size++
			case "growing source":
				size--
			}
			err := copyFileData(reader, dst, size)
			switch scenario {
			case "success":
				if err != nil {
					t.Fatal(err)
				}
				if got, err := os.ReadFile(dst); err != nil || string(got) != "partial" {
					t.Fatalf("incomplete destination: %q, %v", got, err)
				}
			case "racing file", "racing symlink":
				if !errors.Is(err, ErrDestinationExists) {
					t.Fatalf("want collision error, got %v", err)
				}
				if got, err := os.ReadFile(dst); err != nil || string(got) != "existing" {
					t.Fatalf("racing destination changed: %q, %v", got, err)
				}
			default:
				if err == nil || (scenario == "read error" && !errors.Is(err, readErr)) {
					t.Fatalf("copy failure not surfaced: %v", err)
				}
				if _, err := os.Lstat(dst); !os.IsNotExist(err) {
					t.Fatalf("failed copy published destination: %v", err)
				}
			}
			staged, err := filepath.Glob(filepath.Join(dir, ".loom-import-*.partial"))
			if err != nil || len(staged) != 0 {
				t.Fatalf("staging files leaked: %v, %v", staged, err)
			}
		})
	}
}

func TestCopyCrashDoesNotLeavePartialDestinationOrBlockRetry(t *testing.T) {
	if dir := os.Getenv("LOOM_TEST_COPY_CRASH"); dir != "" {
		reader := io.MultiReader(strings.NewReader("partial"), copyReaderFunc(func([]byte) (int, error) {
			os.Exit(73)
			return 0, nil
		}))
		_ = copyFileData(reader, filepath.Join(dir, "episode.mkv"), 14)
		os.Exit(74)
	}
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "source.mkv"), filepath.Join(dir, "episode.mkv")
	if err := os.WriteFile(src, []byte("complete video"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestCopyCrashDoesNotLeavePartialDestinationOrBlockRetry$")
	cmd.Env = append(os.Environ(), "LOOM_TEST_COPY_CRASH="+dir)
	err = cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 73 {
		t.Fatalf("child did not crash during copy: %v", err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("crash exposed partial destination: %v", err)
	}
	staged, err := filepath.Glob(filepath.Join(dir, ".loom-import-*.partial"))
	if err != nil || len(staged) != 1 {
		t.Fatalf("expected abandoned staging file: %v, %v", staged, err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("abandoned staging file blocked retry: %v", err)
	}
	for _, path := range []string{src, dst} {
		if got, err := os.ReadFile(path); err != nil || string(got) != "complete video" {
			t.Fatalf("source or destination incomplete: %q, %v", got, err)
		}
	}
}

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
