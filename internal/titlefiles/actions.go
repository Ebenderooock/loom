package titlefiles

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/mediafiles"
)

// ActionFolder describes the contained, unshared folder used by file management.
// Rescan selections also include unregistered videos, with an empty MediaID.
type ActionFolder struct {
	Path  string
	Files []RegisteredFile
}

// RegisteredFile retains the original movie or episode identity for a file.
type RegisteredFile struct {
	File
	MediaID string
}

// ResolveActionFolder verifies a title folder and its complete registered files.
func (s *Service) ResolveActionFolder(ctx context.Context, mediaType, id string) (ActionFolder, error) {
	return s.resolveActionFolder(ctx, mediaType, id, false)
}

// ResolveRescanFolder includes unregistered videos in the verified title folder.
func (s *Service) ResolveRescanFolder(ctx context.Context, mediaType, id string) (ActionFolder, error) {
	return s.resolveActionFolder(ctx, mediaType, id, true)
}

func (s *Service) resolveActionFolder(ctx context.Context, mediaType, id string, discover bool) (ActionFolder, error) {
	sc, err := s.resolve(ctx, mediaType, id, nil)
	if err != nil {
		return ActionFolder{}, err
	}
	files, err := sc.list(nil)
	if err != nil {
		return ActionFolder{}, err
	}
	result := ActionFolder{Path: sc.root}
	unpacking := &imports.UnpackingSpec{}
	for _, file := range files {
		if (!discover && !file.Tracked) || file.Size <= 0 || !mediafiles.IsVideo(filepath.Ext(file.Path)) {
			continue
		}
		if unpacking.IsSatisfiedBy(ctx, &imports.ImportCandidate{SourcePath: filepath.Join(sc.root, file.Path)}) != nil {
			continue
		}
		if discover && !file.Tracked {
			result.Files = append(result.Files, RegisteredFile{File: file})
			continue
		}
		var recordedSize int64
		var mediaID string
		query := `SELECT size, movie_id FROM movie_files WHERE movie_id=? AND file_path=? AND deleted_at IS NULL`
		if mediaType == "series" {
			query = `SELECT file_size, episode_id FROM episode_files WHERE series_id=? AND file_path=?`
		}
		if err := s.db.QueryRowContext(ctx, query, id, filepath.Join(sc.root, file.Path)).Scan(&recordedSize, &mediaID); err != nil {
			return ActionFolder{}, err
		}
		if !discover && (recordedSize <= 0 || recordedSize != file.Size) {
			continue
		}
		result.Files = append(result.Files, RegisteredFile{File: file, MediaID: mediaID})
	}
	return result, nil
}

// ActionUnavailableReason deliberately excludes database errors and raw paths.
func ActionUnavailableReason(err error) string {
	switch {
	case errors.Is(err, errNotFound):
		return "Assign a library and ensure this title has an existing library folder."
	case errors.Is(err, errConflict):
		return "The title folder is shared, ambiguous, or outside its library."
	default:
		return "The title folder could not be read. Check library access and reload."
	}
}
