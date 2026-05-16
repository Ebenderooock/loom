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
	r.Post("/manual-match", handleManualMatch(pipeline))
	r.Post("/reimport", handleReimport(pipeline))
	r.Post("/scan", handleScanFolder(pipeline))
	r.Get("/decisions", handleListDecisions(pipeline))
	r.Post("/preview", handlePreviewImport(pipeline))
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

		if err := p.SubmitManualImport(req.Path); err != nil {
			status := http.StatusUnprocessableEntity
			if err.Error() == "import queue full, try again later" {
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func handleManualMatch(p *ImportPipeline) http.HandlerFunc {
	type request struct {
		Path      string `json:"path"`
		MediaType string `json:"media_type"`
		MediaID   string `json:"media_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Path == "" || req.MediaType == "" || req.MediaID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path, media_type, and media_id are required"})
			return
		}

		if err := p.SubmitManualMatch(req.Path, MediaType(req.MediaType), req.MediaID); err != nil {
			status := http.StatusUnprocessableEntity
			if err.Error() == "import queue full, try again later" {
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func handleReimport(p *ImportPipeline) http.HandlerFunc {
	type request struct {
		MediaType      string `json:"media_type"`
		MediaID        string `json:"media_id"`
		SourcePath     string `json:"source_path"`
		ConflictPolicy string `json:"conflict_policy,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.MediaType == "" || req.MediaID == "" || req.SourcePath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media_type, media_id, and source_path are required"})
			return
		}

		policy := ConflictReplaceIfBetter
		if req.ConflictPolicy != "" {
			policy = ConflictPolicy(req.ConflictPolicy)
		}

		if err := p.SubmitReimport(MediaType(req.MediaType), req.MediaID, req.SourcePath, ReimportOptions{
			ConflictPolicy: policy,
		}); err != nil {
			status := http.StatusUnprocessableEntity
			if err.Error() == "import queue full, try again later" {
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func handleScanFolder(p *ImportPipeline) http.HandlerFunc {
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

		results, err := p.ScanFolder(r.Context(), req.Path)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		if results == nil {
			results = []ScanResult{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": results})
	}
}

func handleListDecisions(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		mediaID := r.URL.Query().Get("media_id")
		if limit <= 0 {
			limit = 50
		}

		decisions, err := p.decisionLog.ListDecisions(r.Context(), mediaID, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if decisions == nil {
			decisions = []*ImportDecision{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": decisions})
	}
}

func handlePreviewImport(p *ImportPipeline) http.HandlerFunc {
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

		results, err := p.PreviewImport(r.Context(), req.Path)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		if results == nil {
			results = []ImportPreview{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": results})
	}
}
