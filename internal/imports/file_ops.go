package imports

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyFile copies src to dst, preserving file permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
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
	return os.Remove(src)
}

// hardlinkFile creates a hard link from src to dst.
func hardlinkFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	return os.Link(src, dst)
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
