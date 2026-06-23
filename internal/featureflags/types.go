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

	// KeyPlugins toggles custom post-processing plugins (scripts run on events).
	// Disabled by default because plugins execute arbitrary commands as the
	// server process; an admin must opt in. When disabled the runner does not
	// execute any plugin, though definitions and history remain manageable.
	KeyPlugins = "plugins"

	// KeyMusic toggles the Music capability (artists/albums/tracks, music
	// scanning and acquisition). Disabled by default while the feature is
	// incomplete: when off, the Music API routes are not mounted. Flip on once
	// the Music milestones and UI are ready.
	KeyMusic = "music"

	// KeyBuiltinTorrent toggles the built-in torrent ("Rain sidecar") download
	// kind. Its default comes from a build-tag-controlled value
	// (builtinTorrentDefault): ON for normal builds (K8s/Docker) and OFF when
	// built with -tags no_builtin_torrent (single-binary distros such as
	// Synology that ship no Rain sidecar). When off, the builtin/torrent kind is
	// hidden in the UI and rejected by the download-client handlers.
	KeyBuiltinTorrent = "builtin_torrent"
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
	{
		Key:         KeyPlugins,
		Label:       "Plugins (Custom Scripts)",
		Description: "Run admin-defined custom scripts when events fire (grab, import, playback). Plugins execute arbitrary commands as the Loom server process — this is NOT a security sandbox. Disabled by default; enable only if you trust the configured scripts.",
		Category:    "Diagnostics",
		Default:     false,
	},
	{
		Key:         KeyMusic,
		Label:       "Music",
		Description: "Enable the Music capability: manage artists, albums and tracks, scan music libraries, acquire releases, and request artists via the requests portal and chat bots. When off, the Music API is not exposed.",
		Category:    "Media",
		Default:     true,
	},
	{
		Key:         KeyBuiltinTorrent,
		Label:       "Built-in Torrent (Rain)",
		Description: "Offer the built-in torrent download kind, which talks to a Rain sidecar over its RPC endpoint (deployed alongside Loom by the Helm chart). When off, the built-in torrent kind is hidden in the UI and rejected by the download-client API. Default is build-dependent: on for container builds, off for single-binary distributions built without a Rain sidecar.",
		Category:    "Downloads",
		Default:     builtinTorrentDefault,
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
