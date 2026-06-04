package cleanup

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RootsFunc resolves the current set of download folders to scan, one per
// enabled download client.
type RootsFunc func(ctx context.Context) ([]Root, error)

// TrackedPathsFunc returns every on-disk path currently tracked by a download
// client (active downloads) or an in-progress/recent import. Any orphan
// candidate that equals, contains, or is contained by one of these is protected.
type TrackedPathsFunc func(ctx context.Context) ([]string, error)

// ProtectedPathsFunc returns paths that must never be scanned or deleted —
// primarily media library roots. Any download root or orphan candidate that
// overlaps one of these (equals, sits inside, or contains it) is excluded.
type ProtectedPathsFunc func(ctx context.Context) ([]string, error)

// RecycleFunc removes a single orphan entry (file or directory). Implementations
// should prefer recoverable removal (recycle bin) over hard deletion.
type RecycleFunc func(ctx context.Context, path, root string) error

// Service coordinates scanning, review, and deletion of download orphans.
type Service struct {
	store     *Store
	roots     RootsFunc
	tracked   TrackedPathsFunc
	protected ProtectedPathsFunc
	recycle   RecycleFunc
	logger    *slog.Logger
	mu        sync.Mutex // serializes scans and deletes
	skipNames map[string]bool
}

// Options configures a cleanup Service.
type Options struct {
	Store     *Store
	Roots     RootsFunc
	Tracked   TrackedPathsFunc
	Protected ProtectedPathsFunc
	Recycle   RecycleFunc
	Logger    *slog.Logger
}

// NewService constructs a cleanup Service.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:     opts.Store,
		roots:     opts.Roots,
		tracked:   opts.Tracked,
		protected: opts.Protected,
		recycle:   opts.Recycle,
		logger:    logger,
		// Skip the import recycle/incomplete bookkeeping dirs and junk we
		// should never flag or remove.
		skipNames: map[string]bool{
			".loom-cleanup": true,
			".incomplete":   true,
			"incomplete":    true,
		},
	}
}

// Store exposes the underlying store for read-only handlers.
func (s *Service) Store() *Store { return s.store }

// protectedPaths resolves the configured protected (e.g. media library) paths.
// A resolution error is fatal to any destructive operation: callers must abort
// rather than risk treating a library entry as an orphan.
func (s *Service) protectedPaths(ctx context.Context) ([]string, error) {
	if s.protected == nil {
		return nil, nil
	}
	return s.protected(ctx)
}

// rootIsSafe reports whether a download root may be scanned/deleted within. A
// root that overlaps any protected (library) path in either direction is unsafe
// and is skipped entirely.
func rootIsSafe(rootPath string, protected []string) bool {
	return !isProtected(rootPath, protected)
}

// Scan walks every download root, classifies top-level entries, records new
// orphans, and resolves entries that are no longer orphaned. It never deletes.
func (s *Service) Scan(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanLocked(ctx)
}

func (s *Service) scanLocked(ctx context.Context) (int, error) {
	roots, err := s.roots(ctx)
	if err != nil {
		return 0, err
	}
	tracked, err := s.tracked(ctx)
	if err != nil {
		// Without the tracked set we cannot safely classify anything.
		return 0, err
	}
	protected, err := s.protectedPaths(ctx)
	if err != nil {
		// Without the library set we cannot guarantee we won't flag library
		// content, so abort.
		return 0, err
	}

	seen := make(map[string]bool)
	found := 0
	for _, root := range roots {
		rootPath := normPath(root.Path)
		if rootPath == "" {
			continue
		}
		if !rootIsSafe(rootPath, protected) {
			s.logger.Warn("cleanup: skipping download root that overlaps a media library", "root", rootPath)
			continue
		}
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			s.logger.Warn("cleanup: cannot read download root", "root", rootPath, "err", err)
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if s.skipNames[name] {
				continue
			}
			full := filepath.Join(rootPath, name)
			// Never follow symlinks — an orphan symlink could point into a
			// media library. Skip them entirely so we never recurse or delete
			// across the link.
			info, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if isProtected(full, tracked) {
				continue
			}
			seen[full] = true
			found++
			if err := s.store.Upsert(ctx, Orphan{
				Path:      full,
				ClientID:  root.ClientID,
				Root:      rootPath,
				SizeBytes: entrySize(full),
			}); err != nil {
				s.logger.Warn("cleanup: upsert orphan failed", "path", full, "err", err)
			}
		}
	}

	if err := s.store.ResolveStalePending(ctx, seen); err != nil {
		s.logger.Warn("cleanup: resolving stale orphans failed", "err", err)
	}
	return found, nil
}

