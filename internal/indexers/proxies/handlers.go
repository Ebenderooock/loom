package proxies

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Mount registers /api/v1/proxies/* on r. Designed to be passed as
// an indexers.RouteMounter so the routes share the auth scope of the
// indexers handler.
func (s *Service) Mount(r chi.Router) {
	r.Route("/api/v1/proxies", func(r chi.Router) {
		r.Get("/", s.handleList)
		r.Post("/", s.handleCreate)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGet)
			r.Put("/", s.handleReplace)
			r.Patch("/", s.handlePatch)
			r.Delete("/", s.handleDelete)
			r.Post("/test", s.handleTest)
		})
	})
}

// --- error envelope (matches indexers handlers) -------------------

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeErrorWith(w, status, code, msg, nil)
}

func writeErrorWith(w http.ResponseWriter, status int, code, msg string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorPayload{Code: code, Message: msg, Details: details}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- list/get -----------------------------------------------------

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxies": rows})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// --- create -------------------------------------------------------

type createRequest struct {
	ID      string          `json:"id"`
	Kind    Kind            `json:"kind"`
	Name    string          `json:"name"`
	Enabled *bool           `json:"enabled,omitempty"`
	Config  json.RawMessage `json:"config,omitempty"`
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(string(req.Kind)) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "kind is required")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p := Proxy{ID: req.ID, Kind: req.Kind, Name: req.Name, Enabled: enabled, Config: req.Config}
	out, err := s.Create(r.Context(), p)
	if err != nil {
		var ve *ErrValidation
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, "invalid_config", ve.Msg)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// --- replace ------------------------------------------------------

type replaceRequest struct {
	Kind    Kind            `json:"kind"`
	Name    string          `json:"name"`
	Enabled *bool           `json:"enabled,omitempty"`
	Config  json.RawMessage `json:"config,omitempty"`
}

func (s *Service) handleReplace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req replaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p := Proxy{ID: id, Kind: req.Kind, Name: req.Name, Enabled: enabled, Config: req.Config}
	out, err := s.Replace(r.Context(), p)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		var ve *ErrValidation
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, "invalid_config", ve.Msg)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// --- patch --------------------------------------------------------

type patchRequest struct {
	Kind    *Kind            `json:"kind,omitempty"`
	Name    *string          `json:"name,omitempty"`
	Enabled *bool            `json:"enabled,omitempty"`
	Config  *json.RawMessage `json:"config,omitempty"`
}

func (s *Service) handlePatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	patch := Patch{
		ID:      id,
		Kind:    req.Kind,
		Name:    req.Name,
		Enabled: req.Enabled,
		Config:  req.Config,
	}
	out, err := s.Patch(r.Context(), patch)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		var ve *ErrValidation
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, "invalid_config", ve.Msg)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// --- delete -------------------------------------------------------

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.Delete(r.Context(), id)
	if err != nil {
		var inUse *ErrInUse
		if errors.As(err, &inUse) {
			writeErrorWith(w, http.StatusConflict, "proxy_in_use",
				err.Error(),
				map[string]any{"indexer_ids": inUse.IndexerIDs})
			return
		}
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- test ---------------------------------------------------------

func (s *Service) handleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := s.TestProxy(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "test_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
