package imports

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ebenderooock/loom/internal/diskspace"
)

const minFreeSpaceBytes = 500 * 1024 * 1024 // 500MB minimum free space

// FreeSpaceSpec rejects imports when destination has insufficient disk space.
type FreeSpaceSpec struct{}

func (s *FreeSpaceSpec) Name() string { return "FreeSpace" }

func (s *FreeSpaceSpec) IsSatisfiedBy(_ context.Context, c *ImportCandidate) *ImportRejection {
	if c.DestPath == "" {
		return nil
	}

	dir := filepath.Dir(c.DestPath)

	_, available, err := diskspace.Get(dir)
	if err != nil {
		// If we can't stat, allow the import (will fail later with a better error)
		return nil
	}

	needed := uint64(c.FileSize) + minFreeSpaceBytes

	if available < needed {
		return &ImportRejection{
			Reason:  RejectionInsufficientSpace,
			Message: fmt.Sprintf("Not enough disk space: %dMB available, need %dMB", available/(1024*1024), needed/(1024*1024)),
		}
	}

	return nil
}
