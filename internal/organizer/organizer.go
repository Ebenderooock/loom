package organizer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/loomctl/loom/internal/libraries"
	"github.com/loomctl/loom/internal/movies"
)

// MovieProvider is the narrow interface organizer needs from the movies domain.
type MovieProvider interface {
	GetMovie(ctx context.Context, id string) (*movies.Movie, error)
	ListMovies(ctx context.Context, limit, offset int) ([]*movies.Movie, error)
	ListMovieFiles(ctx context.Context, movieID string) ([]*movies.MovieFile, error)
	GetLibraryPath(ctx context.Context, libraryID string) (string, error)
}

// FileUpdater updates movie file records after rename.
type FileUpdater interface {
	UpdateMovieFile(ctx context.Context, mf *movies.MovieFile) error
}

// ConfigStore persists naming configuration.
type ConfigStore interface {
	GetNamingConfig(ctx context.Context) (*NamingConfig, error)
	SaveNamingConfig(ctx context.Context, cfg *NamingConfig) error
}

// Organizer handles file renaming and folder organization.
type Organizer struct {
	movies     MovieProvider
	files      FileUpdater
	configs    ConfigStore
	logger     *slog.Logger
	importMode string // "move" (default), "hardlink", or "hardlink_only"
}

// New creates a new Organizer.
func New(mp MovieProvider, fu FileUpdater, cs ConfigStore, logger *slog.Logger) *Organizer {
	return &Organizer{
		movies:     mp,
		files:      fu,
		configs:    cs,
		logger:     logger,
		importMode: "move",
	}
}

// SetImportMode configures how the organizer moves files.
// Supported modes: "move" (default rename/copy), "hardlink" (try hardlink, fall back to move),
// "hardlink_only" (hardlink or fail).
func (o *Organizer) SetImportMode(mode string) {
	switch mode {
	case "hardlink", "hardlink_only":
		o.importMode = mode
	default:
		o.importMode = "move"
	}
}

// GetConfig returns the current naming configuration.
func (o *Organizer) GetConfig(ctx context.Context) (*NamingConfig, error) {
	cfg, err := o.configs.GetNamingConfig(ctx)
	if err != nil {
		// Return default if not found
		return DefaultNamingConfig(), nil
	}
	return cfg, nil
}

// SaveConfig updates the naming configuration.
func (o *Organizer) SaveConfig(ctx context.Context, cfg *NamingConfig) error {
	if cfg.ID == "" {
		cfg.ID = "default"
	}
	return o.configs.SaveNamingConfig(ctx, cfg)
}

// PreviewMovie returns rename previews for all files of a movie.
func (o *Organizer) PreviewMovie(ctx context.Context, movieID string) ([]RenamePreview, error) {
	cfg, err := o.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("organizer: get config: %w", err)
	}

	movie, err := o.movies.GetMovie(ctx, movieID)
	if err != nil {
		return nil, fmt.Errorf("organizer: get movie: %w", err)
	}

	rootFolderPath, err := o.movies.GetLibraryPath(ctx, movie.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("organizer: get library path: %w", err)
	}

	files, err := o.movies.ListMovieFiles(ctx, movieID)
	if err != nil {
		return nil, fmt.Errorf("organizer: list files: %w", err)
	}

	previews := make([]RenamePreview, 0, len(files))
	seen := make(map[string]bool)

	for _, f := range files {
		target := BuildTargetPath(rootFolderPath, movie, f, cfg)

		// Check for collisions with other files in this batch
		collision := false
		if seen[target] {
			collision = true
			target = resolveCollision(target)
		}
		seen[target] = true

		// Also check disk collision (file exists but is not this file)
		if !collision && target != f.FilePath {
			if _, err := os.Stat(target); err == nil {
				collision = true
			}
		}

		previews = append(previews, RenamePreview{
			FileID:      f.ID,
			MovieID:     movieID,
			MovieTitle:  movie.Title,
			CurrentPath: f.FilePath,
			NewPath:     target,
			Changed:     target != f.FilePath,
			Collision:   collision,
		})
	}
	return previews, nil
}

