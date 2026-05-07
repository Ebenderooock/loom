package main

// Blank-import the NZBGet download client kind for its registration
// side-effect. This file is intentionally separate from
// downloads.go so concurrent agents adding sibling kinds
// (qbittorrent, sabnzbd, transmission, deluge, ...) can each own
// their own file without colliding on a shared import block.
import (
	_ "github.com/ebenderooock/loom/internal/downloads/nzbget"
)
