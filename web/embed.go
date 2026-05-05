//go:build embed

// Package webui exports the embedded React SPA (web/dist/).
// Built only when the "embed" tag is active.
package webui

import "embed"

//go:embed dist/*
var FS embed.FS
