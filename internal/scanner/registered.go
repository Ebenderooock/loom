package scanner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/titlefiles"
)

// RescanRegisteredMovie uses existing file-processing semantics on a verified
// selection. It never walks a library or ingests unregistered download files.
func (s *Scanner) RescanRegisteredMovie(ctx context.Context, id string, folder titlefiles.ActionFolder) (*ScanResult, error) {
	movie, err := s.movieSvc.GetMovie(ctx, id)
	if err != nil {
		return nil, err
	}
	result := registeredResult(movie.LibraryID, folder)
	for _, file := range folder.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(folder.Path, file.Path)
		registered, err := s.movieSvc.GetMovieFileByPath(ctx, path)
		if err == nil && (registered == nil || registered.MovieID != id) {
			err = fmt.Errorf("file registration no longer belongs to selected movie")
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Could not check %s", file.Path))
			s.logger.WarnContext(ctx, "registered movie file rescan failed", "movie", id, "error", err)
			continue
		}
		result.Matched++
	}
	if result.Matched > 0 && len(result.Errors) == 0 {
		if err := s.movieSvc.SetMovieStatus(ctx, id, movies.MovieStatusAvailableRightQuality); err != nil {
			s.logger.WarnContext(ctx, "registered movie availability update failed", "movie", id, "error", err)
			result.Errors = append(result.Errors, "Could not update movie availability.")
		}
	}
	finishRegisteredResult(result)
	return result, nil
}

// RescanRegisteredSeries checks existing episode registrations without discovering files.
func (s *SeriesScanner) RescanRegisteredSeries(ctx context.Context, id string, folder titlefiles.ActionFolder) (*ScanResult, error) {
	sr, err := s.seriesSvc.GetSeries(ctx, id)
	if err != nil {
		return nil, err
	}
	result := registeredResult(sr.LibraryID, folder)
	for _, file := range folder.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(folder.Path, file.Path)
		episode, err := s.seriesSvc.GetEpisode(ctx, file.MediaID)
		if err == nil && (episode == nil || episode.SeriesID != id) {
			err = fmt.Errorf("episode no longer belongs to selected title")
		}
		if err == nil {
			err = s.importEpisodeFile(ctx, episode, path, file.Size, parser.Parse(filepath.Base(path)))
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Could not check %s", file.Path))
			s.logger.WarnContext(ctx, "registered series file rescan failed", "series", id, "error", err)
			continue
		}
		result.Matched++
	}
	finishRegisteredResult(result)
	return result, nil
}

func registeredResult(libraryID string, folder titlefiles.ActionFolder) *ScanResult {
	return &ScanResult{
		ID: uuid.New().String(), LibraryID: libraryID, RootFolderPath: folder.Path,
		Status: ScanStatusRunning, StartedAt: time.Now(), TotalFiles: len(folder.Files),
	}
}

func finishRegisteredResult(result *ScanResult) {
	now := time.Now()
	result.CompletedAt = &now
	result.Status = ScanStatusCompleted
	if len(result.Errors) > 0 {
		result.Status = ScanStatusFailed
	}
}
