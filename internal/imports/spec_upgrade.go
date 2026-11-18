package imports

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ebenderooock/loom/internal/customformats"
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
	tiers    interface {
		ListTiers(context.Context) ([]customformats.Tier, error)
	}
}

// NewUpgradeSpec creates an UpgradeSpec backed by the given profile store.
func NewUpgradeSpec(profiles QualityProfileGetter, tiers ...interface {
	ListTiers(context.Context) ([]customformats.Tier, error)
}) *UpgradeSpec {
	s := &UpgradeSpec{profiles: profiles}
	if len(tiers) != 0 {
		s.tiers = tiers[0]
	}
	return s
}

func (s *UpgradeSpec) Name() string { return "UpgradeSpec" }

func (s *UpgradeSpec) IsSatisfiedBy(ctx context.Context, c *ImportCandidate) *ImportRejection {
	c.ResolvedTier = ""
	if c.QualityProfileID == "" {
		if strings.HasPrefix(c.ExistingQuality, "custom-tier:") {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "existing custom tier cannot be compared without a profile"}
		}
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
	release := c.IncomingRelease
	if release == nil {
		release = parser.Parse(filepath.Base(c.SourcePath))
	}
	if c.IncomingRelease != nil {
		incomingQuality = qualityprofiles.QualityNameFromRelease(c.IncomingRelease)
	}
	if incomingQuality == "" {
		// Fall back to parsing the source filename.
		rel := parser.Parse(filepath.Base(c.SourcePath))
		incomingQuality = qualityprofiles.QualityNameFromRelease(rel)
	}
	if customformats.HasCustomTiers(profile) {
		if s.tiers == nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "custom tier store is unavailable"}
		}
		tiers, err := s.tiers.ListTiers(ctx)
		if err != nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: fmt.Sprintf("cannot load custom tiers: %v", err)}
		}
		ri := customformats.FromParsedRelease(release)
		resolved, err := customformats.ResolveTier(profile, tiers, ri, customformats.BaseTierID(profile, incomingQuality))
		if err != nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: err.Error()}
		}
		if resolved == nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: fmt.Sprintf("incoming quality %q is not allowed by profile %q", incomingQuality, profile.Name)}
		}
		items, err := profile.ParseItems()
		if err != nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "invalid quality profile"}
		}
		if resolved.Kind == customformats.CustomKind {
			c.ResolvedTier = customformats.StoredTierID(resolved.ID)
		}
		if c.ExistingQuality == "" {
			return nil
		}
		existingRank := -1
		for i, item := range items {
			if (item.Kind == customformats.CustomKind && customformats.StoredTierID(item.ID) == c.ExistingQuality) ||
				(item.Kind == "" && (strings.EqualFold(item.Name, c.ExistingQuality) ||
					(strings.EqualFold(item.Name, "unknown") && strings.HasPrefix(strings.ToLower(c.ExistingQuality), "unknown-")))) {
				existingRank = i
				break
			}
		}
		if existingRank < 0 {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "existing quality tier cannot be compared"}
		}
		if resolved.Rank > existingRank {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "incoming quality is lower than the existing quality"}
		}
		if resolved.Rank < existingRank && !profile.UpgradeAllowed {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "upgrades are disabled"}
		}
		cutoff := profile.CutoffRank()
		if cutoff >= 0 && items[cutoff].Allowed && existingRank <= cutoff {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "quality cutoff already met"}
		}
		return nil
	}
	if strings.HasPrefix(c.ExistingQuality, "custom-tier:") {
		return &ImportRejection{Reason: RejectionNotUpgrade, Message: "existing custom tier is not present in the profile"}
	}
	// A known resolution does not make an unknown source a different policy tier.
	// Keep this fallback limited to first imports, never quality replacements.
	unknownSource := incomingQuality == "" || strings.HasPrefix(incomingQuality, "unknown-")
	if unknownSource && c.ExistingQuality == "" &&
		(profile.IsAllowed("unknown") || profile.AllowsUnknownQuality()) {
		return nil
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
	if c.IncomingRelease != nil {
		if incoming.Resolution == 0 {
			incoming.Resolution = c.IncomingRelease.Resolution
		}
		if incoming.Source == "" {
			incoming.Source = strings.ToLower(c.IncomingRelease.Source)
		}
	}
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
