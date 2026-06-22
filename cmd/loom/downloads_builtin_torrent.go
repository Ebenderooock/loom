package main

// This file exists to register the builtin/torrent download client kind.
// The implementation talks to a Rain sidecar over JSON-RPC.
//
// See internal/downloads/torrent/doc.go for behaviour details.

import _ "github.com/ebenderooock/loom/internal/downloads/torrent"
