package imports

import (
	"context"
	"path/filepath"
	"strings"
)

// SampleSpec rejects files that appear to be samples.
type SampleSpec struct{}

func (s *SampleSpec) Name() string { return "NotSample" }

func (s *SampleSpec) IsSatisfiedBy(_ context.Context, c *ImportCandidate) *ImportRejection {
	name := strings.ToLower(filepath.Base(c.SourcePath))

	samplePatterns := []string{"sample", "-sample.", "_sample.", ".sample."}
	for _, pat := range samplePatterns {
		if strings.Contains(name, pat) {
			return &ImportRejection{
				Reason:  RejectionSample,
				Message: "File appears to be a sample",
			}
		}
	}

	// Size-based detection for movies (< 100MB is likely a sample)
	if c.Match != nil && c.Match.MediaType == MediaTypeMovie && c.FileSize > 0 && c.FileSize < 100*1024*1024 {
		return &ImportRejection{
			Reason:  RejectionSample,
			Message: "File is too small to be a movie (< 100MB)",
		}
	}

	return nil
}
