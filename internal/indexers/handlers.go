package indexers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loomctl/loom/internal/indexers/throttle"
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
		r.Get("/definitions", s.handleListDefinitions)
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

	// Phase 2f rate-limit dials. All optional; when omitted the row
	// stores NULL and the runtime falls back to throttle.Defaults().
	RateLimitPerMin  *int `json:"rate_limit_per_min,omitempty"`
	RateLimitBurst   *int `json:"rate_limit_burst,omitempty"`
	RetryMaxAttempts *int `json:"retry_max_attempts,omitempty"`
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
	if rateLimitPresent(req.RateLimitPerMin, req.RateLimitBurst, req.RetryMaxAttempts) {
		if err := s.SetRateLimit(r.Context(), saved.ID, rateLimitConfigFrom(req.RateLimitPerMin, req.RateLimitBurst, req.RetryMaxAttempts)); err != nil {
			writeError(w, http.StatusInternalServerError, "create_rate_limit_failed", err.Error())
			return
		}
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
	writeJSON(w, http.StatusOK, map[string]any{"indexers": defs})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dh, err := s.GetWithHealth(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "indexer not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
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

	// Phase 2f rate-limit dials. Same nullable-on-the-wire semantics
	// as createRequest: omit to leave the row's column NULL.
	RateLimitPerMin  *int `json:"rate_limit_per_min,omitempty"`
	RateLimitBurst   *int `json:"rate_limit_burst,omitempty"`
	RetryMaxAttempts *int `json:"retry_max_attempts,omitempty"`
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
	if rateLimitPresent(req.RateLimitPerMin, req.RateLimitBurst, req.RetryMaxAttempts) {
		if err := s.SetRateLimit(r.Context(), saved.ID, rateLimitConfigFrom(req.RateLimitPerMin, req.RateLimitBurst, req.RetryMaxAttempts)); err != nil {
			writeError(w, http.StatusInternalServerError, "replace_rate_limit_failed", err.Error())
			return
		}
	}
	dh, _ := s.GetWithHealth(r.Context(), saved.ID)
	writeJSON(w, http.StatusOK, dh)
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

	// Phase 2f: nil pointers leave the column unchanged; non-nil
	// values overwrite. Pass an explicit 0 to retry_max_attempts to
	// disable retries; pass a positive integer to PerMinute/Burst to
	// override the package default.
	RateLimitPerMin  *int `json:"rate_limit_per_min,omitempty"`
	RateLimitBurst   *int `json:"rate_limit_burst,omitempty"`
	RetryMaxAttempts *int `json:"retry_max_attempts,omitempty"`
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
	// Patch keeps the "nil = unchanged" convention for rate-limit
	// fields too: only update when at least one was supplied. We
	// merge with the persisted values so a partial PATCH that only
	// changes burst doesn't blow away PerMinute/MaxRetries.
	if req.RateLimitPerMin != nil || req.RateLimitBurst != nil || req.RetryMaxAttempts != nil {
		current, _ := s.repo.GetRateLimit(r.Context(), saved.ID)
		// Convert "unset" sentinels to nil pointers so the merge is
		// correct: PerMinute=0 → nil; MaxRetries=-1 → nil.
		merged := mergeRateLimit(current, req.RateLimitPerMin, req.RateLimitBurst, req.RetryMaxAttempts)
		if err := s.SetRateLimit(r.Context(), saved.ID, merged); err != nil {
			writeError(w, http.StatusInternalServerError, "patch_rate_limit_failed", err.Error())
			return
		}
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
	ScoreResults(out.Results)
	writeJSON(w, http.StatusOK, out)
}

// --- definitions ----------------------------------------------------

func (s *Service) handleListDefinitions(w http.ResponseWriter, r *http.Request) {
	if s.definitionLister == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	all := s.definitionLister.ListDefinitions()

	type settingJSON struct {
		Name    string `json:"name"`
		Type    string `json:"type,omitempty"`
		Label   string `json:"label,omitempty"`
		Default string `json:"default,omitempty"`
	}
	type defJSON struct {
		ID          string        `json:"id"`
		Name        string        `json:"name"`
		Description string        `json:"description,omitempty"`
		Type        string        `json:"type,omitempty"`
		Language    string        `json:"language,omitempty"`
		Links       []string      `json:"links,omitempty"`
		Settings    []settingJSON `json:"settings,omitempty"`
		Categories  []string      `json:"categories,omitempty"`
	}

	defs := make([]defJSON, 0, len(all))
	for _, d := range all {
		j := defJSON{
			ID:          d.ID,
			Name:        d.Name,
			Description: d.Description,
			Type:        d.Type,
			Language:    d.Language,
			Links:       d.Links,
			Categories:  d.Categories,
		}
		for _, st := range d.Settings {
			j.Settings = append(j.Settings, settingJSON{
				Name:    st.Name,
				Type:    st.Type,
				Label:   st.Label,
				Default: st.Default,
			})
		}
		defs = append(defs, j)
	}
	sort.Slice(defs, func(i, j int) bool {
		return strings.ToLower(defs[i].Name) < strings.ToLower(defs[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": defs})
}

// rateLimitPresent reports whether any of the three rate-limit fields
// was provided on a request body. Used by handleCreate / handleReplace
// to decide whether to issue the SetRateLimit call.
func rateLimitPresent(perMin, burst, maxRetries *int) bool {
	return perMin != nil || burst != nil || maxRetries != nil
}

// rateLimitConfigFrom maps create/replace request fields into a
// throttle.Config. Unset pointers become "use the default" sentinels
// (zero for PerMinute/Burst, -1 for MaxRetries) so the repository can
// store them as NULL.
func rateLimitConfigFrom(perMin, burst, maxRetries *int) throttle.Config {
	cfg := throttle.Config{MaxRetries: -1}
	if perMin != nil && *perMin > 0 {
		cfg.PerMinute = *perMin
	}
	if burst != nil && *burst > 0 {
		cfg.Burst = *burst
	}
	if maxRetries != nil && *maxRetries >= 0 {
		cfg.MaxRetries = *maxRetries
	}
	return cfg
}

// mergeRateLimit applies a partial PATCH on top of a current Config,
// preserving any fields the caller didn't touch. It accepts the same
// nullable convention as the create/replace request fields.
func mergeRateLimit(current throttle.Config, perMin, burst, maxRetries *int) throttle.Config {
	if perMin != nil {
		if *perMin > 0 {
			current.PerMinute = *perMin
		} else {
			current.PerMinute = 0
		}
	}
	if burst != nil {
		if *burst > 0 {
			current.Burst = *burst
		} else {
			current.Burst = 0
		}
	}
	if maxRetries != nil {
		if *maxRetries >= 0 {
			current.MaxRetries = *maxRetries
		} else {
			current.MaxRetries = -1
		}
	}
	return current
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
