package apikeys

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey struct{}

// FromContext returns the APIKey from context, or nil.
func FromContext(ctx context.Context) *APIKey {
	k, _ := ctx.Value(contextKey{}).(*APIKey)
	return k
}

// Middleware returns chi-compatible middleware that validates the
// X-Api-Key header or ?apikey= query parameter against the store.
// If neither is present, the request proceeds without API-key context
// (other auth mechanisms may apply).
func Middleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extractKey(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			ak, err := store.ValidateKey(r.Context(), key)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"message": "invalid or expired api key"},
				})
				return
			}
			ctx := context.WithValue(r.Context(), contextKey{}, ak)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractKey(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("X-Api-Key")); k != "" {
		return k
	}
	if k := strings.TrimSpace(r.URL.Query().Get("apikey")); k != "" {
		return k
	}
	return ""
}
