package customformats

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for the /api/v1/custom-formats endpoints.
func Router(store *Store, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	h := &handler{store: store, logger: logger}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Post("/test", h.test)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	return r
}

type handler struct {
	store  *Store
	logger *slog.Logger
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	formats, err := h.store.List(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if formats == nil {
		formats = []CustomFormat{}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": formats})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cf, err := h.store.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "custom format not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, cf)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var cf CustomFormat
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if cf.ID == "" || cf.Name == "" {
		h.writeError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	if err := h.store.Create(r.Context(), &cf); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("custom format created", "id", cf.ID, "name", cf.Name)
	h.writeJSON(w, http.StatusCreated, cf)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var cf CustomFormat
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cf.ID = id
	if err := h.store.Update(r.Context(), &cf); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.writeError(w, http.StatusNotFound, "custom format not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("custom format updated", "id", cf.ID)
	h.writeJSON(w, http.StatusOK, cf)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.writeError(w, http.StatusNotFound, "custom format not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("custom format deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// test evaluates a release title against all stored custom formats.
func (h *handler) test(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		h.writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	formats, err := h.store.List(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ri := ParseReleaseName(req.Title)
	engine := NewEngine(formats)
	matches := engine.ScoreRelease(ri)
	if matches == nil {
		matches = []FormatMatch{}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"release": ri,
		"matches": matches,
		"score":   TotalScore(matches),
	})
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg},
	})
}
