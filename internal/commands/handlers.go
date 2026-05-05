package commands

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for /api/v1/command endpoints.
func Router(queue *Queue, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	h := &cmdHandler{queue: queue, logger: logger}
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.cancel)
	return r
}

type cmdHandler struct {
	queue  *Queue
	logger *slog.Logger
}

func (h *cmdHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     CommandName `json:"name"`
		Body     string      `json:"body"`
		Priority int         `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	cmd, err := h.queue.Enqueue(r.Context(), req.Name, req.Body, req.Priority)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("command queued", "id", cmd.ID, "name", cmd.Name)
	writeJSON(w, http.StatusCreated, cmd)
}

func (h *cmdHandler) list(w http.ResponseWriter, r *http.Request) {
	cmds, err := h.queue.List(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cmds == nil {
		cmds = []Command{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": cmds})
}

func (h *cmdHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cmd, err := h.queue.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "command not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cmd)
}

func (h *cmdHandler) cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.queue.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "command not found or not queued")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("command cancelled", "id", id)
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
