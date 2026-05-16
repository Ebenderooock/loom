package imports

import (
	"context"
	"path/filepath"
	"strings"
)

// UnpackingSpec rejects files that appear to still be unpacking.
type UnpackingSpec struct{}

func (s *UnpackingSpec) Name() string { return "NotUnpacking" }

func (s *UnpackingSpec) IsSatisfiedBy(_ context.Context, c *ImportCandidate) *ImportRejection {
	name := strings.ToLower(filepath.Base(c.SourcePath))
	dir := strings.ToLower(filepath.Base(filepath.Dir(c.SourcePath)))

	unpackPatterns := []string{".!qb", ".part", "_unpack", ".downloading"}
	for _, pat := range unpackPatterns {
		if strings.HasSuffix(name, pat) || strings.Contains(name, pat+".") {
			return &ImportRejection{
				Reason:  RejectionUnpacking,
				Message: "File appears to still be unpacking",
			}
		}
	}

	// Check parent directory for unpack markers
	if strings.HasPrefix(dir, "_unpack") || strings.HasPrefix(dir, "_unrarred") {
		return &ImportRejection{
			Reason:  RejectionUnpacking,
			Message: "File is in an unpacking directory",
		}
	}

	return nil
}
