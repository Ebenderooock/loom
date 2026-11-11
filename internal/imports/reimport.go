package imports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/movies"
)

// ReimportOptions controls reimport behaviour.
type ReimportOptions struct {
	ConflictPolicy ConflictPolicy `json:"conflict_policy"`
}

// ReimportFile forces a reimport of a source file to the library for the
// given media item, even if a file already exists at the destination.
func (p *ImportPipeline) ReimportFile(ctx context.Context, mediaType MediaType, mediaID, sourcePath string, opts ReimportOptions) (*ImportRecord, error) {
	if mediaType == MediaTypeMusic {
		if opts.ConflictPolicy != "" && opts.ConflictPolicy != ConflictReplaceIfBetter {
			return nil, fmt.Errorf("music reimport only supports replace_if_better")
		}
		return p.importMusicMatch(ctx, sourcePath, mediaID)
	}
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
	match, err := p.resolveMediaDestination(ctx, mediaType, mediaID)
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}

	destFile := filepath.Join(match.DestPath, filepath.Base(sourcePath))
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	destAbs, err := filepath.Abs(destFile)
	if err != nil {
		return nil, fmt.Errorf("resolve destination path: %w", err)
	}
	if sourceAbs == destAbs {
		return nil, fmt.Errorf("source is already at the library destination: %s", destFile)
	}

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
	mode := p.importMode
	if mode == ImportModeMove {
		mode = ImportModeHardlink
	}
	if err := importFile(sourcePath, destFile, mode); err != nil {
		_ = p.recordFailure(ctx, string(mediaType), mediaID, filepath.Base(sourcePath), sourcePath, err)
		return nil, fmt.Errorf("import file: %w", err)
	}

	// Update library
	if err := p.updateLibrary(ctx, match, destFile, sourcePath); err != nil {
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
	if err := p.recordStatus(ctx, string(mediaType), mediaID, sourcePath, destFile, StatusImported, "reimport: "+decision.Reason); err != nil {
		return nil, fmt.Errorf("record reimport (source preserved): %w", err)
	}
	if p.importMode == ImportModeMove {
		if err := p.FinalizeImports(ctx, []string{destFile}); err != nil {
			return nil, fmt.Errorf("reimport registered but source cleanup failed: %w", err)
		}
	}

	if match.MediaType == MediaTypeMovie && match.MediaID != "" {
		if err := p.moviesSvc.SetMovieStatus(ctx, match.MediaID, movies.MovieStatusAvailableRightQuality); err != nil {
			p.logger.Warn("failed to update movie status after reimport", "movie_id", match.MediaID, "error", err)
		}
	}
	p.publishNotification(ctx, match, destFile)

	return record, nil
}

// resolveMediaDestination determines the destination directory for a given media item.
func (p *ImportPipeline) resolveMediaDestination(ctx context.Context, mediaType MediaType, mediaID string) (*MatchResult, error) {
	return p.matcher.MatchExact(ctx, mediaType, mediaID)
}
