package titlefiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/cleanup"
	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/parser"
)

var (
	errInvalid  = errors.New("invalid file selection")
	errNotFound = errors.New("title or folder not found")
	errConflict = errors.New("file management is unavailable")
	seasonDir   = regexp.MustCompile(`(?i)^(?:season\s*|s)(\d+)$`)
)

// File describes a regular file, using only a title-relative path on the wire.
type File struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Tracked bool   `json:"tracked"`
	Quality string `json:"quality,omitempty"`
	Format  string `json:"format,omitempty"`
}

// Listing includes the current removal policy, which must be confirmed again
// when deleting so configuration changes cannot silently change the operation.
type Listing struct {
	Files        []File `json:"files"`
	DeletionMode string `json:"deletionMode"`
}

type trackedFile struct {
	path, quality, format string
	season                int
	active                bool
}

type scope struct {
	root, library, title string
	tracked              []trackedFile
}

// Service owns the disk-to-database deletion boundary.
type Service struct {
	db        *sql.DB
	libraries *libraries.Store
	cfg       *appconfig.Config
	audit     *auditlog.Logger
	reconcile func(context.Context) error
	recycle   cleanup.RecycleFunc
	mu        sync.Mutex
}

// New constructs the manager. Reconciliation refreshes fulfilled request
// availability without undoing fulfillment.
func New(db *sql.DB, store *libraries.Store, cfg *appconfig.Config, audit *auditlog.Logger, reconcile func(context.Context) error) *Service {
	if audit == nil {
		audit = auditlog.New(db, nil)
	}
	return &Service{db: db, libraries: store, cfg: cfg, audit: audit, reconcile: reconcile}
}

func (s *Service) deletionMode() string {
	if s.cfg.RecycleBin.Enabled && s.cfg.RecycleBin.Path != "" {
		return "recycle"
	}
	return "permanent"
}

// List enumerates all regular files, never following symlinks.
func (s *Service) List(ctx context.Context, mediaType, id string, season *int) (Listing, error) {
	sc, err := s.resolve(ctx, mediaType, id, season)
	if err != nil {
		return Listing{}, err
	}
	files, err := sc.list(season)
	return Listing{Files: files, DeletionMode: s.deletionMode()}, err
}

func (sc scope) list(season *int) ([]File, error) {
	root, err := sc.openRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return sc.listFromRoot(root, season)
}

func (sc scope) openRoot() (*os.Root, error) {
	library, err := os.OpenRoot(sc.library)
	if err != nil {
		return nil, err
	}
	defer func() { _ = library.Close() }()
	name := filepath.Base(sc.root)
	before, err := library.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: title folder is not a real directory", errConflict)
	}
	root, err := library.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = root.Close()
		return nil, fmt.Errorf("%w: title folder changed; reload files", errConflict)
	}
	return root, nil
}

func (sc scope) listFromRoot(root *os.Root, season *int) ([]File, error) {
	tracked := make(map[string]trackedFile, len(sc.tracked))
	seasonFolders := map[string]map[int]bool{}
	for _, f := range sc.tracked {
		if f.active {
			tracked[filepath.Clean(f.path)] = f
			rel, err := filepath.Rel(sc.root, f.path)
			if err == nil {
				parts := strings.Split(filepath.ToSlash(rel), "/")
				if len(parts) > 1 {
					if seasonFolders[parts[0]] == nil {
						seasonFolders[parts[0]] = map[int]bool{}
					}
					seasonFolders[parts[0]][f.season] = true
				}
			}
		}
	}
	files := []File{}
	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, known := tracked[filepath.Join(sc.root, filepath.FromSlash(path))]
		if season != nil && !inSeason(path, f, known, *season, seasonFolders) {
			return nil
		}
		file := File{Path: path, Size: info.Size(), Tracked: known, Format: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")}
		if known {
			file.Quality = f.quality
			if f.format != "" {
				file.Format = f.format
			}
		}
		files = append(files, file)
		return nil
	})
	return files, err
}

func inSeason(path string, f trackedFile, known bool, season int, folders map[string]map[int]bool) bool {
	if known {
		return f.season == season
	}
	if parts := strings.Split(path, "/"); len(parts) > 1 {
		if assigned := folders[parts[0]]; len(assigned) == 1 && assigned[season] {
			return true
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		if strings.EqualFold(part, "specials") {
			return season == 0
		}
		if m := seasonDir.FindStringSubmatch(part); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n == season
		}
	}
	parsed := parser.Parse(filepath.Base(path))
	return len(parsed.Episodes) > 0 && parsed.Season == season
}

