package imports

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrDestinationExists is returned rather than overwriting an existing file
// or folder. Import conflict handling must be explicit at a higher layer.
var ErrDestinationExists = errors.New("import destination already exists")

func checkDestinationAbsent(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrDestinationExists, path)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("check destination %q: %w", path, err)
	}
	return nil
}

func checkRegularSource(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect source %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink source %q", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", path)
	}
	return nil
}

// copyFile creates a new regular destination file. O_EXCL prevents a racing
// import or a symlink at dst from being followed and truncated.
func copyFile(src, dst string) (retErr error) {
	if err := checkRegularSource(src); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	if err := checkDestinationAbsent(dst); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) //nolint:gosec // Import destinations are library paths; O_EXCL rejects existing files and symlinks.
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", ErrDestinationExists, dst)
		}
		return fmt.Errorf("create dest: %w", err)
	}
	defer func() {
		if closeErr := out.Close(); retErr == nil && closeErr != nil {
			retErr = fmt.Errorf("close destination: %w", closeErr)
		}
		if retErr != nil {
			_ = os.Remove(dst)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync destination: %w", err)
	}
	return nil
}

// moveFile first tries an exclusive hard-link creation, which is atomic and
// cannot replace dst. Cross-device moves fall back to exclusive copy+remove.
func moveFile(src, dst string) error {
	if err := checkRegularSource(src); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	if err := checkDestinationAbsent(dst); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
			_ = os.Remove(dst)
			return fmt.Errorf("remove source after link: %w", err)
		}
		return nil
	} else if os.IsExist(err) {
		return fmt.Errorf("%w: %s", ErrDestinationExists, dst)
	}

	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(dst)
		return fmt.Errorf("remove source after copy: %w", err)
	}
	return nil
}

// hardlinkFile creates a new hard link and never replaces an existing target.
func hardlinkFile(src, dst string) error {
	if err := checkRegularSource(src); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	if err := checkDestinationAbsent(dst); err != nil {
		return err
	}
	if err := os.Link(src, dst); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", ErrDestinationExists, dst)
		}
		return err
	}
	return nil
}

// importFolder moves an entire source folder to an absent destination. It
// rejects existing destinations rather than merging contents, so repeated
// imports cannot silently replace or mix library data.
func importFolder(srcDir, destDir string) error {
	info, err := os.Lstat(srcDir)
	if err != nil {
		return fmt.Errorf("inspect source folder: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("source is not a real directory: %s", srcDir)
	}
	if filepath.Clean(srcDir) == filepath.Clean(destDir) {
		return nil
	}
	srcAbs, srcErr := filepath.Abs(srcDir)
	destAbs, destErr := filepath.Abs(destDir)
	if srcErr == nil && destErr == nil {
		rel, err := filepath.Rel(srcAbs, destAbs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("destination folder must not be inside source folder")
		}
		rel, err = filepath.Rel(destAbs, srcAbs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("source folder must not be inside destination folder")
		}
	}
	if err := validateImportTree(srcDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("create dest parent: %w", err)
	}
	if err := checkDestinationAbsent(destDir); err != nil {
		return err
	}
	if err := os.Mkdir(destDir, 0o755); err != nil { //nolint:gosec // Media folders must remain readable by external media servers.
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", ErrDestinationExists, destDir)
		}
		return fmt.Errorf("create dest dir: %w", err)
	}
	if err := moveContents(srcDir, destDir); err != nil {
		return fmt.Errorf("move folder contents: %w", err)
	}
	return nil
}

func validateImportTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("inspect source entry %q: %w", path, err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect source entry %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in import folder: %s", path)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported source entry type: %s", path)
		}
		return nil
	})
}

// moveContents moves entries individually. Each file operation is exclusive;
// source symlinks and non-regular files are rejected instead of being followed.
func moveContents(srcDir, destDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read source dir: %w", err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(destDir, entry.Name())
		info, err := os.Lstat(srcPath)
		if err != nil {
			return fmt.Errorf("inspect source entry %q: %w", srcPath, err)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("refusing symlink in import folder: %s", srcPath)
		case info.IsDir():
			if err := importFolder(srcPath, dstPath); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := moveFile(srcPath, dstPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported source entry type: %s", srcPath)
		}
	}
	if err := os.Remove(srcDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove empty source folder: %w", err)
	}
	return nil
}

// importFile performs the file import using the specified mode.
func importFile(src, dst string, mode ImportMode) error {
	switch mode {
	case ImportModeCopy:
		return copyFile(src, dst)
	case ImportModeHardlink:
		if err := hardlinkFile(src, dst); err == nil {
			return nil
		} else if errors.Is(err, ErrDestinationExists) {
			return err
		}
		return copyFile(src, dst)
	case ImportModeHardlinkOnly:
		if err := hardlinkFile(src, dst); err != nil {
			return fmt.Errorf("hardlink failed (hardlink_only mode): %w", err)
		}
		return nil
	default:
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
			return fmt.Errorf("inspect media path %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if mediaExtensions[ext] {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
