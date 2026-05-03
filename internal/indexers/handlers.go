package indexers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Mount attaches every /api/v1/indexers/* route to r. It does not
// install authentication — the caller wraps r in the project's
// auth.RequireAuth middleware (see internal/server/server.go).
//
// Any RouteMounter passed via ServiceOptions.RouteExtensions is
// invoked here too, sharing the same auth scope as the indexer
// routes. Phase 2e uses this to attach /api/v1/proxies/* without
// editing server.go.
func (s *Service) Mount(r chi.Router) {
	r.Route("/api/v1/indexers", func(r chi.Router) {
		r.Get("/", s.handleList)
		r.Post("/", s.handleCreate)
		r.Post("/search", s.handleSearch)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGet)
			r.Put("/", s.handleReplace)
			r.Patch("/", s.handlePatch)
			r.Delete("/", s.handleDelete)
			r.Get("/caps", s.handleCaps)
			r.Post("/test", s.handleTest)
		})
	})
	for _, ext := range s.routeExtensions {
		if ext != nil {
			ext(r)
		}
	}
}

// errorBody is the project-wide error envelope returned by the
// indexer handlers. The shape is documented in docs/api.md.
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
	ID         string          `json:"id"`
	Kind       Kind            `json:"kind"`
	Name       string          `json:"name"`
	Enabled    *bool           `json:"enabled,omitempty"`
	Priority   *int            `json:"priority,omitempty"`
	Config     json.RawMessage `json:"config,omitempty"`
	Categories []Category      `json:"categories,omitempty"`
	Tags       []string        `json:"tags,omitempty"`
	ProxyID    string          `json:"proxy_id,omitempty"`
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
		ID:         req.ID,
		Kind:       req.Kind,
		Name:       req.Name,
		Enabled:    enabled,
		Priority:   priority,
		Config:     req.Config,
		Categories: req.Categories,
		Tags:       req.Tags,
		ProxyID:    req.ProxyID,
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
	writeJSON(w, http.StatusCreated, saved)
}

// --- list / get -----------------------------------------------------

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	defs, err := s.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"indexers": defs})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	def, err := s.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "indexer not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	dh := DefinitionWithHealth{Definition: def}
	if h, herr := s.repo.GetHealth(r.Context(), id); herr == nil {
		dh.Health = &h
	}
	writeJSON(w, http.StatusOK, dh)
}

// --- replace --------------------------------------------------------

type replaceRequest struct {
	Kind       Kind            `json:"kind"`
	Name       string          `json:"name"`
	Enabled    bool            `json:"enabled"`
	Priority   int             `json:"priority"`
	Config     json.RawMessage `json:"config,omitempty"`
	Categories []Category      `json:"categories,omitempty"`
	Tags       []string        `json:"tags,omitempty"`
	ProxyID    string          `json:"proxy_id,omitempty"`
}

func (s *Service) handleReplace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req replaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.Kind == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "kind and name are required")
		return
	}
	def := Definition{
		ID:         id,
		Kind:       req.Kind,
		Name:       req.Name,
		Enabled:    req.Enabled,
		Priority:   req.Priority,
		Config:     req.Config,
		Categories: req.Categories,
		Tags:       req.Tags,
		ProxyID:    req.ProxyID,
	}
	saved, err := s.Replace(r.Context(), def)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "indexer not found")
			return
		}
		if errors.Is(err, ErrUnknownKind) {
			writeError(w, http.StatusBadRequest, "unknown_kind", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "replace_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// --- patch ----------------------------------------------------------

type patchRequest struct {
	Name     *string   `json:"name,omitempty"`
	Enabled  *bool     `json:"enabled,omitempty"`
	Priority *int      `json:"priority,omitempty"`
	Tags     *[]string `json:"tags,omitempty"`
	// ProxyID is tri-state on the wire: omitted = unchanged, null or
	// "" = clear, "id" = set. nullableString captures all three.
	ProxyID nullableString `json:"proxy_id,omitempty"`
}

// nullableString is a JSON-tri-state string. Use .Set to test
// presence and .Value for the (possibly empty) string.
type nullableString struct {
	Set   bool
	Value string
}

func (n *nullableString) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = ""
		return nil
	}
	return json.Unmarshal(b, &n.Value)
}

func (s *Service) handlePatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	patch := Patch{
		ID:       id,
		Name:     req.Name,
		Enabled:  req.Enabled,
		Priority: req.Priority,
		Tags:     req.Tags,
	}
	if req.ProxyID.Set {
		v := req.ProxyID.Value
		patch.ProxyID = &v
	}
	saved, err := s.Patch(r.Context(), patch)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "indexer not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "patch_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
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
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func (s *Service) handleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h, err := s.TestOne(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "indexer not found")
		return
	}
	out := testResponse{OK: err == nil, LatencyMS: h.LatencyMS}
	if err != nil {
		out.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// --- caps -----------------------------------------------------------

func (s *Service) handleCaps(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	caps, ok := s.CapsFor(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "indexer not found")
		return
	}
	writeJSON(w, http.StatusOK, caps)
}

// --- search ---------------------------------------------------------

type searchRequest struct {
	Query      string     `json:"query"`
	Categories []Category `json:"categories,omitempty"`
	IndexerIDs []string   `json:"indexer_ids,omitempty"`
	IMDBID     string     `json:"imdb_id,omitempty"`
	TVDBID     string     `json:"tvdb_id,omitempty"`
	TMDBID     string     `json:"tmdb_id,omitempty"`
	Season     int        `json:"season,omitempty"`
	Episode    int        `json:"episode,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	TimeoutMS  int        `json:"timeout_ms,omitempty"`
}

func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	q := Query{
		Term:       req.Query,
		Categories: req.Categories,
		IMDBID:     req.IMDBID,
		TVDBID:     req.TVDBID,
		TMDBID:     req.TMDBID,
		Season:     req.Season,
		Episode:    req.Episode,
		Limit:      req.Limit,
	}
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	out := s.Search(r.Context(), q, req.IndexerIDs, timeout)
	writeJSON(w, http.StatusOK, out)
}

// generateID derives a stable, URL-safe slug from kind + name when the
// caller didn't supply one. We keep it boring on purpose: lowercase
// the name, replace non-alphanumeric runs with hyphens, prepend a
// short kind-prefix.
func generateID(kind Kind, name string) string {
	prefix := strings.TrimSpace(string(kind))
	if i := strings.Index(prefix, "/"); i >= 0 {
		prefix = prefix[i+1:]
	}
	if prefix == "" {
		prefix = "indexer"
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
