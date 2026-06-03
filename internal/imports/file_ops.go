package imports

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyFile copies src to dst. The destination file is always created with
// 0o644 permissions so that retry attempts can overwrite it even when the
// source file is read-only.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	return out.Close()
}

// moveFile renames src to dst, falling back to copy+delete for
// cross-device moves.
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device fallback: copy then remove original.
	if err := copyFile(src, dst); err != nil {
		return err
	}
	// The copy succeeded, so the file is now in its destination. If removing the
	// source fails because it is already gone (e.g. a concurrent/duplicate import
	// removed it first), the move has effectively succeeded — do not report a
	// false failure that would mark the whole import as failed and trigger retries.
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// hardlinkFile creates a hard link from src to dst.
func hardlinkFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	return os.Link(src, dst)
}

// importFolder renames an entire source folder to the destination path,
// preserving all contents (subtitles, NFOs, images, etc.).
// Falls back to per-file move if rename fails (cross-device).
func importFolder(srcDir, destDir string) error {
	// If destination already exists, move contents into it
	if _, err := os.Stat(destDir); err == nil {
		return moveContents(srcDir, destDir)
	}

	// Ensure parent of destination exists
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("create dest parent: %w", err)
	}

	// Try direct rename (same filesystem)
	if err := os.Rename(srcDir, destDir); err == nil {
		return nil
	}

	// Cross-device fallback: create dest and move contents
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	return moveContents(srcDir, destDir)
}

// moveContents moves all files and subdirectories from src into dst.
func moveContents(srcDir, destDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read source dir: %w", err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(destDir, entry.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			// Cross-device: copy + delete
			if entry.IsDir() {
				if err := copyDir(srcPath, dstPath); err != nil {
					return err
				}
				_ = os.RemoveAll(srcPath)
			} else {
				if err := copyFile(srcPath, dstPath); err != nil {
					return err
				}
				_ = os.Remove(srcPath)
			}
		}
	}
	// Remove now-empty source dir
	_ = os.Remove(srcDir)
	return nil
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// importFile performs the file import using the specified mode.
// For "hardlink" mode, it falls back to copy on failure (cross-device).
// For "hardlink_only" mode, it fails if the hardlink cannot be created.
func importFile(src, dst string, mode ImportMode) error {
	switch mode {
	case ImportModeCopy:
		return copyFile(src, dst)
	case ImportModeHardlink:
		if err := hardlinkFile(src, dst); err == nil {
			return nil
		}
		return copyFile(src, dst)
	case ImportModeHardlinkOnly:
		if err := hardlinkFile(src, dst); err != nil {
			return fmt.Errorf("hardlink failed (hardlink_only mode): %w", err)
		}
		return nil
	default: // ImportModeMove
		return moveFile(src, dst)
	}
}

// mediaExtensions is the set of video file extensions we consider importable.
var mediaExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".wmv":  true,
	".ts":   true,
	".mov":  true,
	".flv":  true,
	".webm": true,
}

// scanMediaFiles walks dir and returns paths of video files.
func scanMediaFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if mediaExtensions[ext] {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
