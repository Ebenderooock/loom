//go:build !embed

package webui

import "io/fs"

// FS is nil when built without the embed tag (dev mode — use Vite).
var FS fs.FS
