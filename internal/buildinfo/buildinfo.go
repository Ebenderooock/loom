// Package buildinfo carries build metadata injected via -ldflags.
//
// All values are overwritten at build time. See the project Makefile and
// .goreleaser.yaml for the canonical -X flags.
package buildinfo

import (
	"fmt"
	"runtime"
)

// These are populated via -ldflags -X.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String renders a single-line summary suitable for `loom version`.
func String() string {
	return fmt.Sprintf("loom %s (commit %s, built %s, %s/%s, %s)",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
