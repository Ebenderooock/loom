//go:build !embed

package server

import "net/http"

// spaHandler returns nil when the binary is built without the embed tag.
// In development the Vite dev server handles the frontend on :5173.
func spaHandler() http.Handler { return nil }

func hasSPA() bool { return false }
