package customformats

import (
	"strings"

	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// QualityDecision is the outcome of the one quality decision search and
// import share. A nil Resolution means the profile does not accept the release.
type QualityDecision struct {
	Resolution *Resolution
	Definition *movies.QualityDefinition // matched built-in definition, if any
	Assumed    bool                      // the source was assumed: the release name states none
	Stored     string                    // identity persisted on media files
	Detected   string                    // literal parse, e.g. "unknown-1080p"
}

// ResolveQuality detects a release's quality and places it in the profile.
// Search runs it on the release title at grab time; import reuses that result
// and only runs it itself for downloads that search did not grab.
func ResolveQuality(profile *qualityprofiles.QualityProfile, tiers []Tier, release *parser.Release, ri ReleaseInfo, defs []*movies.QualityDefinition) (QualityDecision, error) {
	d := QualityDecision{}
	if release == nil {
		return d, nil
	}
	d.Detected = qualityprofiles.QualityNameFromRelease(release)
	if len(defs) == 0 {
		defs = qualityprofiles.DefinitionsFromProfile(profile)
	}
	def, assumed := qualityprofiles.MatchDefinition(release, defs)
	d.Definition = def
	items, err := profile.ParseItems()
	if err != nil {
		return d, err
	}
	builtInID := func(def *movies.QualityDefinition) string {
		if def == nil {
			return ""
		}
		for _, item := range items {
			if item.Kind == "" && (item.ID == def.ID || strings.EqualFold(item.Name, def.Name)) {
				return item.ID
			}
		}
		return ""
	}

	if !HasCustomTiers(profile) {
		id := builtInID(def)
		for rank, item := range items {
			if item.ID == id && item.Allowed {
				d.Resolution = &Resolution{ID: item.ID, Name: item.Name, Rank: rank}
				d.Assumed, d.Stored = assumed, def.Name
			}
		}
		return d, nil
	}

	// A detected source wins; an unknown source first tries an explicit
	// Unknown tier and custom tiers, then the shared assumption.
	baseID := BaseTierID(profile, d.Detected)
	if def != nil && !assumed {
		baseID = builtInID(def)
	}
	res, err := ResolveTier(profile, tiers, ri, baseID)
	if err != nil {
		return d, err
	}
	if res == nil && def != nil && assumed {
		if res, err = ResolveTier(profile, tiers, ri, builtInID(def)); err != nil {
			return d, err
		}
		d.Assumed = res != nil && res.Kind == ""
	}
	d.Resolution = res
	switch {
	case res == nil:
	case res.Kind == CustomKind:
		d.Stored = StoredTierID(res.ID)
	case d.Assumed:
		d.Stored = def.Name
	case strings.EqualFold(res.Name, "unknown"):
		d.Stored = d.Detected
	default:
		d.Stored = res.Name
	}
	return d, nil
}
