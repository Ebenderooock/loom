package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Router mounts the analytics endpoints. adminMW guards every route (analytics
// expose other users' watch activity, so they are admin-only).
func Router(svc *Service, adminMW func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	if adminMW != nil {
		r.Use(adminMW)
	}
	r.Get("/streams", handleStreams(svc))
	r.Get("/history", handleHistory(svc))
	r.Get("/stats", handleStats(svc))
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code": http.StatusText(status), "message": msg,
	}})
}

func handleStreams(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"streams": svc.ActiveStreams()})
	}
}

func handleHistory(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := HistoryFilter{User: r.URL.Query().Get("user")}
		if v := r.URL.Query().Get("limit"); v != "" {
			f.Limit, _ = strconv.Atoi(v)
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			f.Offset, _ = strconv.Atoi(v)
		}
		rows, err := svc.History(r.Context(), f)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"history": rows})
	}
}

func handleStats(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := 30
		if v := r.URL.Query().Get("days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				days = n
			}
		}
		stats, err := svc.Stats(r.Context(), days)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}
