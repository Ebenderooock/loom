//go:build embed

package server

import (
	"io/fs"
	"net/http"
	"strings"

	webui "github.com/loomctl/loom/web"
)

// spaHandler serves the embedded React SPA. Static assets (JS, CSS, images)
// are served directly from the embedded filesystem; all other paths fall
// through to index.html so the client-side router can handle them.
func spaHandler() http.Handler {
	sub, err := fs.Sub(webui.FS, "dist")
	if err != nil {
		panic("server: embedded web/dist sub-filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to open the file; if it exists serve it with proper caching.
		if f, err := sub.Open(path); err == nil {
			f.Close()
			// Hashed asset filenames get long cache; HTML gets revalidation.
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// File not found → serve index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func hasSPA() bool { return true }