// PreviewAll returns rename previews for all movies.
func (o *Organizer) PreviewAll(ctx context.Context) ([]RenamePreview, error) {
	allMovies, err := o.movies.ListMovies(ctx, 10000, 0)
	if err != nil {
		return nil, err
	}
	var all []RenamePreview
	for _, m := range allMovies {
		previews, err := o.PreviewMovie(ctx, m.ID)
		if err != nil {
			o.logger.Warn("preview failed for movie", "movie_id", m.ID, "error", err)
			continue
		}
		all = append(all, previews...)
	}
	return all, nil
}

// OrganizeMovie renames and moves all files for a single movie.
func (o *Organizer) OrganizeMovie(ctx context.Context, movieID string) ([]RenameResult, error) {
	cfg, err := o.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.RenameMovies {
		return nil, fmt.Errorf("organizer: renaming is disabled in naming config")
	}

	movie, err := o.movies.GetMovie(ctx, movieID)
	if err != nil {
		return nil, err
	}

	rootFolderPath, err := o.movies.GetLibraryPath(ctx, movie.LibraryID)
	if err != nil {
		return nil, err
	}

	files, err := o.movies.ListMovieFiles(ctx, movieID)
	if err != nil {
		return nil, err
	}

	results := make([]RenameResult, 0, len(files))
	for _, f := range files {
		result := o.organizeFile(ctx, movie, f, rootFolderPath, cfg)
		results = append(results, result)
	}
	return results, nil
}

// OrganizeMovies renames files for multiple movies.
func (o *Organizer) OrganizeMovies(ctx context.Context, movieIDs []string) ([]RenameResult, error) {
	var all []RenameResult
	for _, id := range movieIDs {
		results, err := o.OrganizeMovie(ctx, id)
		if err != nil {
			o.logger.Warn("organize failed for movie", "movie_id", id, "error", err)
			all = append(all, RenameResult{
				MovieID: id,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}
		all = append(all, results...)
	}
	return all, nil
}

// organizeFile handles the atomic rename of a single file.
func (o *Organizer) organizeFile(ctx context.Context, movie *movies.Movie, file *movies.MovieFile, libraryPath string, cfg *NamingConfig) RenameResult {
	target := BuildTargetPath(libraryPath, movie, file, cfg)

	result := RenameResult{
		FileID:  file.ID,
		MovieID: movie.ID,
		OldPath: file.FilePath,
		NewPath: target,
	}

	// No change needed
	if target == file.FilePath {
		result.Success = true
		return result
	}

	// Verify target is inside root folder (prevent path escape)
	cleanTarget := filepath.Clean(target)
	cleanRoot := filepath.Clean(libraryPath)
	if !strings.HasPrefix(cleanTarget, cleanRoot+string(filepath.Separator)) && cleanTarget != cleanRoot {
		result.Error = "target path escapes root folder"
		return result
	}

	// Check collision on disk
	if _, err := os.Stat(target); err == nil {
		// Resolve collision with suffix
		target = resolveCollision(target)
		result.NewPath = target
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(target)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		result.Error = fmt.Sprintf("create directory: %v", err)
		return result
	}

	// Move/link the file according to import mode
	if err := o.importFile(file.FilePath, target); err != nil {
		result.Error = fmt.Sprintf("move file: %v", err)
		return result
	}

	// Move sidecar files (.srt, .nfo, .sub, .idx, .ass)
	o.moveSidecars(file.FilePath, target)

	// Update database record
	oldPath := file.FilePath
	file.FilePath = target
	file.UpdatedAt = time.Now()
	if err := o.files.UpdateMovieFile(ctx, file); err != nil {
		// Rollback: move file back
		o.logger.Error("DB update failed, rolling back file move", "error", err)
		if rbErr := moveFile(target, oldPath); rbErr != nil {
			o.logger.Error("rollback failed — manual intervention required",
				"target", target, "original", oldPath, "error", rbErr)
		}
		file.FilePath = oldPath
		result.Error = fmt.Sprintf("update database: %v", err)
		return result
	}

	// Clean up empty source directory
	o.cleanEmptyDir(filepath.Dir(oldPath), libraryPath)

	result.Success = true
	o.logger.Info("file organized",
		"movie", movie.Title,
		"from", oldPath,
		"to", target,
	)
	return result
}

// importFile moves or links a file according to the organizer's import mode.
func (o *Organizer) importFile(src, dst string) error {
	switch o.importMode {
	case "hardlink":
		// Try hardlink first, fall back to move
		if err := os.Link(src, dst); err == nil {
			return nil
		}
		return moveFile(src, dst)
	case "hardlink_only":
		// Hardlink only — no fallback
		if err := os.Link(src, dst); err != nil {
			return fmt.Errorf("hardlink failed (hardlink_only mode): %w", err)
		}
		return nil
	default:
		return moveFile(src, dst)
	}
}

// moveFile moves a file, falling back to copy+delete for cross-device moves.
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	// Check for cross-device error
	if !errors.Is(err, os.ErrNotExist) && isCrossDevice(err) {
		return copyAndDelete(src, dst)
	}
	return err
}

// isCrossDevice checks if an error is a cross-device link error.
func isCrossDevice(err error) bool {
	return strings.Contains(err.Error(), "cross-device") ||
		strings.Contains(err.Error(), "invalid cross-device link") ||
		strings.Contains(err.Error(), "EXDEV")
}

// copyAndDelete copies a file then removes the original.
func copyAndDelete(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	// Check free space (best effort)
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		// Clean up partial copy
		os.Remove(dst)
		return fmt.Errorf("copy: %w", err)
	}

	// Sync to disk before deleting source
	if err := dstFile.Sync(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("sync: %w", err)
	}
	dstFile.Close()
	srcFile.Close()

	// Preserve modification time
	os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())

	return os.Remove(src)
}

