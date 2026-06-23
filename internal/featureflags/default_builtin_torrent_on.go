//go:build !no_builtin_torrent

package featureflags

// builtinTorrentDefault is the registry default for the built-in torrent
// (Rain sidecar) flag. Normal builds default it ON so K8s/Docker behaviour is
// unchanged. Building with -tags no_builtin_torrent selects the OFF variant in
// default_builtin_torrent_off.go (single-binary distros such as Synology that
// have no Rain sidecar).
const builtinTorrentDefault = true
