package customformats

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// CustomKind distinguishes a matcher-backed item from a built-in tier.
const CustomKind = "custom"

// StoredTierID encodes an immutable tier identity in existing quality fields.
func StoredTierID(id string) string { return "custom-tier:" + id }

// StoredID extracts a persisted custom tier identity.
func StoredID(quality string) string { return strings.TrimPrefix(quality, "custom-tier:") }

// HasCustomTiers reports whether a profile references custom matchers.
func HasCustomTiers(profile *qualityprofiles.QualityProfile) bool {
	if profile == nil {
		return false
	}
	items, err := profile.ParseItems()
	if err != nil {
		return false
	}
	for _, item := range items {
		if item.Kind == CustomKind {
			return true
		}
	}
	return false
}

// BaseTierID looks up the parsed, actual source/resolution in the profile.
func BaseTierID(profile *qualityprofiles.QualityProfile, quality string) string {
	if profile == nil {
		return ""
	}
	if quality == "" || strings.HasPrefix(quality, "unknown-") {
		quality = "unknown"
	}
	items, err := profile.ParseItems()
	if err != nil {
		return ""
	}
	for _, item := range items {
		if item.Kind == "" && strings.EqualFold(item.Name, quality) {
			return item.ID
		}
	}
	return ""
}

// Tier is a reusable matcher-backed quality tier. ID is its stable identity;
// Name is only a display label.
type Tier struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Specifications []Specification `json:"specifications"`
}

// Resolution describes the selected item. Rank is zero-based, best first.
type Resolution struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Rank int    `json:"rank"`
	Kind string `json:"kind,omitempty"`
}

// Validate rejects rules which cannot be evaluated consistently during both
// search and import. In particular, indexer and file-only evidence is not
// necessarily available at both stages.
func (t Tier) Validate() error {
	if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("tier id and name are required")
	}
	if len(t.Specifications) == 0 {
		return fmt.Errorf("at least one specification is required")
	}
	for _, spec := range t.Specifications {
		switch spec.Implementation {
		case ImplReleaseTitle, ImplQuality,
			ImplSource, ImplResolution,
			ImplCodec, ImplAudio,
			ImplReleaseGroup, ImplLanguage:
		default:
			return fmt.Errorf("unsupported tier specification %q", spec.Implementation)
		}
		value, ok := spec.Fields["value"].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s requires a non-empty value", spec.Implementation)
		}
		if spec.Implementation == ImplReleaseTitle {
			if _, err := regexp.Compile("(?i)" + value); err != nil {
				return fmt.Errorf("invalid release title expression: %w", err)
			}
		}
	}
	return nil
}

// ResolveTier selects the first allowed matching item in profile order.
// baseID identifies the actual known-source built-in tier, if any; callers must
// never infer a source from resolution when obtaining it. An explicit exclusion
// of that built-in tier takes precedence over custom matches.
// Invalid or dangling custom references fail closed.
func ResolveTier(profile *qualityprofiles.QualityProfile, tiers []Tier, release ReleaseInfo, baseID string) (*Resolution, error) {
	if profile == nil {
		return nil, fmt.Errorf("quality profile is required")
	}
	items, err := profile.ParseItems()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Tier, len(tiers))
	for _, tier := range tiers {
		if _, exists := byID[tier.ID]; exists {
			return nil, fmt.Errorf("duplicate custom tier %q", tier.ID)
		}
		byID[tier.ID] = tier
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.ID == "" || seen[item.ID] {
			return nil, fmt.Errorf("profile tier IDs must be non-empty and unique")
		}
		seen[item.ID] = true
		if item.Kind != "" && item.Kind != CustomKind {
			return nil, fmt.Errorf("unknown tier kind %q", item.Kind)
		}
		if item.Kind == CustomKind {
			tier, ok := byID[item.ID]
			if !ok {
				return nil, fmt.Errorf("custom tier %q not found", item.ID)
			}
			if err := tier.Validate(); err != nil {
				return nil, fmt.Errorf("invalid custom tier %q: %w", tier.ID, err)
			}
		}
		knownSource := strings.TrimSpace(release.Source) != "" && !strings.EqualFold(strings.TrimSpace(release.Source), "unknown")
		if knownSource && item.Kind == "" && item.ID == baseID && !item.Allowed {
			return nil, nil
		}
	}
	for rank, item := range items {
		if !item.Allowed {
			continue
		}
		if item.Kind == "" {
			if baseID != "" && item.ID == baseID {
				return &Resolution{ID: item.ID, Name: item.Name, Rank: rank}, nil
			}
			continue
		}
		tier := byID[item.ID]
		engine := NewEngine([]CustomFormat{{
			ID: tier.ID, Name: tier.Name, Specifications: tier.Specifications,
		}})
		if len(engine.ScoreRelease(release)) != 0 {
			return &Resolution{ID: tier.ID, Name: tier.Name, Rank: rank, Kind: CustomKind}, nil
		}
	}
	return nil, nil
}
