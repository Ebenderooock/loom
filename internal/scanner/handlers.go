package scanner

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/libraries"
)

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// RegisterRoutes registers scanner endpoints on the given router.
// These should be mounted under /api/v1/movies/scan
func RegisterRoutes(r chi.Router, scanner *Scanner, libStore *libraries.Store) {
	r.Route("/scan", func(r chi.Router) {
		r.Post("/", startScan(scanner, libStore))
		r.Get("/unmatched", getUnmatched(scanner))
		r.Post("/match", matchFile(scanner))
		r.Get("/{scanId}", getScanStatus(scanner))
	})
	r.Post("/{id}/rescan", rescanMovie(scanner, libStore))
}

type startScanRequest struct {
	LibraryID string `json:"libraryId"`
}

func startScan(s *Scanner, libStore *libraries.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.LibraryID == "" {
			writeJSONError(w, "libraryId is required", http.StatusBadRequest)
			return
		}

		lib, err := libStore.Get(r.Context(), req.LibraryID)
		if err != nil {
			writeJSONError(w, "library not found", http.StatusNotFound)
			return
		}

		scanID := s.StartScan(r.Context(), lib.ID, lib.Path)

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
			writeJSONError(w, "scan not found", http.StatusNotFound)
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
	LibraryID        string `json:"libraryId"`
	QualityProfileID string `json:"qualityProfileId"`
}

// RegisterSeriesRoutes registers series scanner endpoints on the given router.
// These should be mounted under /api/v1/series/scan
func RegisterSeriesRoutes(r chi.Router, ss *SeriesScanner, libStore *libraries.Store) {
	r.Route("/scan", func(r chi.Router) {
		r.Post("/", startSeriesScan(ss, libStore))
		r.Get("/unmatched", getSeriesUnmatched(ss))
		r.Get("/{scanId}", getSeriesScanStatus(ss))
	})
	r.Post("/{id}/rescan", rescanSeries(ss, libStore))
}

type startSeriesScanRequest struct {
	LibraryID string `json:"libraryId"`
}

func startSeriesScan(ss *SeriesScanner, libStore *libraries.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startSeriesScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.LibraryID == "" {
			writeJSONError(w, "libraryId is required", http.StatusBadRequest)
			return
		}

		lib, err := libStore.Get(r.Context(), req.LibraryID)
		if err != nil {
			writeJSONError(w, "library not found", http.StatusNotFound)
			return
		}

		scanID, err := ss.StartSeriesScan(r.Context(), lib.ID, lib.Path)
		if err != nil {
			writeJSONError(w, "scan failed", http.StatusInternalServerError)
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
			writeJSONError(w, "scan not found", http.StatusNotFound)
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
			writeJSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := s.MatchFile(r.Context(), req.UnmatchedID, req.TmdbID, req.LibraryID, req.QualityProfileID); err != nil {
			writeJSONError(w, "match failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "matched"})
	}
}

func rescanSeries(ss *SeriesScanner, libStore *libraries.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesID := chi.URLParam(r, "id")
		if seriesID == "" {
			writeJSONError(w, "series id required", http.StatusBadRequest)
			return
		}

		// Determine which library this series belongs to
		var req struct {
			LibraryID string `json:"libraryId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req) // body may be empty

		if req.LibraryID == "" {
			http.Error(w, `{"error":"libraryId is required"}`, http.StatusBadRequest)
			return
		}

		lib, err := libStore.Get(r.Context(), req.LibraryID)
		if err != nil {
			http.Error(w, `{"error":"library not found"}`, http.StatusNotFound)
			return
		}

		result, err := ss.RescanSeries(r.Context(), seriesID, lib.Path)
		if err != nil {
			writeJSONError(w, "rescan failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func rescanMovie(s *Scanner, libStore *libraries.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		movieID := chi.URLParam(r, "id")

		var req struct {
			LibraryID string `json:"libraryId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.LibraryID == "" {
			writeJSONError(w, "libraryId is required", http.StatusBadRequest)
			return
		}

		lib, err := libStore.Get(r.Context(), req.LibraryID)
		if err != nil {
			writeJSONError(w, "library not found", http.StatusNotFound)
			return
		}

		result, err := s.RescanMovie(r.Context(), movieID, lib.Path)
		if err != nil {
			writeJSONError(w, "rescan failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
