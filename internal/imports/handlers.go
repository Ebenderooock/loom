package imports

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with import history and manual import endpoints.
func Router(pipeline *ImportPipeline) chi.Router {
	r := chi.NewRouter()
	r.Get("/history", handleListHistory(pipeline))
	r.Post("/manual", handleManualImport(pipeline))
	return r
}

func handleListHistory(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 50
		}

		records, err := p.ListHistory(r.Context(), limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if records == nil {
			records = []*ImportRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": records})
	}
}

func handleManualImport(p *ImportPipeline) http.HandlerFunc {
	type request struct {
		Path string `json:"path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}

		if err := p.ImportManual(r.Context(), req.Path); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
