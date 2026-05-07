package series

import (
	"encoding/json"
	"net/http"
)

// bulkIDsRequest is the payload for bulk archive/unarchive operations.
type bulkIDsRequest struct {
	IDs []string `json:"ids"`
}

func bulkArchiveSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bulkIDsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(req.IDs) == 0 {
			http.Error(w, "ids required", http.StatusBadRequest)
			return
		}
		for _, id := range req.IDs {
			_ = svc.SetMonitoringStatus(r.Context(), id, MonitoringArchived)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func bulkUnarchiveSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bulkIDsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(req.IDs) == 0 {
			http.Error(w, "ids required", http.StatusBadRequest)
			return
		}
		for _, id := range req.IDs {
			_ = svc.SetMonitoringStatus(r.Context(), id, MonitoringMonitored)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// bulkUpdateSeriesRequest is the payload for POST /api/v1/series/bulk.
type bulkUpdateSeriesRequest struct {
	IDs              []string          `json:"ids"`
	MonitoringStatus *MonitoringStatus `json:"monitoring_status,omitempty"`
	QualityProfileID *string           `json:"quality_profile_id,omitempty"`
	Delete           bool              `json:"delete,omitempty"`
}

// bulkUpdateSeries applies updates to many series in one call.
func bulkUpdateSeries(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bulkUpdateSeriesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(req.IDs) == 0 {
			http.Error(w, "ids required", http.StatusBadRequest)
			return
		}

		var updated []*Series
		for _, id := range req.IDs {
			if req.Delete {
				if err := svc.DeleteSeries(r.Context(), id); err != nil {
					continue
				}
				continue
			}
			sr, err := svc.GetSeries(r.Context(), id)
			if err != nil || sr == nil {
				continue
			}
			changed := false
			if req.MonitoringStatus != nil {
				if err := svc.SetMonitoringStatus(r.Context(), id, *req.MonitoringStatus); err == nil {
					sr.MonitoringStatus = *req.MonitoringStatus
					changed = true
				}
			}
			if req.QualityProfileID != nil {
				sr.QualityProfileID = *req.QualityProfileID
				changed = true
			}
			if changed {
				_ = svc.UpdateSeries(r.Context(), sr)
			}
			updated = append(updated, sr)
		}

		if updated == nil {
			updated = []*Series{}
		}
		out := make([]map[string]interface{}, 0, len(updated))
		for _, s := range updated {
			out = append(out, seriesToResponse(s))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": out,
		})
	}
}
