package imports

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// QualityProfileGetter retrieves a quality profile by ID.
type QualityProfileGetter interface {
	Get(ctx context.Context, id string) (*qualityprofiles.QualityProfile, error)
}

// QualityDefinitionLister lists the built-in quality definitions.
type QualityDefinitionLister interface {
	ListQualityDefinitions(ctx context.Context) ([]*movies.QualityDefinition, error)
}

// UpgradeSpec enforces profile acceptance and prevents quality downgrades.
// It reuses the quality search decided at grab time and only detects quality
// itself, with the detector search uses, for downloads search did not grab.
type UpgradeSpec struct {
	profiles QualityProfileGetter
	tiers    interface {
		ListTiers(context.Context) ([]customformats.Tier, error)
	}
	defs QualityDefinitionLister
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

// SetDefinitions supplies the quality definitions search matches against.
// Without them, definitions are derived from the profile's item names.
func (s *UpgradeSpec) SetDefinitions(defs QualityDefinitionLister) { s.defs = defs }

func (s *UpgradeSpec) Name() string { return "UpgradeSpec" }

// IsSatisfiedBy checks the profile for downloads Loom did not grab. A grabbed
// download already passed search's profile check, and a manual one is the
// user's choice, so their quality problems become warnings, not rejections.
func (s *UpgradeSpec) IsSatisfiedBy(ctx context.Context, c *ImportCandidate) *ImportRejection {
	rejection := s.check(ctx, c)
	if !c.GrabApproved {
		return rejection
	}
	if rejection != nil {
		c.QualityWarnings = append(c.QualityWarnings, rejection.Message)
		if c.ResolvedTier == "" {
			c.ResolvedTier = fallbackQuality(c)
		}
		return nil
	}
	if c.GrabbedManually {
		if warning := s.cutoffWarning(ctx, c); warning != "" {
			c.QualityWarnings = append(c.QualityWarnings, warning)
		}
	}
	return nil
}

// fallbackQuality names a grabbed file's quality when the profile cannot place it.
func fallbackQuality(c *ImportCandidate) string {
	if g := c.GrabbedQuality; g != "" && customformats.StoredID(g) == g {
		return g
	}
	release := c.IncomingRelease
	if release == nil {
		release = parser.Parse(filepath.Base(c.SourcePath))
	}
	return qualityprofiles.QualityNameFromRelease(release)
}

// cutoffWarning reports a manually grabbed file that falls short of the cutoff.
func (s *UpgradeSpec) cutoffWarning(ctx context.Context, c *ImportCandidate) string {
	if c.QualityProfileID == "" || s.profiles == nil {
		return ""
	}
	profile, err := s.profiles.Get(ctx, c.QualityProfileID)
	if err != nil || profile == nil {
		return ""
	}
	items, err := profile.ParseItems()
	cutoff := profile.CutoffRank()
	if err != nil || cutoff < 0 {
		return ""
	}
	quality := c.ResolvedTier
	if quality == "" {
		quality = fallbackQuality(c)
	}
	if rank := qualityItemRank(items, quality); rank >= 0 && rank <= cutoff {
		return ""
	}
	return fmt.Sprintf("quality %q does not meet the cutoff %q of profile %q", quality, items[cutoff].Name, profile.Name)
}

func (s *UpgradeSpec) check(ctx context.Context, c *ImportCandidate) *ImportRejection {
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
	items, err := profile.ParseItems()
	if err != nil {
		return &ImportRejection{Reason: RejectionNotUpgrade, Message: "invalid quality profile"}
	}
	custom := customformats.HasCustomTiers(profile)

	release := c.IncomingRelease
	if release == nil {
		release = parser.Parse(filepath.Base(c.SourcePath))
	}
	incomingQuality := qualityprofiles.QualityNameFromRelease(release)

	var tiers []customformats.Tier
	if custom {
		if s.tiers == nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "custom tier store is unavailable"}
		}
		if tiers, err = s.tiers.ListTiers(ctx); err != nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: fmt.Sprintf("cannot load custom tiers: %v", err)}
		}
	}
	ri := customformats.FromParsedRelease(release)
	grabbed := c.GrabbedQuality
	// A grabbed custom tier holds only while the file still matches it, for
	// example an x264 episode inside a pack grabbed as an x265 tier does not.
	if id := customformats.StoredID(grabbed); id != grabbed && !customTierMatches(tiers, id, ri) {
		grabbed = ""
	}

	rank, stored := -1, ""
	if grabbed != "" {
		incomingQuality = grabbed
		rank = qualityItemRank(items, grabbed)
		if rank < 0 || !items[rank].Allowed {
			return &ImportRejection{Reason: RejectionNotUpgrade,
				Message: fmt.Sprintf("grabbed quality %q is no longer allowed by profile %q", grabbed, profile.Name)}
		}
		stored = grabbed
	} else {
		var defs []*movies.QualityDefinition
		if s.defs != nil {
			if defs, err = s.defs.ListQualityDefinitions(ctx); err != nil {
				return &ImportRejection{Reason: RejectionNotUpgrade, Message: fmt.Sprintf("cannot load quality definitions: %v", err)}
			}
		}
		decision, err := customformats.ResolveQuality(profile, tiers, release, ri, defs)
		if err != nil {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: err.Error()}
		}
		if decision.Resolution != nil {
			rank, stored = decision.Resolution.Rank, decision.Stored
		}
	}
	if rank < 0 {
		// A known resolution does not make an unknown source a different policy tier.
		// Keep this fallback limited to first imports, never quality replacements.
		unknownSource := incomingQuality == "" || strings.HasPrefix(incomingQuality, "unknown-")
		if !custom && unknownSource && c.ExistingQuality == "" &&
			(profile.IsAllowed("unknown") || profile.AllowsUnknownQuality()) {
			return nil
		}
		return &ImportRejection{Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf("incoming quality %q is not allowed by profile %q", incomingQuality, profile.Name)}
	}
	c.ResolvedTier = stored
	if c.ExistingQuality == "" {
		return nil
	}
	if !custom && strings.HasPrefix(c.ExistingQuality, "custom-tier:") {
		return &ImportRejection{Reason: RejectionNotUpgrade, Message: "existing custom tier is not present in the profile"}
	}
	// A previously imported quality can be disallowed but still has a rank.
	existingRank := qualityItemRank(items, c.ExistingQuality)
	if existingRank < 0 {
		if custom {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "existing quality tier cannot be compared"}
		}
		return rejectKnownDowngrade(c)
	}
	// Profiles are ordered best first.
	if rank > existingRank {
		return &ImportRejection{
			Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf(
				"incoming quality %q (rank %d) is lower than existing %q (rank %d)",
				incomingQuality, rank, c.ExistingQuality, existingRank),
		}
	}
	if rank < existingRank && !profile.UpgradeAllowed {
		return &ImportRejection{Reason: RejectionNotUpgrade,
			Message: fmt.Sprintf("upgrades are disabled on profile %q", profile.Name)}
	}
	// The Unknown tier spans resolutions, so its rank alone cannot rule out
	// replacing an unknown-1080p file with a 720p one.
	if strings.HasPrefix(strings.ToLower(c.ExistingQuality), "unknown-") {
		if r := rejectKnownDowngrade(c); r != nil {
			return r
		}
	}
	if custom {
		cutoff := profile.CutoffRank()
		if cutoff >= 0 && items[cutoff].Allowed && existingRank <= cutoff {
			return &ImportRejection{Reason: RejectionNotUpgrade, Message: "quality cutoff already met"}
		}
	}
	return nil
}

func customTierMatches(tiers []customformats.Tier, id string, ri customformats.ReleaseInfo) bool {
	for _, tier := range tiers {
		if tier.ID == id {
			engine := customformats.NewEngine([]customformats.CustomFormat{{
				ID: tier.ID, Name: tier.Name, Specifications: tier.Specifications,
			}})
			return len(engine.ScoreRelease(ri)) != 0
		}
	}
	return false
}

// qualityItemRank finds a stored quality identity in the profile.
func qualityItemRank(items []qualityprofiles.QualityItem, quality string) int {
	for i, item := range items {
		if (item.Kind == customformats.CustomKind && customformats.StoredTierID(item.ID) == quality) ||
			(item.Kind == "" && (strings.EqualFold(item.Name, quality) ||
				(strings.EqualFold(item.Name, "unknown") && strings.HasPrefix(strings.ToLower(quality), "unknown-")))) {
			return i
		}
	}
	return -1
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
