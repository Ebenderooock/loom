package apikeys

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for /api/v1/api-keys endpoints.
func Router(store *Store, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	h := &handler{store: store, logger: logger}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Delete("/{id}", h.delete)
	return r
}

type handler struct {
	store  *Store
	logger *slog.Logger
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	keys, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []APIKey{}
	}
	out := make([]APIKeyResponse, len(keys))
	for i, k := range keys {
		out[i] = APIKeyResponse{
			ID:        k.ID,
			Name:      k.Name,
			Key:       MaskKey(k.Key),
			Scopes:    k.Scopes,
			ExpiresAt: k.ExpiresAt,
			LastUsed:  k.LastUsed,
			CreatedAt: k.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Scopes string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Scopes == "" {
		req.Scopes = "*"
	}

	k, rawKey, err := h.store.Create(r.Context(), req.Name, req.Scopes, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logger.Info("api key created", "id", k.ID, "name", k.Name)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         k.ID,
		"name":       k.Name,
		"key":        rawKey,
		"scopes":     k.Scopes,
		"created_at": k.CreatedAt,
	})
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "api key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("api key revoked", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg},
	})
}
