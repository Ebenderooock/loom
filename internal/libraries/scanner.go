package libraries

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"context"
)

// mediaExtensions are common video file extensions to discover.
var mediaExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".m4v":  true,
	".ts":   true,
	".webm": true,
	".mpg":  true,
	".mpeg": true,
	".ogv":  true,
}

// Scanner discovers media files in library root folders.
type Scanner struct {
	store  *Store
	logger *slog.Logger
}

// NewScanner creates a library scanner.
func NewScanner(store *Store, logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{store: store, logger: logger}
}

// ScanLibrary walks the root folder of a library and upserts discovered media files.
func (sc *Scanner) ScanLibrary(ctx context.Context, lib *Library) error {
	sc.logger.Info("scanning library", "id", lib.ID, "path", lib.Path)
	scanStart := time.Now().UTC()

	err := filepath.WalkDir(lib.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			sc.logger.Warn("walk error", "path", path, "err", err)
			return nil // skip inaccessible entries
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			// Skip hidden directories.
			if strings.HasPrefix(d.Name(), ".") && path != lib.Path {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !mediaExtensions[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			sc.logger.Warn("stat error", "path", path, "err", err)
			return nil
		}

		f := &LibraryFile{
			LibraryID: lib.ID,
			Path:      path,
			SizeBytes: info.Size(),
		}
		if err := sc.store.UpsertFile(ctx, f); err != nil {
			sc.logger.Error("upsert file", "path", path, "err", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", lib.Path, err)
	}

	// Clean up files that no longer exist on disk.
	removed, err := sc.store.DeleteStaleFiles(ctx, lib.ID, scanStart)
	if err != nil {
		sc.logger.Error("delete stale files", "library", lib.ID, "err", err)
	} else if removed > 0 {
		sc.logger.Info("removed stale files", "library", lib.ID, "count", removed)
	}

	sc.logger.Info("scan complete", "id", lib.ID, "elapsed", time.Since(scanStart).String())
	return nil
}

// GetDiskSpace returns disk usage for the given path.
func GetDiskSpace(path string) (DiskSpace, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskSpace{}, err
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize)
	return DiskSpace{
		TotalBytes: total,
		FreeBytes:  free,
		UsedBytes:  total - free,
	}, nil
}

// ListUnmappedFolders returns top-level subfolders of a library that don't have
// any mapped files (files with a media_id set).
func (sc *Scanner) ListUnmappedFolders(ctx context.Context, lib *Library) ([]UnmappedFolder, error) {
	entries, err := os.ReadDir(lib.Path)
	if err != nil {
		return nil, err
	}

	// Collect mapped directory prefixes from library files.
	files, err := sc.store.ListFiles(ctx, lib.ID)
	if err != nil {
		return nil, err
	}
	mappedDirs := make(map[string]bool)
	for _, f := range files {
		if f.MediaID != nil {
			rel, _ := filepath.Rel(lib.Path, f.Path)
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) > 0 {
				mappedDirs[parts[0]] = true
			}
		}
	}

	var unmapped []UnmappedFolder
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !mappedDirs[e.Name()] {
			unmapped = append(unmapped, UnmappedFolder{
				Name: e.Name(),
				Path: filepath.Join(lib.Path, e.Name()),
			})
		}
	}
	return unmapped, nil
}
