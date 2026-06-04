// Package featureflags provides a small registry-backed feature toggle
// system. Each feature has an in-code definition (key, label, default) and an
// optional persisted override stored in the feature_flags table. The current
// value of a flag is its override if one has been set, otherwise the
// registry default.
package featureflags

// Keys for known feature flags. Use these constants instead of raw strings.
const (
	// KeySearchLog toggles the Search Log (a.k.a. search debug log / search
	// queue). When disabled the autosearch engine stops recording new search
	// trace entries; historical entries remain readable.
	KeySearchLog = "search_log"

	// KeyMediaAnalytics toggles media-server analytics sampling. When disabled
	// the poller stops collecting new play history; existing history and the
	// reports remain readable.
	KeyMediaAnalytics = "media_analytics"
)

// FlagDef is the static, in-code definition of a feature toggle.
type FlagDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Default     bool   `json:"default"`
}

// Flag is a FlagDef combined with its currently effective value.
type Flag struct {
	FlagDef
	Enabled bool `json:"enabled"`
}

// Definitions is the registry of all known feature flags. Adding a flag here
// makes it appear in the API/UI automatically with its default value.
var Definitions = []FlagDef{
	{
		Key:         KeySearchLog,
		Label:       "Search Log",
		Description: "Record detailed per-search traces (indexer results, rejection reasons, grabs) shown under Search Queue. Disabling stops new traces; existing history stays viewable.",
		Category:    "Diagnostics",
		Default:     true,
	},
	{
		Key:         KeyMediaAnalytics,
		Label:       "Media Analytics",
		Description: "Monitor active streams and record watch history from connected Plex/Emby/Jellyfin servers, shown under Analytics. Disabling stops new sampling; existing history stays viewable.",
		Category:    "Diagnostics",
		Default:     true,
	},
}

// definition returns the FlagDef for a key and whether it is known.
func definition(key string) (FlagDef, bool) {
	for _, d := range Definitions {
		if d.Key == key {
			return d, true
		}
	}
	return FlagDef{}, false
}
