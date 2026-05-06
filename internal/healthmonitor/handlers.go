package healthmonitor

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with health monitor endpoints.
func Router(m *Monitor) chi.Router {
	r := chi.NewRouter()
	r.Get("/", handleGetHealth(m))
	r.Post("/check", handleRunChecks(m))
	return r
}

// handleGetHealth returns the most recent check results without
// re-running the checks.
func handleGetHealth(m *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		results := m.LastResults()
		writeJSON(w, http.StatusOK, results)
	}
}

// handleRunChecks triggers an immediate health check and returns the
// results.
func handleRunChecks(m *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := m.RunChecks(r.Context())
		writeJSON(w, http.StatusOK, results)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
