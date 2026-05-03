package main

// Blank-import the Deluge download client kind for its registration
// side-effect. This file is intentionally separate from
// downloads.go so concurrent agents adding sibling kinds
// (qbittorrent, transmission, sabnzbd, ...) can each own their own
// file without colliding on a shared import block.
//
// See internal/downloads/deluge/doc.go for behaviour details.

import _ "github.com/loomctl/loom/internal/downloads/deluge"
