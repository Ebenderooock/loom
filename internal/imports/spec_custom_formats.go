package imports

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/parser"
)

// CustomFormatLister loads current rules so profile edits apply without restart.
type CustomFormatLister interface {
	List(context.Context) ([]customformats.CustomFormat, error)
}

// CustomFormatSpec enforces the assigned profile's minimum format score.
// Formats supplement base quality acceptance; they do not redefine quality tiers.
type CustomFormatSpec struct {
	profiles QualityProfileGetter
	formats  CustomFormatLister
}

// NewCustomFormatSpec creates an import check using live profile and format data.
func NewCustomFormatSpec(profiles QualityProfileGetter, formats CustomFormatLister) *CustomFormatSpec {
	return &CustomFormatSpec{profiles: profiles, formats: formats}
}

// Name identifies this import specification.
func (s *CustomFormatSpec) Name() string { return "CustomFormatSpec" }

// IsSatisfiedBy rejects files whose matching profile points fall below the
// minimum. Downloads Loom grabbed were scored by search, so a low score only
// adds a warning.
func (s *CustomFormatSpec) IsSatisfiedBy(ctx context.Context, c *ImportCandidate) *ImportRejection {
	rejection := s.check(ctx, c)
	if rejection != nil && c.GrabApproved {
		c.QualityWarnings = append(c.QualityWarnings, rejection.Message)
		return nil
	}
	return rejection
}

func (s *CustomFormatSpec) check(ctx context.Context, c *ImportCandidate) *ImportRejection {
	reject := func(message string) *ImportRejection {
		return &ImportRejection{Reason: RejectionCustomFormat, Message: message}
	}
	if c.QualityProfileID == "" {
		return nil
	}
	if s.profiles == nil {
		return reject("quality profile store is unavailable")
	}
	profile, err := s.profiles.Get(ctx, c.QualityProfileID)
	if err != nil || profile == nil {
		return reject(fmt.Sprintf("cannot load quality profile %q", c.QualityProfileID))
	}
	var matches []customformats.FormatMatch
	if len(profile.FormatItems) > 0 {
		if s.formats == nil {
			return reject("custom format store is unavailable")
		}
		formats, err := s.formats.List(ctx)
		if err != nil {
			return reject(fmt.Sprintf("cannot load custom formats: %v", err))
		}
		release := c.IncomingRelease
		if release == nil {
			release = parser.Parse(filepath.Base(c.SourcePath))
		}
		ri := customformats.FromParsedRelease(release)
		if ri.Title == "" {
			ri.Title = filepath.Base(c.SourcePath)
		}
		ri.Size = c.FileSize
		matches = customformats.NewEngine(formats).ScoreFileMetadata(ri, customformats.ExtractFileMetadata(c.SourcePath))
	}
	scores := make(map[string]int, len(profile.FormatItems))
	for _, item := range profile.FormatItems {
		scores[item.FormatID] = item.Score
	}
	total := customformats.ApplyScores(matches, scores)
	if total < profile.MinFormatScore {
		return reject(fmt.Sprintf("custom format score %d is below minimum %d for profile %q", total, profile.MinFormatScore, profile.Name))
	}
	return nil
}
