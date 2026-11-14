package imports

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// Serialize imports to the same destination within this process.
var replacementLocks = struct {
	sync.Mutex
	entries map[string]*importDestinationLock
}{entries: make(map[string]*importDestinationLock)}

type importDestinationLock struct {
	sync.Mutex
	users int
}

func lockImportDestination(path string) func() {
	path, _ = filepath.Abs(path)
	path = filepath.Clean(path)
	replacementLocks.Lock()
	m := replacementLocks.entries[path]
	if m == nil {
		m = &importDestinationLock{}
		replacementLocks.entries[path] = m
	}
	m.users++
	replacementLocks.Unlock()
	m.Lock()
	return func() {
		m.Unlock()
		replacementLocks.Lock()
		m.users--
		if m.users == 0 {
			delete(replacementLocks.entries, path)
		}
		replacementLocks.Unlock()
	}
}

// matchedDestination verifies that the existing pathname belongs to this item.
func (p *ImportPipeline) matchedDestination(ctx context.Context, match *MatchResult, dest string) (bool, string, error) {
	var otherMovies, otherEpisodes int
	movieID, episodeID := "", ""
	switch match.MediaType {
	case MediaTypeMovie:
		movieID = match.MediaID
	case MediaTypeEpisode:
		episodeID = match.MediaID
	}
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM movie_files WHERE file_path = ? AND movie_id <> ?`, dest, movieID).Scan(&otherMovies); err != nil {
		return false, "", err
	}
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files WHERE file_path = ? AND episode_id <> ?`, dest, episodeID).Scan(&otherEpisodes); err != nil {
		return false, "", err
	}
	if otherMovies+otherEpisodes != 0 {
		return false, "", fmt.Errorf("%w: destination belongs to another media item: %s", ErrDestinationExists, dest)
	}
	switch match.MediaType {
	case MediaTypeMovie:
		files, err := p.moviesSvc.ListMovieFiles(ctx, match.MediaID)
		if err != nil {
			return false, "", err
		}
		for _, f := range files {
			if filepath.Clean(f.FilePath) == filepath.Clean(dest) {
				return true, f.Quality, nil
			}
		}
	case MediaTypeEpisode:
		var quality string
		err := p.db.QueryRowContext(ctx, `SELECT COALESCE(quality, '') FROM episode_files WHERE episode_id = ? AND file_path = ? LIMIT 1`, match.MediaID, dest).Scan(&quality)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, "", err
		}
		return err == nil, quality, nil
	}
	return false, "", nil
}

// replaceMatched stages and verifies the new file before replacing the old one.
// Both files remain recoverable until library registration succeeds.
func (p *ImportPipeline) replaceMatched(ctx context.Context, match *MatchResult, source, dest string, old os.FileInfo, size int64, quality string) (retErr error) {
	dir, err := os.MkdirTemp(filepath.Dir(dest), ".loom-replacement-*")
	if err != nil {
		return fmt.Errorf("create replacement staging directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			if err := os.RemoveAll(dir); err != nil { //nolint:gosec // Private directory created immediately above.
				retErr = errors.Join(retErr, fmt.Errorf("clean replacement staging directory %s: %w", dir, err))
			}
		}
	}()
	staged := filepath.Join(dir, "incoming")
	backup := filepath.Join(dir, "previous")
	mode := p.importMode
	if mode == ImportModeMove {
		mode = ImportModeHardlink
	}
	if err := importFile(source, staged, mode); err != nil {
		return fmt.Errorf("stage replacement: %w", err)
	}
	if p.verifier != nil {
		if vr := p.verifier.Verify(staged, size); !vr.OK {
			return fmt.Errorf("verification failed: %s", vr.Reason)
		}
	}
	srcInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	stagedInfo, err := os.Lstat(staged)
	if err != nil {
		return err
	}
	same, err := p.sameContent(ctx, source, staged, srcInfo, stagedInfo)
	if err != nil {
		return err
	}
	if !same {
		return errors.New("replacement verification failed: source and staged content differ")
	}
	current, err := os.Lstat(dest)
	if err != nil || !os.SameFile(old, current) {
		return fmt.Errorf("replacement destination changed: %s", dest)
	}
	// Preserve the old file without taking its live pathname away. The final
	// rename then replaces it in one operation after the new file is ready.
	if err := importFile(dest, backup, ImportModeHardlink); err != nil {
		return fmt.Errorf("preserve existing destination: %w", err)
	}
	saved, err := os.Lstat(backup)
	if err != nil {
		return fmt.Errorf("inspect replacement backup: %w", err)
	}
	same, err = p.sameContent(ctx, dest, backup, old, saved)
	if err != nil {
		return fmt.Errorf("verify replacement backup: %w", err)
	}
	if !same {
		return errors.New("replacement backup differs from the existing destination")
	}
	current, err = os.Lstat(dest)
	if err != nil || !os.SameFile(old, current) || old.Size() != current.Size() || !old.ModTime().Equal(current.ModTime()) {
		return fmt.Errorf("replacement destination changed: %s", dest)
	}
	if err := os.Rename(staged, dest); err != nil {
		return fmt.Errorf("publish replacement: %w", err)
	}
	keep = true
	if err := p.updateReplacementRecord(ctx, match, dest, size, quality); err != nil {
		current, statErr := os.Lstat(dest)
		if statErr != nil || !os.SameFile(stagedInfo, current) {
			return fmt.Errorf("library update failed: %w; destination changed; old file preserved at %s", err, backup)
		}
		if restoreErr := os.Rename(backup, dest); restoreErr != nil {
			return fmt.Errorf("library update failed: %w; old file preserved at %s (restore: %w)", err, backup, restoreErr)
		}
		keep = false
		return fmt.Errorf("library update failed (source preserved): %w", err)
	}
	if p.recycler != nil && p.recycler.Enabled && p.recycler.Path != "" {
		if err := p.recycler.recycleAs(backup, dest, filepath.Dir(match.DestPath)); err != nil {
			return fmt.Errorf("recycle old file (preserved at %s): %w", backup, err)
		}
	}
	keep = false
	return nil
}

// Update the matched record rather than adding a duplicate episode file or
// retaining stale quality metadata after a same-path replacement.
func (p *ImportPipeline) updateReplacementRecord(ctx context.Context, match *MatchResult, dest string, size int64, quality string) error {
	var query string
	switch match.MediaType {
	case MediaTypeMovie:
		query = `UPDATE movie_files SET size = ?, quality = COALESCE(NULLIF(?, ''), quality) WHERE movie_id = ? AND file_path = ? AND deleted_at IS NULL`
	case MediaTypeEpisode:
		query = `UPDATE episode_files SET file_size = ?, quality = COALESCE(NULLIF(?, ''), quality) WHERE episode_id = ? AND file_path = ?`
	default:
		return fmt.Errorf("unsupported replacement media type %q", match.MediaType)
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, query, size, quality, match.MediaID, dest)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expected one matched file record, updated %d", n)
	}
	return tx.Commit()
}

func releaseQuality(path string) string {
	return qualityprofiles.QualityNameFromRelease(parser.Parse(filepath.Base(path)))
}
