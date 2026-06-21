package main

// This file exists to register the built-in torrent download client
// kind. It lives in its own file (rather than in cmd/loom/downloads.go)
// so that parallel agents adding other download kinds can do the same
// without merge conflicts on a shared blank-import block.
//
// See internal/downloads/torrent/doc.go for behaviour details.

import _ "github.com/ebenderooock/loom/internal/downloads/torrent"
