package scanner

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/loomctl/loom/internal/movies"
)

// RegisterRoutes registers scanner endpoints on the given router.
// These should be mounted under /api/v1/movies/scan
func RegisterRoutes(r chi.Router, scanner *Scanner, movieSvc movies.Service) {
	r.Route("/scan", func(r chi.Router) {
		r.Post("/", startScan(scanner, movieSvc))
		r.Get("/unmatched", getUnmatched(scanner))
		r.Post("/match", matchFile(scanner))
		r.Get("/{scanId}", getScanStatus(scanner))
	})
}

type startScanRequest struct {
	RootFolderID string `json:"rootFolderId"`
}

func startScan(s *Scanner, movieSvc movies.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.RootFolderID == "" {
			http.Error(w, `{"error":"rootFolderId is required"}`, http.StatusBadRequest)
			return
		}

		folder, err := movieSvc.GetRootFolder(r.Context(), req.RootFolderID)
		if err != nil {
			http.Error(w, `{"error":"root folder not found"}`, http.StatusNotFound)
			return
		}

		scanID := s.StartScan(r.Context(), folder)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"scanId": scanID,
			"status": "running",
		})
	}
}

func getScanStatus(s *Scanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scanID := chi.URLParam(r, "scanId")
		result := s.GetScan(scanID)
		if result == nil {
			http.Error(w, `{"error":"scan not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func getUnmatched(s *Scanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files := s.GetAllUnmatched()
		if files == nil {
			files = []*UnmatchedFile{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	}
}

type matchFileRequest struct {
	UnmatchedID      string `json:"unmatchedId"`
	TmdbID           string `json:"tmdbId"`
	RootFolderID     string `json:"rootFolderId"`
	QualityProfileID string `json:"qualityProfileId"`
}

// RegisterSeriesRoutes registers series scanner endpoints on the given router.
// These should be mounted under /api/v1/series/scan
func RegisterSeriesRoutes(r chi.Router, ss *SeriesScanner) {
	r.Route("/scan", func(r chi.Router) {
		r.Post("/", startSeriesScan(ss))
		r.Get("/unmatched", getSeriesUnmatched(ss))
		r.Get("/{scanId}", getSeriesScanStatus(ss))
	})
}

type startSeriesScanRequest struct {
	Path string `json:"path"`
}

func startSeriesScan(ss *SeriesScanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startSeriesScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.Path == "" {
			http.Error(w, `{"error":"path is required"}`, http.StatusBadRequest)
			return
		}

		scanID, err := ss.StartSeriesScan(r.Context(), req.Path)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"scanId": scanID,
			"status": "running",
		})
	}
}

func getSeriesScanStatus(ss *SeriesScanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scanID := chi.URLParam(r, "scanId")
		result := ss.GetSeriesScanStatus(scanID)
		if result == nil {
			http.Error(w, `{"error":"scan not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func getSeriesUnmatched(ss *SeriesScanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files := ss.GetSeriesUnmatched()
		if files == nil {
			files = []*UnmatchedFile{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	}
}

func matchFile(s *Scanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req matchFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if err := s.MatchFile(r.Context(), req.UnmatchedID, req.TmdbID, req.RootFolderID, req.QualityProfileID); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "matched"})
	}
}
