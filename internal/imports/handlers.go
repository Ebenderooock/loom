package imports

import (
	"encoding/json"
	"errors"
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
	r.Post("/scans", handleStartFolderScan(pipeline))
	r.Get("/scans/{id}", handleGetFolderScan(pipeline))
	r.Delete("/scans/{id}", handleCancelFolderScan(pipeline))
	r.Get("/decisions", handleListDecisions(pipeline))
	r.Post("/preview", handlePreviewImport(pipeline))
	r.Get("/jobs", handleListJobs(pipeline))
	r.Get("/jobs/{id}", handleGetJob(pipeline))
	r.Post("/jobs/{id}/retry", handleRetryJob(pipeline))
	r.Post("/batch", handleBatchImport(pipeline))
	return r
}

func handleListHistory(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 50
		}

		mediaType, mediaID := r.URL.Query().Get("media_type"), r.URL.Query().Get("media_id")
		if (mediaType == "") != (mediaID == "") || (mediaType != "" && mediaType != "movie" && mediaType != "series") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media_type (movie or series) and media_id must be provided together"})
			return
		}
		records, err := p.ListTitleHistory(r.Context(), mediaType, mediaID, limit, offset)
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

		job, err := p.EnqueueImport(r.Context(), JobRequest{Kind: JobKindManual, Path: req.Path})
		writeEnqueued(w, job, err)
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

		job, err := p.EnqueueImport(r.Context(), JobRequest{
			Kind: JobKindManualMatch, Path: req.Path, MediaType: MediaType(req.MediaType), MediaID: req.MediaID,
		})
		writeEnqueued(w, job, err)
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

		job, err := p.EnqueueImport(r.Context(), JobRequest{
			Kind: JobKindReimport, Path: req.SourcePath, MediaType: MediaType(req.MediaType), MediaID: req.MediaID,
			ConflictPolicy: policy,
		})
		writeEnqueued(w, job, err)
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

func handleStartFolderScan(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		scan, err := p.StartFolderScan(r.Context(), req.Path)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errScansBusy) {
				status = http.StatusTooManyRequests
			} else if errors.Is(err, errScansStopped) {
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Location", "/api/v1/imports/scans/"+scan.ID)
		writeJSON(w, http.StatusAccepted, map[string]any{"data": scan})
	}
}

func handleGetFolderScan(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		offset, limit := 0, maxScanPage
		for name, target := range map[string]*int{"offset": &offset, "limit": &limit} {
			if raw := r.URL.Query().Get(name); raw != "" {
				value, err := strconv.Atoi(raw)
				if err != nil || value < 0 || (name == "limit" && value == 0) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
					return
				}
				*target = value
			}
		}
		scan := p.GetFolderScan(chi.URLParam(r, "id"), offset, limit)
		if scan == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scan not found or expired; start a new scan"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": scan})
	}
}

func handleCancelFolderScan(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scan := p.CancelFolderScan(chi.URLParam(r, "id"))
		if scan == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scan not found or expired"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": scan})
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

// writeEnqueued keeps the legacy {status:"accepted"} shape while adding the
// queued job so clients can follow its progress.
func writeEnqueued(w http.ResponseWriter, job *ImportJob, err error) {
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "job_id": job.ID, "data": job})
}

func handleListJobs(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		jobs, err := p.ListJobs(r.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": jobs})
	}
}

func handleGetJob(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := p.GetJob(r.Context(), chi.URLParam(r, "id"), true)
		if errors.Is(err, ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": job})
	}
}

func handleRetryJob(p *ImportPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := p.RetryJob(r.Context(), chi.URLParam(r, "id"))
		switch {
		case errors.Is(err, ErrJobNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrJobActive), errors.Is(err, ErrNothingToRetry), errors.Is(err, ErrPendingReview):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusAccepted, map[string]any{"data": job})
		}
	}
}

// maxBatchPaths bounds a single batch request.
const maxBatchPaths = 1000

func handleBatchImport(p *ImportPipeline) http.HandlerFunc {
	type request struct {
		Paths     []string `json:"paths"`
		MediaType string   `json:"media_type,omitempty"`
		MediaID   string   `json:"media_id,omitempty"`
	}
	type pathError struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if len(req.Paths) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "paths is required"})
			return
		}
		if len(req.Paths) > maxBatchPaths {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many paths in one batch"})
			return
		}
		if (req.MediaType == "") != (req.MediaID == "") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media_type and media_id must be provided together"})
			return
		}
		kind := JobKindManual
		if req.MediaID != "" {
			kind = JobKindManualMatch
		}
		jobs := []*ImportJob{}
		errs := []pathError{}
		seen := map[string]bool{}
		for _, path := range req.Paths {
			job, err := p.EnqueueImport(r.Context(), JobRequest{
				Kind: kind, Path: path, MediaType: MediaType(req.MediaType), MediaID: req.MediaID,
			})
			if err != nil {
				errs = append(errs, pathError{Path: path, Error: err.Error()})
				continue
			}
			if !seen[job.ID] {
				seen[job.ID] = true
				jobs = append(jobs, job)
			}
		}
		status := http.StatusAccepted
		if len(jobs) == 0 {
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, map[string]any{"data": jobs, "errors": errs})
	}
}
