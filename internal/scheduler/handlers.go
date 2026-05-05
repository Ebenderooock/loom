package scheduler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for the rolling-search API endpoints.
func Router(rs *RollingSearcher) chi.Router {
	r := chi.NewRouter()
	r.Get("/status", handleStatus(rs))
	r.Post("/trigger", handleTrigger(rs))
	r.Get("/config", handleGetConfig(rs))
	r.Put("/config", handleUpdateConfig(rs))
	return r
}

func handleStatus(rs *RollingSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := rs.Status(r.Context())
		writeJSON(w, http.StatusOK, status)
	}
}

func handleTrigger(rs *RollingSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		go rs.Trigger(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
	}
}

func handleGetConfig(rs *RollingSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, rs.Config())
	}
}

func handleUpdateConfig(rs *RollingSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg RollingSearchConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if cfg.IntervalHours <= 0 {
			cfg.IntervalHours = 12
		}
		if cfg.BatchSize <= 0 {
			cfg.BatchSize = 5
		}
		if cfg.MinResearchDays <= 0 {
			cfg.MinResearchDays = 7
		}
		if cfg.MaxSearchesPerDay <= 0 {
			cfg.MaxSearchesPerDay = 100
		}
		rs.UpdateConfig(r.Context(), cfg)
		writeJSON(w, http.StatusOK, cfg)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
