package cleanup

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router exposing the cleanup endpoints. It is mounted
// under /api/v1/cleanup by the server.
func Router(svc *Service) chi.Router {
	r := chi.NewRouter()
	r.Get("/orphans", svc.handleListOrphans)
	r.Post("/scan", svc.handleScan)
	r.Get("/settings", svc.handleGetSettings)
	r.Put("/settings", svc.handleSaveSettings)
	r.Post("/orphans/{id}/approve", svc.handleApprove)
	r.Post("/orphans/{id}/ignore", svc.handleIgnore)
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}

func (s *Service) handleListOrphans(w http.ResponseWriter, r *http.Request) {
	status := OrphanStatus(r.URL.Query().Get("status"))
	if status == "" {
		status = StatusPending
	}
	if status == "all" {
		status = ""
	}
	orphans, err := s.store.List(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if orphans == nil {
		orphans = []Orphan{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": orphans})
}

func (s *Service) handleScan(w http.ResponseWriter, r *http.Request) {
	found, err := s.Scan(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": found})
}

func (s *Service) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Service) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var settings Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SaveSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	saved, _ := s.store.GetSettings(r.Context())
	writeJSON(w, http.StatusOK, saved)
}

func (s *Service) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Approve(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (s *Service) handleIgnore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Ignore(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
}
