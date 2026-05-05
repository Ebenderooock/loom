package anime

import (
	"sort"
	"strings"
)

// Tier constants for release group quality scoring.
const (
	Tier1Score = 100 // Premium encodes
	Tier2Score = 75  // Standard reliable groups
	Tier3Score = 50  // Quick/acceptable releases
)

// defaultGroups defines known anime release groups by quality tier.
var defaultGroups = map[string]ReleaseGroup{
	// Tier 1 — best quality encodes
	"Kametsu":   {Name: "Kametsu", Score: Tier1Score},
	"EMBER":     {Name: "EMBER", Score: Tier1Score},
	"Cleo":      {Name: "Cleo", Score: Tier1Score},
	"Nep_Blanc": {Name: "Nep_Blanc", Score: Tier1Score},
	"YURASUKA":  {Name: "YURASUKA", Score: Tier1Score},

	// Tier 2 — good reliable groups
	"SubsPlease": {Name: "SubsPlease", Score: Tier2Score},
	"Erai-raws":  {Name: "Erai-raws", Score: Tier2Score},
	"Judas":      {Name: "Judas", Score: Tier2Score},
	"Tsundere":   {Name: "Tsundere", Score: Tier2Score},

	// Tier 3 — acceptable quick releases
	"ASW":       {Name: "ASW", Score: Tier3Score},
	"ToonsHub":  {Name: "ToonsHub", Score: Tier3Score},
	"Anime Time": {Name: "Anime Time", Score: Tier3Score},
	"SUGOI":     {Name: "SUGOI", Score: Tier3Score},
}

// ScoreGroup returns the quality score for a release group name.
// Custom scoring from prefs overrides defaults. Unknown groups score 0.
func ScoreGroup(name string, customScoring map[string]int) int {
	if customScoring != nil {
		// Case-insensitive lookup in custom scoring
		for k, v := range customScoring {
			if strings.EqualFold(k, name) {
				return v
			}
		}
	}
	// Case-insensitive lookup in defaults
	for k, g := range defaultGroups {
		if strings.EqualFold(k, name) {
			return g.Score
		}
	}
	return 0
}

// DefaultGroups returns a copy of the built-in release group registry
// sorted by score descending.
func DefaultGroups() []ReleaseGroup {
	groups := make([]ReleaseGroup, 0, len(defaultGroups))
	for _, g := range defaultGroups {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Score != groups[j].Score {
			return groups[i].Score > groups[j].Score
		}
		return groups[i].Name < groups[j].Name
	})
	return groups
}
