package server

import "net/http"

// requireFeature hides the wrapped routes (404) while enabled reports false.
func requireFeature(enabled func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled() {
				writeJSONNotFound(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"not found"}`))
}
