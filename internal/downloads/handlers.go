package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/workflows"
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
			r.Post("/remove", s.handleRemove)
			r.Post("/set-priority", s.handleSetPriority)
			r.Post("/set-speed-limit", s.handleSetSpeedLimit)
			r.Post("/force-start", s.handleForceStart)
			r.Post("/recheck", s.handleRecheck)
			r.Post("/reannounce", s.handleReannounce)
		})
	})
	// Aggregate activity endpoint across all download clients
	r.Get("/api/v1/activity", s.handleActivity)
	// Aggregate actions (frontend sends client_id + item_id in body)
	r.Post("/api/v1/activity/pause", s.handleActivityPause)
	r.Post("/api/v1/activity/resume", s.handleActivityResume)
	r.Post("/api/v1/activity/remove", s.handleActivityRemove)
	// Per-item detail endpoint (torrent-aware)
	r.Get("/api/v1/activity/detail", s.handleActivityDetail)
	// Download history endpoint
	r.Get("/api/v1/downloads/history", s.handleHistory)
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
	Name            *string         `json:"name,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
	Priority        *int            `json:"priority,omitempty"`
	Host            *string         `json:"host,omitempty"`
	Port            *int            `json:"port,omitempty"`
	TLS             *bool           `json:"tls,omitempty"`
	Username        *string         `json:"username,omitempty"`
	Password        *string         `json:"password,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	CategoryDefault *string         `json:"category_default,omitempty"`
	SavePathDefault *string         `json:"save_path_default,omitempty"`
	RemoveCompleted *bool           `json:"remove_completed,omitempty"`
	RemoveFailed    *bool           `json:"remove_failed,omitempty"`
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
		ID:              "_test_ephemeral",
		Kind:            req.Kind,
		Name:            req.Name,
		Protocol:        req.Protocol,
		Enabled:         true,
		Host:            req.Host,
		Port:            req.Port,
		TLS:             req.TLS,
		Username:        req.Username,
		Password:        req.Password,
		Config:          req.Config,
		SavePathDefault: req.SavePathDefault,
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
	req.Normalize()
	if req.Magnet == "" && req.TorrentURL == "" && req.NZBURL == "" && len(req.RawBytes) == 0 {
		slog.Warn("handleAdd: empty download request after normalize",
			"client_id", id, "title", req.Title,
			"magnet", req.Magnet, "torrent_url", req.TorrentURL,
			"infohash", req.Infohash)
	}
	res, err := c.Add(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "add_failed", err.Error())
		return
	}

	// Record grab linkage when media context is provided (interactive search).
	s.recordManualGrab(r.Context(), res, req)

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

// --- remove / priority / speed-limit / force-start / recheck / reannounce ---

type removeRequest struct {
	IDs         []string `json:"ids,omitempty"`
	DeleteFiles bool     `json:"delete_files"`
}

type setPriorityRequest struct {
	IDs      []string `json:"ids,omitempty"`
	Priority Priority `json:"priority"`
}

type setSpeedLimitRequest struct {
	IDs              []string `json:"ids,omitempty"`
	LimitBytesPerSec int64    `json:"limit_bytes_per_sec"`
}