// AutoDelete recycles pending orphans older than the retention period, when
// auto-delete is enabled. Each candidate is revalidated immediately before
// removal. Returns the number of entries deleted.
func (s *Service) AutoDelete(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return 0, err
	}
	if !settings.AutoDeleteEnabled {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)

	pending, err := s.store.List(ctx, StatusPending)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, o := range pending {
		if o.FirstSeenAt.IsZero() || o.FirstSeenAt.After(cutoff) {
			continue
		}
		ok, err := s.removeLocked(ctx, o)
		if err != nil {
			s.logger.Warn("cleanup: auto-delete failed", "path", o.Path, "err", err)
			continue
		}
		if ok {
			deleted++
		}
	}
	return deleted, nil
}

// Approve immediately recycles a single orphan after revalidation.
func (s *Service) Approve(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.removeLocked(ctx, o)
	return err
}

// Ignore marks an orphan to be kept permanently.
func (s *Service) Ignore(ctx context.Context, id string) error {
	return s.store.MarkIgnored(ctx, id)
}

// removeLocked revalidates an orphan against the current download/import state
// and roots, then recycles it. The caller must hold s.mu. It returns
// (deleted, error); deleted is false when the orphan resolved itself.
func (s *Service) removeLocked(ctx context.Context, o Orphan) (bool, error) {
	roots, err := s.roots(ctx)
	if err != nil {
		return false, err
	}
	tracked, err := s.tracked(ctx)
	if err != nil {
		return false, err
	}
	protected, err := s.protectedPaths(ctx)
	if err != nil {
		return false, err
	}
	rootPaths := make([]string, 0, len(roots))
	for _, r := range roots {
		rootPaths = append(rootPaths, r.Path)
	}

	// Revalidate: still inside a download root, still untracked, still present,
	// and not overlapping a protected media library.
	if !underAnyRoot(o.Path, rootPaths) {
		return false, s.store.Delete(ctx, o.ID)
	}
	if isProtected(o.Path, protected) {
		// A library now overlaps this path — never delete; drop the record.
		s.logger.Warn("cleanup: refusing to delete orphan overlapping a media library", "path", o.Path)
		return false, s.store.Delete(ctx, o.ID)
	}
	if isProtected(o.Path, tracked) {
		return false, s.store.Delete(ctx, o.ID)
	}
	info, err := os.Lstat(o.Path)
	if os.IsNotExist(err) {
		return false, s.store.Delete(ctx, o.ID)
	}
	if err != nil {
		return false, s.store.MarkFailed(ctx, o.ID, err.Error())
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Should never reach here (scan skips symlinks) but guard anyway.
		return false, s.store.Delete(ctx, o.ID)
	}

	if err := s.recycle(ctx, o.Path, o.Root); err != nil {
		_ = s.store.MarkFailed(ctx, o.ID, err.Error())
		return false, err
	}
	if err := s.store.MarkDeleted(ctx, o.ID); err != nil {
		return true, err
	}
	s.logger.Info("cleanup: removed orphan", "path", o.Path, "size", o.SizeBytes)
	return true, nil
}

// entrySize returns the total size of a file, or the recursive size of a
// directory. It never follows symlinks and tolerates partial walk errors.
func entrySize(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total
}
