package imports

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// QualityProfileGetter retrieves a quality profile by ID.
type QualityProfileGetter interface {
	Get(ctx context.Context, id string) (*qualityprofiles.QualityProfile, error)
}

// UpgradeSpec enforces profile acceptance and prevents quality downgrades.
type UpgradeSpec struct {
	profiles QualityProfileGetter
}

// NewUpgradeSpec creates an UpgradeSpec backed by the given profile store.
func NewUpgradeSpec(profiles QualityProfileGetter) *UpgradeSpec {
	return &UpgradeSpec{profiles: profiles}
}

func (s *UpgradeSpec) Name() string { return "UpgradeSpec" }

func (s *UpgradeSpec) IsSatisfiedBy(ctx context.Context, c *ImportCandidate) *ImportRejection {
	if c.QualityProfileID == "" {
		// In the absence of a profile, reject a downgrade only where both
		// filenames/records carry enough information to compare.
		return rejectKnownDowngrade(c)
	}

	if s.profiles == nil {
		return &ImportRejection{Reason: RejectionNotUpgrade, Message: "quality profile store is unavailable"}
	}
	profile, err := s.profiles.Get(ctx, c.QualityProfileID)
	if err != nil || profile == nil {
		return &ImportRejection{
			Reason:  RejectionNotUpgrade,
			Message: fmt.Sprintf("cannot load quality profile %q", c.QualityProfileID),
		}
	}

	// Determine the incoming quality name.
	incomingQuality := ""
	if c.IncomingRelease != nil {
		incomingQuality = qualityprofiles.QualityNameFromRelease(c.IncomingRelease)
	}
	if incomingQuality == "" {
		// Fall back to parsing the source filename.
		rel := parser.Parse(filepath.Base(c.SourcePath))
		incomingQuality = qualityprofiles.QualityNameFromRelease(rel)
	}
	incomingRank := profile.Rank(incomingQuality)
	if incomingRank < 0 {
		return &ImportRejection{Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf("incoming quality %q is not allowed by profile %q", incomingQuality, profile.Name)}
	}
	if c.ExistingQuality == "" {
		return nil
	}
	// A previously imported quality can be disallowed but still has a rank.
	items, err := profile.ParseItems()
	if err != nil {
		return &ImportRejection{Reason: RejectionNotUpgrade, Message: "invalid quality profile"}
	}
	existingRank := -1
	for i, item := range items {
		if strings.EqualFold(item.Name, c.ExistingQuality) {
			existingRank = i
			break
		}
	}
	if existingRank < 0 {
		return rejectKnownDowngrade(c)
	}
	// Profiles are ordered best first.
	if incomingRank > existingRank {
		return &ImportRejection{
			Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf(
				"incoming quality %q (rank %d) is lower than existing %q (rank %d)",
				incomingQuality, incomingRank, c.ExistingQuality, existingRank),
		}
	}

	if incomingRank < existingRank && !profile.UpgradeAllowed {
		return &ImportRejection{Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf("upgrades are disabled on profile %q", profile.Name)}
	}
	return nil
}

func rejectKnownDowngrade(c *ImportCandidate) *ImportRejection {
	existing := ParseFileQuality(c.ExistingQuality)
	incoming := ParseFileQuality(c.SourcePath)
	if existing.Resolution == 0 || incoming.Resolution == 0 {
		return nil
	}
	lower := incoming.Resolution < existing.Resolution
	if incoming.Resolution == existing.Resolution && incoming.Source != "" && existing.Source != "" {
		// Codec differences alone do not change the quality tier.
		incoming.Codec, existing.Codec = "", ""
		lower = qualityScore(incoming) < qualityScore(existing)
	}
	if lower {
		return &ImportRejection{Reason: RejectionNotUpgrade, Message: "incoming quality is lower than the existing quality"}
	}
	return nil
}
