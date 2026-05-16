package imports

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
)

// RecycleBin moves replaced files to a configurable directory instead of
// deleting them outright. Files are stored with their relative library
// path preserved so they can be recovered if needed.
type RecycleBin struct {
	Enabled bool
	Path    string
}

// NewRecycleBin creates a RecycleBin from the application config.
// If the recycle bin section is disabled or the path is empty, the
// returned bin is a no-op.
func NewRecycleBin(cfg *appconfig.Config) *RecycleBin {
	return &RecycleBin{
		Enabled: cfg.RecycleBin.Enabled,
		Path:    cfg.RecycleBin.Path,
	}
}

// Recycle moves filePath into the recycle bin, preserving the relative
// path from libraryRoot.
//
// Example: libraryRoot="/movies", filePath="/movies/Film (2024)/Film.mkv"
// → recycled to <bin>/movies/Film (2024)/Film.mkv
func (r *RecycleBin) Recycle(filePath string, libraryRoot string) error {
	if !r.Enabled || r.Path == "" {
		// Disabled — fall back to plain deletion.
		return os.Remove(filePath)
	}

	rel, err := filepath.Rel(libraryRoot, filePath)
	if err != nil {
		// Can't compute relative path — use the base directory name + file.
		rel = filepath.Join(filepath.Base(libraryRoot), filepath.Base(filePath))
	}

	// Prefix with the library root's base name so files from different
	// libraries don't collide.
	dest := filepath.Join(r.Path, filepath.Base(libraryRoot), rel)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create recycle dir: %w", err)
	}

	// Avoid overwriting an already-recycled file by adding a timestamp.
	if _, err := os.Stat(dest); err == nil {
		ext := filepath.Ext(dest)
		base := strings.TrimSuffix(dest, ext)
		dest = fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
	}

	if err := os.Rename(filePath, dest); err != nil {
		// Cross-device fallback: copy + remove.
		if cpErr := copyFile(filePath, dest); cpErr != nil {
			return fmt.Errorf("recycle copy: %w", cpErr)
		}
		return os.Remove(filePath)
	}
	return nil
}

// CleanOld removes recycled files older than maxAge.
func (r *RecycleBin) CleanOld(maxAge time.Duration) error {
	if !r.Enabled || r.Path == "" {
		return nil
	}

	cutoff := time.Now().Add(-maxAge)

	return filepath.Walk(r.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("remove old recycled file %s: %w", path, removeErr)
			}
		}
		return nil
	})
}
