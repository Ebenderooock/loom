package autosearch

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Handler provides HTTP endpoints for the autosearch engine.
type Handler struct {
	engine *Engine
	logger *slog.Logger
}

// NewHandler creates a new autosearch HTTP handler.
func NewHandler(engine *Engine, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		engine: engine,
		logger: logger.With("module", "autosearch/handler"),
	}
}

// Mount registers autosearch routes on the given mux.
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/autosearch", h.HandleAutoSearch)
}

// HandleAutoSearch triggers an automated search + grab for a media item.
func (h *Handler) HandleAutoSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
		return
	}
	if req.QualityProfileID == "" {
		http.Error(w, `{"error":"quality_profile_id is required"}`, http.StatusBadRequest)
		return
	}

	result, err := h.engine.SearchAndGrab(r.Context(), req)
	if err != nil {
		h.logger.Error("autosearch failed", "error", err, "title", req.Title)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
