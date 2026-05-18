package imports

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// QualityProfileGetter retrieves a quality profile by ID.
type QualityProfileGetter interface {
	Get(ctx context.Context, id string) (*qualityprofiles.QualityProfile, error)
}

// UpgradeSpec rejects incoming files that are not a quality upgrade over
// the existing file, following the Radarr/Sonarr upgrade model.
type UpgradeSpec struct {
	profiles QualityProfileGetter
}

// NewUpgradeSpec creates an UpgradeSpec backed by the given profile store.
func NewUpgradeSpec(profiles QualityProfileGetter) *UpgradeSpec {
	return &UpgradeSpec{profiles: profiles}
}

func (s *UpgradeSpec) Name() string { return "UpgradeSpec" }

func (s *UpgradeSpec) IsSatisfiedBy(ctx context.Context, c *ImportCandidate) *ImportRejection {
	// No existing file — always allow (first import).
	if c.ExistingQuality == "" {
		return nil
	}

	// Need a quality profile to make a decision.
	if c.QualityProfileID == "" {
		return nil // no profile configured — allow
	}

	profile, err := s.profiles.Get(ctx, c.QualityProfileID)
	if err != nil || profile == nil {
		return nil // profile missing — allow rather than block
	}

	// Upgrades disabled at the profile level.
	if !profile.UpgradeAllowed {
		return &ImportRejection{
			Reason:  RejectionNotUpgrade,
			Message: fmt.Sprintf("upgrades are disabled on profile %q", profile.Name),
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
	if incomingQuality == "" {
		return nil // can't determine quality — allow
	}

	existingRank := profile.Rank(c.ExistingQuality)
	incomingRank := profile.Rank(incomingQuality)

	// If either quality is unknown to the profile, skip upgrade check.
	if existingRank < 0 || incomingRank < 0 {
		return nil
	}

	// Allow PROPER/REPACK of the same quality (revision upgrade).
	if incomingRank == existingRank {
		if c.IncomingRelease != nil && (c.IncomingRelease.IsProper || c.IncomingRelease.IsRepack) {
			return nil
		}
		return &ImportRejection{
			Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf(
				"incoming quality %q (rank %d) is not an upgrade over existing %q (rank %d)",
				incomingQuality, incomingRank, c.ExistingQuality, existingRank),
		}
	}

	// Higher rank index = better quality in the ordered items list.
	if incomingRank < existingRank {
		return &ImportRejection{
			Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf(
				"incoming quality %q (rank %d) is lower than existing %q (rank %d)",
				incomingQuality, incomingRank, c.ExistingQuality, existingRank),
		}
	}

	// Incoming rank > existing rank — upgrade allowed.
	return nil
}
