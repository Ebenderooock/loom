package rss

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Mount attaches every /api/v1/rss/sources/* route to r. The caller
// wraps r in auth.RequireAuth (see internal/server/server.go).
func (s *SourcesService) Mount(r chi.Router) {
	r.Route("/api/v1/rss/sources", func(r chi.Router) {
		r.Get("/", s.handleListSources)
		r.Post("/", s.handleCreateSource)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetSource)
			r.Put("/", s.handleUpdateSource)
			r.Patch("/", s.handlePatchSource)
			r.Delete("/", s.handleDeleteSource)
			r.Post("/test", s.handleTestSource)
		})
	})
}

// errorBody is the error envelope returned by source handlers.
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

// generateID creates a simple ID from the source type and name.
// Format: <type>-<normalized-name> (e.g., "rss-tmdb-trending").
func generateID(sourceType SourceType, name string) string {
	typePrefix := strings.TrimSpace(string(sourceType))
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, normalized)
	normalized = strings.TrimLeft(normalized, "-")
	normalized = strings.TrimRight(normalized, "-")
	normalized = strings.ReplaceAll(normalized, "--", "-")
	if normalized == "" {
		normalized = "unnamed"
	}
	return typePrefix + "-" + normalized
}

// --- create ---

type createSourceRequest struct {
	ID     string          `json:"id,omitempty"`
	Name   string          `json:"name"`
	Type   SourceType      `json:"type"`
	Config json.RawMessage `json:"config"`
}

func (s *SourcesService) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	var req createSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "type is required")
		return
	}
	if len(req.Config) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "config is required")
		return
	}

	// Validate config structure based on type
	if err := validateConfig(req.Type, req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}

	if req.ID == "" {
		req.ID = generateID(req.Type, req.Name)
	}

	source, err := s.CreateSource(r.Context(), req.ID, req.Name, req.Type, req.Config)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeError(w, http.StatusConflict, "name_exists", "source name already exists")
			return
		}
		s.logger.Error("create source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create source")
		return
	}

	writeJSON(w, http.StatusCreated, source)
}

// --- get ---

func (s *SourcesService) handleGetSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	source, err := s.GetSource(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "source not found")
			return
		}
		s.logger.Error("get source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get source")
		return
	}

	writeJSON(w, http.StatusOK, source)
}

// --- list ---

func (s *SourcesService) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.ListSources(r.Context())
	if err != nil {
		s.logger.Error("list sources failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to list sources")
		return
	}

	if sources == nil {
		sources = make([]*UserSource, 0)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sources": sources,
		"total":   len(sources),
	})
}

// --- update ---

type updateSourceRequest struct {
	Name    string          `json:"name"`
	Type    SourceType      `json:"type"`
	Config  json.RawMessage `json:"config"`
	Enabled *bool           `json:"enabled,omitempty"`
}

func (s *SourcesService) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	var req updateSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "type is required")
		return
	}
	if len(req.Config) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "config is required")
		return
	}

	// Validate config structure based on type
	if err := validateConfig(req.Type, req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	source, err := s.UpdateSource(r.Context(), id, req.Name, req.Type, enabled, req.Config)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "source not found")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeError(w, http.StatusConflict, "name_exists", "source name already exists")
			return
		}
		s.logger.Error("update source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update source")
		return
	}

	writeJSON(w, http.StatusOK, source)
}

// --- patch ---

type patchSourceRequest struct {
	Name    *string         `json:"name,omitempty"`
	Type    *SourceType     `json:"type,omitempty"`
	Config  json.RawMessage `json:"config,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

func (s *SourcesService) handlePatchSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	var req patchSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	// Fetch the existing source to preserve unpatched fields
	existing, err := s.GetSource(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "source not found")
			return
		}
		s.logger.Error("get source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get source")
		return
	}

	// Apply patches
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}

	typ := existing.Type
	if req.Type != nil {
		typ = *req.Type
	}

	config := existing.Config
	if len(req.Config) > 0 {
		config = req.Config
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Validate config if changed
	if len(req.Config) > 0 {
		if err := validateConfig(typ, config); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
			return
		}
	}

	source, err := s.UpdateSource(r.Context(), id, name, typ, enabled, config)
	if err != nil {
		s.logger.Error("patch source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to patch source")
		return
	}

	writeJSON(w, http.StatusOK, source)
}

// --- delete ---

func (s *SourcesService) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	if err := s.DeleteSource(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "source not found")
			return
		}
		s.logger.Error("delete source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete source")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- test ---

func (s *SourcesService) handleTestSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	source, err := s.GetSource(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "source not found")
			return
		}
		s.logger.Error("get source failed", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get source")
		return
	}

	// Test the source and fetch preview items
	items, testErr := s.testSourceWithPreview(r.Context(), source)

	if testErr != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   testErr.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "test successful",
		"items":   items,
		"count":   len(items),
	})
}

// testSource performs a test fetch against the source.
func (s *SourcesService) testSource(ctx context.Context, source *UserSource) error {
	switch source.Type {
	case SourceTypeRSS:
		var cfg RSSSourceConfig
		if err := json.Unmarshal(source.Config, &cfg); err != nil {
			return errors.New("invalid RSS config")
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return errors.New("RSS URL is required")
		}

		// Create a generic RSS feed source and fetch
		feedSrc := NewGenericRSSFeedSource(source.ID, source.Name, cfg.URL, time.Hour, s.logger)
		_, err := feedSrc.Fetch(ctx)
		if err != nil {
			return fmt.Errorf("RSS fetch failed: %w", err)
		}
		return nil

	case SourceTypeScraper:
		var cfg ScraperConfig
		if err := json.Unmarshal(source.Config, &cfg); err != nil {
			return errors.New("invalid scraper config")
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return errors.New("scraper URL is required")
		}
		if strings.TrimSpace(cfg.ItemSelector) == "" {
			return errors.New("item selector is required")
		}
		if strings.TrimSpace(cfg.TitleSelector) == "" {
			return errors.New("title selector is required")
		}

		// Create a web scraper and fetch
		scraper, err := NewWebScraper(s.logger, source.ID, source.Name, cfg)
		if err != nil {
			return fmt.Errorf("invalid scraper config: %w", err)
		}
		_, fetchErr := scraper.Fetch(ctx)
		if fetchErr != nil {
			return fmt.Errorf("scraper fetch failed: %w", fetchErr)
		}
		return nil

	default:
		return errors.New("unknown source type")
	}
}

// testSourceWithPreview performs a test fetch and returns a preview of items (max 5).
func (s *SourcesService) testSourceWithPreview(ctx context.Context, source *UserSource) ([]*Item, error) {
	switch source.Type {
	case SourceTypeRSS:
		var cfg RSSSourceConfig
		if err := json.Unmarshal(source.Config, &cfg); err != nil {
			return nil, errors.New("invalid RSS config")
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, errors.New("RSS URL is required")
		}

		// Create a generic RSS feed source and fetch
		feedSrc := NewGenericRSSFeedSource(source.ID, source.Name, cfg.URL, time.Hour, s.logger)
		items, err := feedSrc.Fetch(ctx)
		if err != nil {
			return nil, fmt.Errorf("RSS fetch failed: %w", err)
		}

		// Return first 5 items
		limit := 5
		if len(items) < limit {
			limit = len(items)
		}
		return items[:limit], nil

	case SourceTypeScraper:
		var cfg ScraperConfig
		if err := json.Unmarshal(source.Config, &cfg); err != nil {
			return nil, errors.New("invalid scraper config")
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, errors.New("scraper URL is required")
		}
		if strings.TrimSpace(cfg.ItemSelector) == "" {
			return nil, errors.New("item selector is required")
		}
		if strings.TrimSpace(cfg.TitleSelector) == "" {
			return nil, errors.New("title selector is required")
		}

		// Create a web scraper and fetch
		scraper, err := NewWebScraper(s.logger, source.ID, source.Name, cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid scraper config: %w", err)
		}
		items, fetchErr := scraper.Fetch(ctx)
		if fetchErr != nil {
			return nil, fmt.Errorf("scraper fetch failed: %w", fetchErr)
		}

		// Return first 5 items
		limit := 5
		if len(items) < limit {
			limit = len(items)
		}
		return items[:limit], nil

	default:
		return nil, errors.New("unknown source type")
	}
}
func validateConfig(typ SourceType, config json.RawMessage) error {
	if len(config) == 0 {
		return errors.New("config cannot be empty")
	}

	switch typ {
	case SourceTypeRSS:
		var cfg RSSSourceConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return errors.New("invalid RSS config: " + err.Error())
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return errors.New("RSS URL is required")
		}
		return nil

	case SourceTypeScraper:
		var cfg ScraperConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return errors.New("invalid scraper config: " + err.Error())
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return errors.New("scraper URL is required")
		}
		if strings.TrimSpace(cfg.SelectorType) == "" {
			return errors.New("selector type is required")
		}
		if cfg.SelectorType != "css" && cfg.SelectorType != "xpath" {
			return errors.New("selector type must be 'css' or 'xpath'")
		}
		if strings.TrimSpace(cfg.ItemSelector) == "" {
			return errors.New("item selector is required")
		}
		if strings.TrimSpace(cfg.TitleSelector) == "" {
			return errors.New("title selector is required")
		}
		return nil

	default:
		return errors.New("unknown source type")
	}
}