func (s *Service) handleRemove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	var req removeRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "at least one id is required")
		return
	}
	if err := c.Remove(r.Context(), req.IDs, req.DeleteFiles); err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	// Cancel any workflows tracking these downloads
	if s.orchestrator != nil {
		s.orchestrator.NotifyDownloadRemoved(id, req.IDs)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleSetPriority(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	var req setPriorityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := c.SetPriority(r.Context(), req.Priority, req.IDs...); err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleSetSpeedLimit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	var req setSpeedLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := c.SetSpeedLimit(r.Context(), req.LimitBytesPerSec, req.IDs...); err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleIDsAction(w http.ResponseWriter, r *http.Request, action func(context.Context, ...string) error) {
	var req idsRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
	}
	if err := action(r.Context(), req.IDs...); err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleForceStart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	s.handleIDsAction(w, r, c.ForceStart)
}

func (s *Service) handleRecheck(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	s.handleIDsAction(w, r, c.Recheck)
}

func (s *Service) handleReannounce(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	s.handleIDsAction(w, r, c.Reannounce)
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

// handleHistory returns paginated download history.
func (s *Service) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.historyStore == nil {
		writeJSON(w, http.StatusOK, []HistoryEntry{})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	entries, err := s.historyStore.List(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody{
			Error: errorPayload{Message: "failed to list download history", Code: "internal_error"},
		})
		return
	}
	if entries == nil {
		entries = []HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// recordManualGrab records grab linkage for interactive search grabs.
// If no media context is present or no orchestrator is configured, this is a no-op.
func (s *Service) recordManualGrab(ctx context.Context, res AddResult, req AddRequest) {
	if req.MediaType == "" || s.orchestrator == nil {
		return
	}

	var wf *workflows.Workflow
	var err error
	switch req.MediaType {
	case "episode":
		if len(req.EpisodeIDs) > 0 {
			wf, err = s.orchestrator.StartSearch(ctx, workflows.TypeEpisodeSearch, workflows.MediaTypeEpisode, "", req.EpisodeIDs)
		}
	case "movie":
		if req.MovieID != "" {
			wf, err = s.orchestrator.StartSearch(ctx, workflows.TypeMovieSearch, workflows.MediaTypeMovie, "", []string{req.MovieID})
		}
	}
	if err != nil {
		s.logger.Warn("failed to create manual grab workflow",
			"client_id", res.ClientID, "item_id", res.ItemID,
			"media_type", req.MediaType, "err", err)
		return
	}
	if wf != nil {
		s.orchestrator.Send(workflows.CmdGrabbed{
			WorkflowID:           wf.ID,
			ClientID:             res.ClientID,
			DownloadID:           res.ItemID,
			Title:                req.Title,
			SeedRatioLimit:       req.SeedRatioLimit,
			SeedTimeLimitMinutes: req.SeedTimeLimitMinutes,
		})
		s.logger.Info("recorded manual grab workflow",
			"workflow_id", wf.ID, "client_id", res.ClientID, "item_id", res.ItemID,
			"media_type", req.MediaType)
	}
}

// --- Aggregate activity actions (work across all clients) ---

type activityActionRequest struct {
	ClientID    string   `json:"client_id"`
	IDs         []string `json:"ids"`
	DeleteFiles bool     `json:"delete_files"`
}

func (s *Service) handleActivityPause(w http.ResponseWriter, r *http.Request) {
	s.handleActivityAction(w, r, func(c DownloadClient, ctx context.Context, ids []string) error {
		return c.Pause(ctx, ids...)
	})
}

func (s *Service) handleActivityResume(w http.ResponseWriter, r *http.Request) {
	s.handleActivityAction(w, r, func(c DownloadClient, ctx context.Context, ids []string) error {
		return c.Resume(ctx, ids...)
	})
}

func (s *Service) handleActivityRemove(w http.ResponseWriter, r *http.Request) {
	var req activityActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.ClientID == "" || len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "client_id and ids are required")
		return
	}
	c, ok := s.registry.Get(req.ClientID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	if err := c.Remove(r.Context(), req.IDs, req.DeleteFiles); err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	// Cancel any workflows tracking these downloads
	if s.orchestrator != nil {
		s.orchestrator.NotifyDownloadRemoved(req.ClientID, req.IDs)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleActivityAction(w http.ResponseWriter, r *http.Request, fn func(DownloadClient, context.Context, []string) error) {
	var req activityActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.ClientID == "" || len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "client_id and ids are required")
		return
	}
	c, ok := s.registry.Get(req.ClientID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}
	if err := fn(c, r.Context(), req.IDs); err != nil {
		writeError(w, http.StatusBadGateway, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleActivityDetail returns detailed torrent information for a single item.
func (s *Service) handleActivityDetail(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	itemID := r.URL.Query().Get("item_id")
	if clientID == "" || itemID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "client_id and item_id query params are required")
		return
	}

	c, ok := s.registry.Get(clientID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "download client not found")
		return
	}

	// For clients that support detailed info (e.g. builtin/torrent).
	if dp, ok := c.(DetailProvider); ok {
		detail, err := dp.Detail(r.Context(), itemID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, detail)
		return
	}

	// For non-torrent clients, return basic status info.
	items, err := c.Status(r.Context(), itemID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "status_failed", err.Error())
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	writeJSON(w, http.StatusOK, items[0])
}
