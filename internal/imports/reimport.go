package imports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ReimportOptions controls reimport behaviour.
type ReimportOptions struct {
	ConflictPolicy ConflictPolicy `json:"conflict_policy"`
}

// ReimportFile forces a reimport of a source file to the library for the
// given media item, even if a file already exists at the destination.
func (p *ImportPipeline) ReimportFile(ctx context.Context, mediaType MediaType, mediaID, sourcePath string, opts ReimportOptions) (*ImportRecord, error) {
	if opts.ConflictPolicy == "" {
		opts.ConflictPolicy = ConflictReplaceIfBetter
	}

	// Verify source exists
	srcInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("source file not found: %w", err)
	}
	if srcInfo.IsDir() {
		return nil, fmt.Errorf("source path is a directory, expected a file")
	}

	// Resolve destination via matcher
	match, err := p.resolveMediaDestination(ctx, mediaType, mediaID, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}

	destFile := filepath.Join(match.DestPath, filepath.Base(sourcePath))

	// Check for existing file at destination
	existingInfo, existsErr := os.Stat(destFile)
	fileExists := existsErr == nil

	var decision ConflictDecision
	if fileExists {
		existing := FileInfo{
			Path:    destFile,
			Size:    existingInfo.Size(),
			Quality: ParseFileQuality(destFile),
		}
		incoming := FileInfo{
			Path:    sourcePath,
			Size:    srcInfo.Size(),
			Quality: ParseFileQuality(sourcePath),
		}

		decision = ResolveConflict(existing, incoming, opts.ConflictPolicy)

		// Log the conflict decision
		if p.decisionLog != nil {
			_ = p.decisionLog.Log(ctx, ImportDecision{
				SourcePath:     sourcePath,
				DestPath:       destFile,
				MediaType:      string(mediaType),
				MediaID:        mediaID,
				Action:         string(decision.Action),
				Reason:         decision.Reason,
				ConflictPolicy: string(opts.ConflictPolicy),
				FileSize:       srcInfo.Size(),
				FileQuality:    fmt.Sprintf("%dp/%s/%s", incoming.Quality.Resolution, incoming.Quality.Source, incoming.Quality.Codec),
			})
		}

		switch decision.Action {
		case ActionSkip:
			p.logger.Info("reimport skipped",
				"source", sourcePath,
				"dest", destFile,
				"reason", decision.Reason,
			)
			return &ImportRecord{
				ID:         uuid.New().String(),
				MediaType:  mediaType,
				MediaID:    mediaID,
				SourcePath: sourcePath,
				DestPath:   destFile,
				ImportMode: p.importMode,
				Status:     StatusPending,
				Error:      "skipped: " + decision.Reason,
				ImportedAt: time.Now().UTC(),
			}, nil

		case ActionReplace:
			p.logger.Info("reimport replacing existing file",
				"source", sourcePath,
				"dest", destFile,
				"reason", decision.Reason,
			)
			if err := os.Remove(destFile); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("remove existing file: %w", err)
			}

		case ActionKeep:
			destFile = versionedPath(destFile)
			p.logger.Info("reimport keeping both files",
				"source", sourcePath,
				"dest", destFile,
			)
		}
	} else {
		decision = ConflictDecision{Action: ActionImport, Reason: "no existing file at destination"}
		if p.decisionLog != nil {
			_ = p.decisionLog.Log(ctx, ImportDecision{
				SourcePath:     sourcePath,
				DestPath:       destFile,
				MediaType:      string(mediaType),
				MediaID:        mediaID,
				Action:         string(ActionImport),
				Reason:         decision.Reason,
				ConflictPolicy: string(opts.ConflictPolicy),
				FileSize:       srcInfo.Size(),
			})
		}
	}

	// Perform the import
	if err := importFile(sourcePath, destFile, p.importMode); err != nil {
		_ = p.recordFailure(ctx, string(mediaType), mediaID, filepath.Base(sourcePath), sourcePath, err)
		return nil, fmt.Errorf("import file: %w", err)
	}

	// Update library
	if err := p.updateLibrary(ctx, match, destFile, sourcePath); err != nil {
		p.logger.Error("library update failed after reimport, cleaning up",
			"error", err, "dest", destFile)
		_ = os.Remove(destFile)
		return nil, fmt.Errorf("update library: %w", err)
	}

	// Record success
	record := &ImportRecord{
		ID:         uuid.New().String(),
		MediaType:  mediaType,
		MediaID:    mediaID,
		SourcePath: sourcePath,
		DestPath:   destFile,
		ImportMode: p.importMode,
		Status:     StatusImported,
		ImportedAt: time.Now().UTC(),
	}
	_ = p.recordStatus(ctx, string(mediaType), mediaID, sourcePath, destFile, StatusImported, "reimport: "+decision.Reason)

	p.publishNotification(ctx, match, destFile)

	return record, nil
}

// resolveMediaDestination determines the destination directory for a given media item.
func (p *ImportPipeline) resolveMediaDestination(ctx context.Context, mediaType MediaType, mediaID, sourcePath string) (*MatchResult, error) {
	switch mediaType {
	case MediaTypeMovie:
		movie, err := p.matcher.moviesSvc.GetMovie(ctx, mediaID)
		if err != nil {
			return nil, fmt.Errorf("get movie %s: %w", mediaID, err)
		}
		rootFolder, err := p.matcher.moviesSvc.GetRootFolder(ctx, movie.RootFolderID)
		if err != nil {
			return nil, fmt.Errorf("get root folder: %w", err)
		}
		destDir := filepath.Join(rootFolder.Path, sanitizeDirName(fmt.Sprintf("%s (%d)", movie.Title, movie.Year)))
		return &MatchResult{
			Matched:   true,
			MediaType: MediaTypeMovie,
			MediaID:   mediaID,
			Title:     movie.Title,
			Year:      movie.Year,
			DestPath:  destDir,
		}, nil

	case MediaTypeEpisode:
		// Find the episode by listing all episodes across seasons
		allSeries, err := p.matcher.seriesSvc.ListSeries(ctx)
		if err != nil {
			return nil, fmt.Errorf("list series: %w", err)
		}
		for _, s := range allSeries {
			seasons, err := p.matcher.seriesSvc.ListSeasons(ctx, s.ID)
			if err != nil {
				continue
			}
			for _, season := range seasons {
				episodes, err := p.matcher.seriesSvc.ListEpisodes(ctx, s.ID, &season.SeasonNumber)
				if err != nil {
					continue
				}
				for _, ep := range episodes {
					if ep.ID == mediaID {
						destDir := filepath.Join(
							sanitizeDirName(s.Title),
							fmt.Sprintf("Season %02d", season.SeasonNumber),
						)
						return &MatchResult{
							Matched:   true,
							MediaType: MediaTypeEpisode,
							MediaID:   mediaID,
							Title:     s.Title,
							Season:    season.SeasonNumber,
							Episode:   ep.EpisodeNumber,
							DestPath:  destDir,
						}, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("episode %s not found in any series", mediaID)

	default:
		return nil, fmt.Errorf("unknown media type: %s", mediaType)
	}
}
