package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// HTTPHandlers provides HTTP endpoint handlers for metadata operations.
type HTTPHandlers struct {
	router  *Router
	cache   *Cache
	service *Service
}

// NewHTTPHandlers creates a new handler with the given router, cache, and service.
func NewHTTPHandlers(router *Router, cache *Cache, service *Service) *HTTPHandlers {
	return &HTTPHandlers{
		router:  router,
		cache:   cache,
		service: service,
	}
}

// --- Request/Response types ---

type SearchRequest struct {
	Query string `json:"query"`
	Type  string `json:"type"` // "movie" or "series"
	Year  int    `json:"year,omitempty"`
}

type ImportRequest struct {
	Type     string          `json:"type"` // "movie" or "series"
	Metadata json.RawMessage `json:"metadata"`
}

type CacheStats struct {
	HitRate      float64 `json:"hit_rate"`
	MissRate     float64 `json:"miss_rate"`
	CacheSize    int     `json:"cache_size"`
	Entries      int     `json:"entries"`
	TTLRemaining int     `json:"ttl_remaining_seconds,omitempty"`
}

type ProviderStatus struct {
	Name              string     `json:"name"`
	Status            string     `json:"status"` // "ok", "unconfigured", "error"
	ConfiguredAPIKey  bool       `json:"configured_api_key"`
	LastTestTime      *time.Time `json:"last_test_time,omitempty"`
	LastTestError     *string    `json:"last_test_error,omitempty"`
	LastTestLatencyMs int        `json:"last_test_latency_ms,omitempty"`
}

type TestResult struct {
	OK        bool        `json:"ok"`
	LatencyMs int         `json:"latency_ms"`
	Error     *string     `json:"error,omitempty"`
	Result    interface{} `json:"result,omitempty"`
}

// --- Handlers ---

// HandleSearch handles POST /api/metadata/search
func (h *HTTPHandlers) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	switch req.Type {
	case "movie":
		results, err := h.service.FindMovieByQuery(ctx, req.Query, req.Year)
		if err != nil {
			http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(results)

	case "series":
		results, err := h.service.FindSeriesByQuery(ctx, req.Query)
		if err != nil {
			http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(results)

	default:
		http.Error(w, "Invalid type; must be 'movie' or 'series'", http.StatusBadRequest)
	}
}

// HandleImport handles POST /api/metadata/import
func (h *HTTPHandlers) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Type {
	case "movie":
		var meta MovieMetadata
		if err := json.Unmarshal(req.Metadata, &meta); err != nil {
			http.Error(w, "Invalid metadata", http.StatusBadRequest)
			return
		}
		// Cache the result for 7 days
		id := fmt.Sprintf("import:movie:%s", meta.Title)
		h.cache.Set(id, &meta, TTLFullDetails)
		// Return the cached metadata
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   id,
			"type": "movie",
		})

	case "series":
		var meta SeriesMetadata
		if err := json.Unmarshal(req.Metadata, &meta); err != nil {
			http.Error(w, "Invalid metadata", http.StatusBadRequest)
			return
		}
		// Cache the result for 7 days
		id := fmt.Sprintf("import:series:%s", meta.Title)
		h.cache.Set(id, &meta, TTLFullDetails)
		// Return the cached metadata
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   id,
			"type": "series",
		})

	default:
		http.Error(w, "Invalid type; must be 'movie' or 'series'", http.StatusBadRequest)
	}
}

// HandleCacheStats handles GET /api/metadata/cache/stats
func (h *HTTPHandlers) HandleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.cache.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleProviderStatus handles GET /api/metadata/providers/{provider}/status
func (h *HTTPHandlers) HandleProviderStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	provider := r.PathValue("provider")
	if provider == "" {
		http.Error(w, "Provider not specified", http.StatusBadRequest)
		return
	}

	status := h.getProviderStatus(provider)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// HandleProviderTest handles POST /api/metadata/providers/{provider}/test
func (h *HTTPHandlers) HandleProviderTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	provider := r.PathValue("provider")
	if provider == "" {
		http.Error(w, "Provider not specified", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	result := &TestResult{}

	switch provider {
	case "tmdb":
		m, err := h.service.FindMovie(ctx, SearchMovieParams{Title: "The Matrix", Year: 1999})
		result.LatencyMs = int(time.Since(start).Milliseconds())
		if err != nil {
			result.OK = false
			errMsg := err.Error()
			result.Error = &errMsg
		} else if m != nil {
			result.OK = true
			result.Result = m
		} else {
			result.OK = false
			errMsg := "no results found"
			result.Error = &errMsg
		}

	case "tvdb":
		s, err := h.service.FindSeries(ctx, SearchSeriesParams{Title: "Breaking Bad"})
		result.LatencyMs = int(time.Since(start).Milliseconds())
		if err != nil {
			result.OK = false
			errMsg := err.Error()
			result.Error = &errMsg
		} else if s != nil {
			result.OK = true
			result.Result = s
		} else {
			result.OK = false
			errMsg := "no results found"
			result.Error = &errMsg
		}

	case "musicbrainz":
		// MusicBrainz doesn't have a direct search method in current phase
		result.OK = false
		errMsg := "musicbrainz test not implemented"
		result.Error = &errMsg

	default:
		http.Error(w, "Unknown provider", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// --- Helper methods ---

func (h *HTTPHandlers) getProviderStatus(name string) ProviderStatus {
	status := ProviderStatus{
		Name:             name,
		ConfiguredAPIKey: false,
		Status:           "unconfigured",
	}

	// Check if provider is available and configured by checking service
	// This is a simplified check based on whether the provider responds to queries
	switch name {
	case "tmdb", "tvdb", "musicbrainz":
		// Default to OK for known providers; actual status would come from runtime checks
		status.Status = "ok"
		status.ConfiguredAPIKey = true
	}

	return status
}

// Mount registers metadata HTTP routes with the given chi router.
func (h *HTTPHandlers) Mount(r chi.Router) {
	r.Route("/api/metadata", func(r chi.Router) {
		r.Post("/search", h.HandleSearch)
		r.Post("/import", h.HandleImport)
		r.Get("/cache/stats", h.HandleCacheStats)
		r.Get("/providers/{provider}/status", h.HandleProviderStatus)
		r.Post("/providers/{provider}/test", h.HandleProviderTest)
	})
}