// moveSidecars moves subtitle and metadata files alongside the main video file.
func (o *Organizer) moveSidecars(oldVideoPath, newVideoPath string) {
	sidecarExts := []string{".srt", ".sub", ".idx", ".ass", ".ssa", ".nfo", ".jpg", ".png"}
	oldBase := strings.TrimSuffix(oldVideoPath, filepath.Ext(oldVideoPath))
	newBase := strings.TrimSuffix(newVideoPath, filepath.Ext(newVideoPath))
	oldDir := filepath.Dir(oldVideoPath)

	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(oldDir, name)
		nameNoExt := strings.TrimSuffix(name, filepath.Ext(name))
		oldBaseFile := filepath.Base(oldBase)

		// Match files that start with the same base name (handles .en.srt, .forced.srt etc.)
		if strings.HasPrefix(nameNoExt, oldBaseFile) || nameNoExt == oldBaseFile {
			ext := filepath.Ext(name)
			// Check it's actually a sidecar extension
			isSidecar := false
			for _, se := range sidecarExts {
				if strings.EqualFold(ext, se) {
					isSidecar = true
					break
				}
			}
			// Also handle multi-extension like .en.srt
			if !isSidecar {
				for _, se := range sidecarExts {
					if strings.HasSuffix(strings.ToLower(name), se) {
						isSidecar = true
						break
					}
				}
			}
			if !isSidecar {
				continue
			}

			// Compute new sidecar path
			suffix := name[len(oldBaseFile):]
			newSidecar := newBase + suffix

			if err := moveFile(fullPath, newSidecar); err != nil {
				o.logger.Warn("failed to move sidecar", "file", fullPath, "error", err)
			}
		}
	}
}

