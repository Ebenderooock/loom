package imports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/downloads"
)

// ReimportOptions controls reimport behaviour.
type ReimportOptions struct {
	ConflictPolicy ConflictPolicy `json:"conflict_policy"`
}

// ReimportFile imports to the explicitly selected media item. Even an explicit
// replacement uses the normal profile, downgrade and recovery checks.
func (p *ImportPipeline) ReimportFile(ctx context.Context, mediaType MediaType, mediaID, sourcePath string, opts ReimportOptions) (*ImportRecord, error) {
	if mediaType == MediaTypeMusic {
		if opts.ConflictPolicy != "" && opts.ConflictPolicy != ConflictReplaceIfBetter {
			return nil, fmt.Errorf("music reimport only supports replace_if_better")
		}
		return p.importMusicMatch(ctx, sourcePath, mediaID)
	}
	switch opts.ConflictPolicy {
	case "", ConflictSkip, ConflictReplaceIfBetter, ConflictReplaceAlways, ConflictKeepBoth:
	default:
		return nil, fmt.Errorf("unknown conflict policy %q", opts.ConflictPolicy)
	}
	if err := checkRegularSource(sourcePath); err != nil {
		return nil, err
	}
	match, err := p.resolveMediaDestination(ctx, mediaType, mediaID)
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}
	name := filepath.Base(sourcePath)
	dest := filepath.Join(match.DestPath, name)
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, err
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	if sourceAbs == destAbs {
		return nil, fmt.Errorf("source is already at the library destination: %s", dest)
	}
	if _, err := os.Lstat(dest); err == nil {
		if opts.ConflictPolicy == ConflictSkip {
			return &ImportRecord{ID: uuid.New().String(), MediaType: mediaType, MediaID: mediaID,
				SourcePath: sourcePath, DestPath: dest, ImportMode: p.importMode, Status: StatusPending,
				Error: "skipped: policy is skip", ImportedAt: time.Now().UTC()}, nil
		}
		if opts.ConflictPolicy == ConflictKeepBoth {
			dest = versionedPath(dest)
			name = filepath.Base(dest)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	dest, err = p.importMatchedWith(ctx, &downloads.DownloadCompletedEvent{Title: filepath.Base(sourcePath)}, match, sourcePath, name, true)
	if err != nil {
		return nil, err
	}
	return &ImportRecord{ID: uuid.New().String(), MediaType: mediaType, MediaID: mediaID,
		SourcePath: sourcePath, DestPath: dest, ImportMode: p.importMode, Status: StatusImported, ImportedAt: time.Now().UTC()}, nil
}

func (p *ImportPipeline) resolveMediaDestination(ctx context.Context, mediaType MediaType, mediaID string) (*MatchResult, error) {
	return p.matcher.MatchExact(ctx, mediaType, mediaID)
}
