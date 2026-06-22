// Package torrent implements the "builtin/torrent" download client kind.
// It talks to a co-located Rain sidecar via JSON-RPC instead of embedding
// a BitTorrent engine directly in the Loom process.
//
// Importing the package for its side-effect registers the kind with
// the downloads core:
//
//	import _ "github.com/ebenderooock/loom/internal/downloads/torrent"
//
// The Client type satisfies downloads.DownloadClient and is normally
// constructed by the package factory rather than directly.
package torrent
