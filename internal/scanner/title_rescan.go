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

// RescanMovieFolder registers videos only for the selected movie in its verified folder.
func (s *Scanner) RescanMovieFolder(ctx context.Context, id string, folder titlefiles.ActionFolder) (*ScanResult, error) {
	movie, err := s.movieSvc.GetMovie(ctx, id)
	if err != nil {
		return nil, err
	}
	result := folderRescanResult(movie.LibraryID, folder)
	for _, file := range folder.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !file.Tracked && (file.Size < 50*1024*1024 || shouldIgnore(file.Path)) {
			result.TotalFiles--
			continue
		}
		path := filepath.Join(folder.Path, filepath.FromSlash(file.Path))
		registered, err := s.movieSvc.GetMovieFileByPathIncludingDeleted(ctx, path)
		if err == nil && registered != nil && registered.MovieID != id {
			err = fmt.Errorf("file registration no longer belongs to selected movie")
		}
		if err == nil && (registered == nil || registered.DeletedAt != nil || registered.Size != file.Size) {
			parsed := parser.Parse(filepath.Base(path))
			now := time.Now()
			mf := &movies.MovieFile{
				ID: uuid.New().String(), MovieID: id, FilePath: path, Size: file.Size,
				Quality: qualityFromParsedInfo(parsed.Resolution, parsed.Source, parsed.IsRemux),
				Format:  parsed.Source, DateAdded: now, CreatedAt: now, UpdatedAt: now,
			}
			if registered == nil {
				err = s.movieSvc.AddMovieFile(ctx, mf)
			} else {
				err = s.movieSvc.ReviveMovieFile(ctx, registered.ID, mf)
			}
			if err == nil {
				result.Imported++
			}
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Could not check %s", file.Path))
			s.logger.WarnContext(ctx, "movie folder rescan failed", "movie", id, "error", err)
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
	finishFolderRescanResult(result)
	return result, nil
}

// RescanSeriesFolder discovers episodes only inside the selected series' verified folder.
func (s *SeriesScanner) RescanSeriesFolder(ctx context.Context, id string, folder titlefiles.ActionFolder) (*ScanResult, error) {
	sr, err := s.seriesSvc.GetSeries(ctx, id)
	if err != nil {
		return nil, err
	}
	result := folderRescanResult(sr.LibraryID, folder)
	for _, file := range folder.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !file.Tracked && (file.Size < 10*1024*1024 || shouldIgnore(file.Path)) {
			result.TotalFiles--
			continue
		}
		path := filepath.Join(folder.Path, filepath.FromSlash(file.Path))
		parsed := parser.Parse(filepath.Base(path))
		mediaID := file.MediaID
		if mediaID == "" {
			season := parsed.Season
			for dir := filepath.Dir(path); season < 0 && dir != folder.Path; dir = filepath.Dir(dir) {
				season = extractSeasonFromDir(dir)
			}
			episode, err := s.findEpisode(ctx, id, season, parsed.Episode)
			if err != nil {
				s.logger.WarnContext(ctx, "series episode lookup failed", "series", id, "error", err)
				result.Errors = append(result.Errors, fmt.Sprintf("Could not match %s", file.Path))
				continue
			}
			if episode == nil {
				result.Unmatched++
				continue
			}
			mediaID = episode.ID
		}
		episode, err := s.seriesSvc.GetEpisode(ctx, mediaID)
		if err == nil && (episode == nil || episode.SeriesID != id) {
			err = fmt.Errorf("episode no longer belongs to selected title")
		}
		if err == nil {
			err = s.importEpisodeFile(ctx, episode, path, file.Size, parsed)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Could not check %s", file.Path))
			s.logger.WarnContext(ctx, "series folder rescan failed", "series", id, "error", err)
			continue
		}
		result.Matched++
		if !file.Tracked {
			result.Imported++
		}
	}
	finishFolderRescanResult(result)
	return result, nil
}

func folderRescanResult(libraryID string, folder titlefiles.ActionFolder) *ScanResult {
	return &ScanResult{
		ID: uuid.New().String(), LibraryID: libraryID, RootFolderPath: folder.Path,
		Status: ScanStatusRunning, StartedAt: time.Now(), TotalFiles: len(folder.Files),
	}
}

func finishFolderRescanResult(result *ScanResult) {
	now := time.Now()
	result.CompletedAt = &now
	result.Status = ScanStatusCompleted
	if len(result.Errors) > 0 {
		result.Status = ScanStatusFailed
	}
}
