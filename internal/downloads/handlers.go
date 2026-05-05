package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Mount attaches every /api/v1/download-clients/* route to r. The
// caller wraps r in auth.RequireAuth (see internal/server/server.go).
//
// RouteExtensions registered via ServiceOptions are invoked here too,
// sharing the same auth scope.
func (s *Service) Mount(r chi.Router) {
	r.Route("/api/v1/download-clients", func(r chi.Router) {
		r.Get("/", s.handleList)
		r.Post("/", s.handleCreate)
		r.Post("/test", s.handleTestConfig)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGet)
			r.Put("/", s.handleReplace)
			r.Patch("/", s.handlePatch)
			r.Delete("/", s.handleDelete)
			r.Post("/test", s.handleTest)
			r.Get("/categories", s.handleCategories)
			r.Get("/free-space", s.handleFreeSpace)
			r.Get("/items", s.handleItems)
			r.Post("/items", s.handleAdd)
			r.Post("/pause", s.handlePause)
			r.Post("/resume", s.handleResume)
		})
	})
	// Aggregate activity endpoint across all download clients
	r.Get("/api/v1/activity", s.handleActivity)
	for _, ext := range s.routeExtensions {
		if ext != nil {
			ext(r)
		}
	}
}

// errorBody is the project-wide error envelope returned by the
// download-client handlers. The shape matches indexers'.
type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorPayload{Code: code, Message: msg}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- create ---------------------------------------------------------

type createRequest struct {
	ID              string          `json:"id"`
	Kind            Kind            `json:"kind"`
	Name            string          `json:"name"`
	Protocol        Protocol        `json:"protocol"`
	Enabled         *bool           `json:"enabled,omitempty"`
	Priority        *int            `json:"priority,omitempty"`
	Host            string          `json:"host,omitempty"`
	Port            int             `json:"port,omitempty"`
	TLS             bool            `json:"tls,omitempty"`
	Username        string          `json:"username,omitempty"`
	Password        string          `json:"password,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	CategoryDefault string          `json:"category_default,omitempty"`
	SavePathDefault string          `json:"save_path_default,omitempty"`
	RemoveCompleted bool            `json:"remove_completed,omitempty"`
	RemoveFailed    bool            `json:"remove_failed,omitempty"`
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.Kind == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "kind is required")
		return
	}
	if req.Protocol == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "protocol is required")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.ID == "" {
		req.ID = generateID(req.Kind, req.Name)
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := 25
	if req.Priority != nil {
		priority = *req.Priority
	}

	def := Definition{
		ID:              req.ID,
		Name:            req.Name,
		Kind:            req.Kind,
		Protocol:        req.Protocol,
		Enabled:         enabled,
		Priority:        priority,
		Host:            req.Host,
		Port:            req.Port,
		TLS:             req.TLS,
		Username:        req.Username,
		Password:        req.Password,
		Config:          req.Config,
		CategoryDefault: req.CategoryDefault,
		SavePathDefault: req.SavePathDefault,
		RemoveCompleted: req.RemoveCompleted,
		RemoveFailed:    req.RemoveFailed,
	}
	saved, err := s.Create(r.Context(), def)
	if err != nil {
		if errors.Is(err, ErrUnknownKind) {
			writeError(w, http.StatusBadRequest, "unknown_kind", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}
	dh, _ := s.GetWithHealth(r.Context(), saved.ID)
	writeJSON(w, http.StatusCreated, dh)
}

// --- list / get -----------------------------------------------------

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	defs, err := s.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"download_clients": defs})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dh, err := s.GetWithHealth(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "download client not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dh)
}

// --- replace --------------------------------------------------------

type replaceRequest struct {
	Kind            Kind            `json:"kind"`
	Name            string          `json:"name"`
	Protocol        Protocol        `json:"protocol"`
	Enabled         bool            `json:"enabled"`
	Priority        int             `json:"priority"`
	Host            string          `json:"host,omitempty"`
	Port            int             `json:"port,omitempty"`
	TLS             bool            `json:"tls,omitempty"`
	Username        string          `json:"username,omitempty"`
	Password        string          `json:"password,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	CategoryDefault string          `json:"category_default,omitempty"`
	SavePathDefault string          `json:"save_path_default,omitempty"`
	RemoveCompleted bool            `json:"remove_completed,omitempty"`
	RemoveFailed    bool            `json:"remove_failed,omitempty"`
}

func (s *Service) handleReplace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req replaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.Kind == "" || req.Name == "" || req.Protocol == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "kind, name, and protocol are required")
		return
	}
	def := Definition{
		ID:              id,
		Name:            req.Name,
		Kind:            req.Kind,
		Protocol:        req.Protocol,
		Enabled:         req.Enabled,
		Priority:        req.Priority,
		Host:            req.Host,
		Port:            req.Port,
		TLS:             req.TLS,
		Username:        req.Username,
		Password:        req.Password,
		Config:          req.Config,
		CategoryDefault: req.CategoryDefault,
		SavePathDefault: req.SavePathDefault,
		RemoveCompleted: req.RemoveCompleted,
		RemoveFailed:    req.RemoveFailed,
	}
	saved, err := s.Replace(r.Context(), def)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "download client not found")
			return
		}
		if errors.Is(err, ErrUnknownKind) {
			writeError(w, http.StatusBadRequest, "unknown_kind", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "replace_failed", err.Error())
		return
	}
	dh, _ := s.GetWithHealth(r.Context(), saved.ID)
	writeJSON(w, http.StatusOK, dh)
}

// --- patch ----------------------------------------------------------

type patchRequest struct {
	Name            *string `json:"name,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
	Priority        *int    `json:"priority,omitempty"`
	CategoryDefault *string `json:"category_default,omitempty"`
	SavePathDefault *string `json:"save_path_default,omitempty"`
	RemoveCompleted *bool   `json:"remove_completed,omitempty"`
	RemoveFailed    *bool   `json:"remove_failed,omitempty"`
}

func (s *Service) handlePatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	patch := Patch{
		ID:              id,
		Name:            req.Name,
		Enabled:         req.Enabled,
		Priority:        req.Priority,
		CategoryDefault: req.CategoryDefault,
		SavePathDefault: req.SavePathDefault,
		RemoveCompleted: req.RemoveCompleted,
		RemoveFailed:    req.RemoveFailed,
	}
	saved, err := s.Patch(r.Context(), patch)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "download client not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "patch_failed", err.Error())
		return
	}
	dh, _ := s.GetWithHealth(r.Context(), saved.ID)
	writeJSON(w, http.StatusOK, dh)
}

// --- delete ---------------------------------------------------------

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- test -----------------------------------------------------------

type testResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (s *Service) handleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := s.TestOne(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	out := testResponse{OK: err == nil}
	if err != nil {
		out.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTestConfig tests a download client configuration without saving it.
// POST /api/v1/download-clients/test  (no {id} param — uses request body)
func (s *Service) handleTestConfig(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.Kind == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "kind is required")
		return
	}

	def := Definition{
		ID:       "_test_ephemeral",
		Kind:     req.Kind,
		Name:     req.Name,
		Protocol: req.Protocol,
		Enabled:  true,
		Host:     req.Host,
		Port:     req.Port,
		TLS:      req.TLS,
		Username: req.Username,
		Password: req.Password,
		Config:   req.Config,
	}

	c, err := build(r.Context(), def)
	if err != nil {
		writeJSON(w, http.StatusOK, testResponse{OK: false, Error: err.Error()})
		return
	}

	ctx := r.Context()
	if s.healthTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.healthTimeout)
		defer cancel()
	}

	testErr := c.Test(ctx)
	out := testResponse{OK: testErr == nil}
	if testErr != nil {
		out.Error = testErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// --- categories -----------------------------------------------------

func (s *Service) handleCategories(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	cats, err := c.Categories(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "categories_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

// --- free-space -----------------------------------------------------

type freeSpaceResponse struct {
	Bytes   int64  `json:"bytes"`
	Unknown bool   `json:"unknown,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Service) handleFreeSpace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	bytes, err := c.FreeSpace(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, freeSpaceResponse{Bytes: -1, Unknown: true, Error: err.Error()})
		return
	}
	resp := freeSpaceResponse{Bytes: bytes}
	if bytes < 0 {
		resp.Unknown = true
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- items (status / add) ------------------------------------------

func (s *Service) handleItems(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	ids := splitCSV(r.URL.Query().Get("ids"))
	items, err := c.Status(r.Context(), ids...)
	if err != nil {
		writeError(w, http.StatusBadGateway, "status_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Service) handleAdd(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	res, err := c.Add(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "add_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, res)
}

// --- pause / resume ------------------------------------------------

type idsRequest struct {
	IDs []string `json:"ids,omitempty"`
}

func (s *Service) handlePause(w http.ResponseWriter, r *http.Request) {
	s.handlePauseResume(w, r, true)
}

func (s *Service) handleResume(w http.ResponseWriter, r *http.Request) {
	s.handlePauseResume(w, r, false)
}

func (s *Service) handlePauseResume(w http.ResponseWriter, r *http.Request, pause bool) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	var req idsRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
	}
	var err error
	if pause {
		err = c.Pause(r.Context(), req.IDs...)
	} else {
		err = c.Resume(r.Context(), req.IDs...)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- helpers --------------------------------------------------------

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// generateID derives a stable, URL-safe slug from kind + name when the
// caller didn't supply one.
func generateID(kind Kind, name string) string {
	prefix := strings.TrimSpace(string(kind))
	if i := strings.Index(prefix, "/"); i >= 0 {
		prefix = prefix[i+1:]
	}
	if prefix == "" {
		prefix = "client"
	}
	slug := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevDash := false
	for _, r := range slug {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		out = "x"
	}
	return prefix + "-" + out
}

// handleActivity returns aggregated download items from all configured clients.
func (s *Service) handleActivity(w http.ResponseWriter, r *http.Request) {
	opts := s.FanOutOpts(nil)
	status := s.registry.Status(r.Context(), nil, opts)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  status.Items,
		"errors": status.Errors,
	})
}
