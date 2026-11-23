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
		if pat == ".part" && !hasIncompletePartMarker(name) {
			continue
		}
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

func hasIncompletePartMarker(name string) bool {
	if strings.HasSuffix(name, ".part") {
		return true
	}
	for {
		_, rest, found := strings.Cut(name, ".part.")
		if !found {
			return false
		}
		// Part.1 is a numbered title or media part, not a transfer marker.
		// Keep checking so a later .part marker still blocks incomplete files.
		number, tail, separated := strings.Cut(rest, ".")
		if !separated || tail == "" || number == "" || strings.Trim(number, "0123456789") != "" {
			return true
		}
		name = rest
	}
}
