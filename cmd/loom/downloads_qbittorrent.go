package main

// This file exists to register the qBittorrent download client kind.
// It lives in its own file (rather than in cmd/loom/downloads.go) so
// that parallel agents adding other download kinds can do the same
// without merge conflicts on a shared blank-import block.
//
// See internal/downloads/qbittorrent/doc.go for behaviour details.

import _ "github.com/ebenderooock/loom/internal/downloads/qbittorrent"
