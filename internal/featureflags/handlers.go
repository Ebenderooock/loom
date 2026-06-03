package featureflags

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for the feature flag endpoints. adminMW guards
// mutating routes; pass a no-op when auth is disabled.
func Router(svc *Service, adminMW func(http.Handler) http.Handler) chi.Router {
	if adminMW == nil {
		adminMW = func(next http.Handler) http.Handler { return next }
	}
	r := chi.NewRouter()
	r.Get("/", handleList(svc))
	r.With(adminMW).Put("/{key}", handleSet(svc))
	return r
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"features": svc.List()})
	}
}

func handleSet(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if err := svc.Set(r.Context(), key, body.Enabled); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"key": key, "enabled": body.Enabled})
	}
}
