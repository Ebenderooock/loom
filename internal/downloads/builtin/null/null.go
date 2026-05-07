// Package null is a thin re-export package whose import side-effect
// guarantees the "builtin/null" download client kind is registered.
//
// The actual implementation lives inline in internal/downloads/kinds.go
// so the core package is self-sufficient at startup; this package
// exists for symmetry with future per-kind sub-packages
// (internal/downloads/qbittorrent, .../sabnzbd, etc.) that will follow
// the "import for side effect" idiom.
//
//	import _ "github.com/ebenderooock/loom/internal/downloads/builtin/null"
//
// is equivalent to the implicit registration that ships with the core
// downloads package and is therefore harmless to add.
package null

import (
	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind is the kind string this package registers under.
const Kind = downloads.KindNull
