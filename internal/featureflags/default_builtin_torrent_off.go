//go:build no_builtin_torrent

package featureflags

// builtinTorrentDefault is the registry default for the built-in torrent
// (Rain sidecar) flag. This OFF variant is selected by -tags no_builtin_torrent
// so single-binary distributions (e.g. Synology spksrc) default the built-in
// torrent OFF. The ON variant lives in default_builtin_torrent_on.go.
const builtinTorrentDefault = false
