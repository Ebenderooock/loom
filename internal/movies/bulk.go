package movies

import (
	"encoding/json"
	"net/http"
)

// bulkUpdateRequest is the payload for POST /api/v1/movies/bulk.
type bulkUpdateRequest struct {
	IDs              []string          `json:"ids"`
	MonitoringStatus *MonitoringStatus `json:"monitoring_status,omitempty"`
	QualityProfileID *string           `json:"quality_profile_id,omitempty"`
	Delete           bool              `json:"delete,omitempty"`
}

// bulkUpdateMovies applies updates to many movies in one call.
func bulkUpdateMovies(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bulkUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(req.IDs) == 0 {
			http.Error(w, "ids required", http.StatusBadRequest)
			return
		}

		var updated []*Movie
		for _, id := range req.IDs {
			if req.Delete {
				if err := svc.DeleteMovie(r.Context(), id); err != nil {
					continue
				}
				continue
			}
			movie, err := svc.GetMovie(r.Context(), id)
			if err != nil || movie == nil {
				continue
			}
			changed := false
			if req.MonitoringStatus != nil {
				if err := svc.SetMonitoringStatus(r.Context(), id, *req.MonitoringStatus); err == nil {
					movie.MonitoringStatus = *req.MonitoringStatus
					changed = true
				}
			}
			if req.QualityProfileID != nil {
				movie.QualityProfileID = *req.QualityProfileID
				changed = true
			}
			if changed {
				_ = svc.UpdateMovie(r.Context(), movie)
			}
			updated = append(updated, movie)
		}

		if updated == nil {
			updated = []*Movie{}
		}
		out := make([]map[string]interface{}, 0, len(updated))
		for _, m := range updated {
			out = append(out, movieToResponse(m))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": out,
		})
	}
}