// Delete prevalidates the entire selection. Runtime failures may leave an
// explicitly reported partial result; each successful disk removal is reconciled
// and audited before proceeding to the next file.
func (s *Service) Delete(ctx context.Context, mediaType, id string, season *int, paths []string, mode string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := []string{}
	if len(paths) == 0 || len(paths) > 1000 {
		return deleted, fmt.Errorf("%w: select between 1 and 1000 files", errInvalid)
	}
	if mode != s.deletionMode() {
		return deleted, fmt.Errorf("%w: deletion policy changed; reload files and confirm again", errConflict)
	}
	sc, err := s.resolve(ctx, mediaType, id, season)
	if err != nil {
		return deleted, err
	}
	root, err := sc.openRoot()
	if err != nil {
		return deleted, err
	}
	defer func() { _ = root.Close() }()
	files, err := sc.listFromRoot(root, season)
	if err != nil {
		return deleted, err
	}
	available := make(map[string]bool, len(files))
	for _, f := range files {
		available[f.Path] = true
	}
	for _, path := range paths {
		if !available[path] {
			return deleted, fmt.Errorf("%w: %q is not an available file in this scope", errInvalid, path)
		}
		delete(available, path)
		if err := safeFile(sc.root, path); err != nil {
			return deleted, err
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO title_file_folders(media_type,title_id,library_id,folder)
		SELECT ?, ?, library_id, ? FROM `+map[string]string{"movie": "movies", "series": "series"}[mediaType]+` WHERE id=?
		ON CONFLICT(media_type,title_id) DO UPDATE SET library_id=excluded.library_id, folder=excluded.folder`,
		mediaType, id, filepath.Base(sc.root), id)
	if err != nil {
		return deleted, err
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		if err := safeFile(sc.root, path); err != nil {
			return deleted, err
		}
		absolute := filepath.Join(sc.root, filepath.FromSlash(path))
		if mode == "recycle" {
			recycle := s.recycle
			if recycle == nil {
				bin := imports.NewRecycleBin(s.cfg)
				recycle = func(_ context.Context, _ string, library string) error {
					return bin.RecycleFromRoot(root, filepath.FromSlash(path), library)
				}
			}
			err = recycle(ctx, absolute, sc.library)
		} else {
			err = root.Remove(filepath.FromSlash(path))
		}
		if err != nil {
			return deleted, fmt.Errorf("remove %q: %w", path, err)
		}
		deleted = append(deleted, path)
		// Once disk changes, client disconnects must not cancel bookkeeping.
		bookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		actor := ""
		if identity := auth.IdentityFrom(ctx); identity != nil {
			actor = identity.Username
		}
		s.audit.Log(bookCtx, auditlog.Entry{
			Category: "library", EventType: "file_deleted", Message: "Deleted file from " + sc.title,
			EntityType: &mediaType, EntityID: &id, EntityName: &sc.title,
			Detail: auditlog.DetailJSON(map[string]any{"path": path, "mode": mode, "actor": actor}),
		})
		err = s.libraries.ReconcileRemovedPaths(bookCtx, []string{absolute})
		if err == nil {
			_, err = s.db.ExecContext(bookCtx, `DELETE FROM library_files WHERE path = ?`, absolute)
		}
		if err == nil && s.reconcile != nil {
			err = s.reconcile(bookCtx)
		}
		cancel()
		if err != nil {
			return deleted, fmt.Errorf("file removed but database/request reconciliation failed: %w", err)
		}
	}
	return deleted, nil
}

func safeFile(root, path string) error {
	if !fs.ValidPath(path) || strings.Contains(path, `\`) || !filepath.IsLocal(filepath.FromSlash(path)) {
		return fmt.Errorf("%w: unsafe relative path", errInvalid)
	}
	current := root
	for _, part := range strings.Split(path, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic links cannot be deleted", errInvalid)
		}
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return fmt.Errorf("%w: file escapes title folder", errInvalid)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: only regular files can be deleted", errInvalid)
	}
	return nil
}
