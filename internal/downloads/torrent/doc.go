// Package torrent implements the "builtin/torrent" download client
// kind. It embeds the anacrolix/torrent library directly into the
// Loom process, providing a zero-configuration BitTorrent engine
// that requires no external daemon.
//
// A single Engine is shared across all Definitions of kind
// "builtin/torrent" within a Loom process. Each Definition may
// differ in category, save path, and seed policy, but they all
// share the same listen port, connection pool, and piece cache.
//
// Importing the package for its side-effect registers the kind with
// the downloads core:
//
//	import _ "github.com/ebenderooock/loom/internal/downloads/torrent"
//
// The Client type satisfies downloads.DownloadClient and is normally
// constructed by the package factory rather than directly.
package torrent
