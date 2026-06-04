package requests

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/go-chi/chi/v5"
)

// userIDStr renders an auth identity's numeric user id as a stable string key.
func userIDStr(id *auth.Identity) string {
	return strconv.FormatInt(id.UserID, 10)
}

// Router returns a chi.Router exposing the request endpoints, mounted under
// /api/v1/requests. User endpoints require authentication; admin endpoints are
// wrapped with the supplied adminOnly middleware.
func Router(svc *Service, adminOnly func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()

	// Any authenticated user.
	r.Post("/", svc.handleCreate)
	r.Get("/mine", svc.handleListMine)

	// Admin only.
	r.Group(func(ar chi.Router) {
		ar.Use(adminOnly)
		ar.Get("/", svc.handleListAll)
		ar.Post("/{id}/approve", svc.handleApprove)
		ar.Post("/{id}/reject", svc.handleReject)
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"code": http.StatusText(status), "message": msg},
	})
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if id == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req, err := s.Create(r.Context(), userIDStr(id), id.Username, in)
	switch {
	case errors.Is(err, ErrDuplicate):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, ErrAlreadyAvailable):
		writeError(w, http.StatusConflict, "this title is already in your library")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Service) handleListMine(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if id == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	list, err := s.ListMine(r.Context(), userIDStr(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []Request{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (s *Service) handleListAll(w http.ResponseWriter, r *http.Request) {
	status := Status(r.URL.Query().Get("status"))
	if status == "all" {
		status = ""
	}
	list, err := s.ListAll(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []Request{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

type approveBody struct {
	QualityProfileID string `json:"quality_profile_id"`
	LibraryID        string `json:"library_id"`
}

func (s *Service) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if id == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body approveBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.QualityProfileID == "" || body.LibraryID == "" {
		writeError(w, http.StatusBadRequest, "quality_profile_id and library_id are required")
		return
	}
	req, err := s.Approve(r.Context(), chi.URLParam(r, "id"), body.QualityProfileID, body.LibraryID, id.Username)
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "request not found")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type rejectBody struct {
	Reason string `json:"reason"`
}

func (s *Service) handleReject(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if id == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body rejectBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	req, err := s.Reject(r.Context(), chi.URLParam(r, "id"), body.Reason, id.Username)
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "request not found")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}
