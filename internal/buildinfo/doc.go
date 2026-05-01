// Package buildinfo carries build metadata (version, commit, build date)
// injected at link time via -ldflags -X. The values are surfaced through
// the loom version command and the GET /api/v1/system/status response.
//
// The canonical -X flags live in the project Makefile and
// .goreleaser.yaml. See docs/development.md for the build flow and
// docs/api.md for how the values appear on the wire.
package buildinfo
