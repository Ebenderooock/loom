package httpparam

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// Param returns a decoded route parameter. Chi matches on URL.RawPath when Go
// preserves one, such as for encodeURIComponent(":"), so its parameter is still
// escaped. Decoding only then avoids double-decoding literal percent signs.
func Param(r *http.Request, key string) string {
	value := chi.URLParam(r, key)
	if r.URL.RawPath == "" {
		return value
	}
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}