// cleanEmptyDir removes empty directories up to (but not including) the root folder.
func (o *Organizer) cleanEmptyDir(dir, rootPath string) {
	cleanDir := filepath.Clean(dir)
	cleanRoot := filepath.Clean(rootPath)

	for cleanDir != cleanRoot && strings.HasPrefix(cleanDir, cleanRoot) {
		entries, err := os.ReadDir(cleanDir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(cleanDir); err != nil {
			break
		}
		o.logger.Debug("removed empty directory", "dir", cleanDir)
		cleanDir = filepath.Dir(cleanDir)
	}
}

// resolveCollision adds a numeric suffix to avoid path collisions.
func resolveCollision(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i <= 99; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	// Last resort: use UUID
	return fmt.Sprintf("%s (%s)%s", base, uuid.New().String()[:8], ext)
}

// SQLiteConfigStore persists naming config in SQLite.
type SQLiteConfigStore struct {
	db *sql.DB
}

// NewSQLiteConfigStore creates a config store backed by SQLite.
func NewSQLiteConfigStore(db *sql.DB) *SQLiteConfigStore {
	return &SQLiteConfigStore{db: db}
}

func (s *SQLiteConfigStore) GetNamingConfig(ctx context.Context) (*NamingConfig, error) {
	cfg := &NamingConfig{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, movie_folder_format, movie_file_format, colon_replacement, rename_movies FROM naming_config WHERE id = 'default'`,
	).Scan(&cfg.ID, &cfg.MovieFolderFormat, &cfg.MovieFileFormat, &cfg.ColonReplacement, &cfg.RenameMovies)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *SQLiteConfigStore) SaveNamingConfig(ctx context.Context, cfg *NamingConfig) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO naming_config (id, movie_folder_format, movie_file_format, colon_replacement, rename_movies, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   movie_folder_format = excluded.movie_folder_format,
		   movie_file_format = excluded.movie_file_format,
		   colon_replacement = excluded.colon_replacement,
		   rename_movies = excluded.rename_movies,
		   updated_at = CURRENT_TIMESTAMP`,
		cfg.ID, cfg.MovieFolderFormat, cfg.MovieFileFormat, cfg.ColonReplacement, cfg.RenameMovies,
	)
	return err
}

// MovieServiceAdapter adapts movies.Service to the MovieProvider interface.
type MovieServiceAdapter struct {
	Svc      movies.Service
	LibStore *libraries.Store
}

func (a *MovieServiceAdapter) GetMovie(ctx context.Context, id string) (*movies.Movie, error) {
	return a.Svc.GetMovie(ctx, id)
}

func (a *MovieServiceAdapter) ListMovies(ctx context.Context, limit, offset int) ([]*movies.Movie, error) {
	return a.Svc.ListMovies(ctx, limit, offset)
}

func (a *MovieServiceAdapter) ListMovieFiles(ctx context.Context, movieID string) ([]*movies.MovieFile, error) {
	return a.Svc.ListMovieFiles(ctx, movieID)
}

func (a *MovieServiceAdapter) GetLibraryPath(ctx context.Context, libraryID string) (string, error) {
	lib, err := a.LibStore.Get(ctx, libraryID)
	if err != nil {
		return "", err
	}
	return lib.Path, nil
}

// RepoFileUpdater adapts movies.Repository to the FileUpdater interface.
type RepoFileUpdater struct {
	Repo movies.Repository
}

func (u *RepoFileUpdater) UpdateMovieFile(ctx context.Context, mf *movies.MovieFile) error {
	return u.Repo.UpdateMovieFile(ctx, mf)
}

// PreviewSampleResponse is a preview using sample data for settings UI.
type PreviewSampleResponse struct {
	FolderExample string `json:"folder_example"`
	FileExample   string `json:"file_example"`
	FullPath      string `json:"full_path"`
}

// PreviewSample returns a preview with sample movie data for the naming settings UI.
func PreviewSample(cfg *NamingConfig) *PreviewSampleResponse {
	sampleMovie := &movies.Movie{
		Title: "The Dark Knight",
		Year:  2008,
	}
	sampleFile := &movies.MovieFile{
		Quality: "Bluray-1080p",
		Format:  "mkv",
		MediaInfo: map[string]interface{}{
			"audio_codec":    "DTS-HD MA",
			"audio_channels": "5.1",
			"dynamic_range":  "SDR",
		},
		FilePath: "/movies/The.Dark.Knight.2008.1080p.BluRay.x264-GROUP.mkv",
	}

	folder := FormatFolderName(sampleMovie, cfg)
	file := FormatFileName(sampleMovie, sampleFile, cfg) + ".mkv"

	return &PreviewSampleResponse{
		FolderExample: folder,
		FileExample:   file,
		FullPath:      filepath.Join("/movies", folder, file),
	}
}

// NamingConfigRequest is the API payload for updating naming config.
type NamingConfigRequest struct {
	MovieFolderFormat string `json:"movie_folder_format"`
	MovieFileFormat   string `json:"movie_file_format"`
	ColonReplacement  string `json:"colon_replacement"`
	RenameMovies      bool   `json:"rename_movies"`
}

// OrganizeRequest is the API payload for organizing movies.
type OrganizeRequest struct {
	MovieIDs []string `json:"movie_ids"`
}

// PreviewRequest is the API payload for previewing renames.
type PreviewRequest struct {
	MovieIDs []string `json:"movie_ids"`
}
